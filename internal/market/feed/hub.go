package feed

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"vergex/internal/market/ws"
	"vergex/internal/venue/binance"
)

const (
	// closedQueueSize is how many finished candles may wait for the writer. Past
	// it they are dropped: the water mark stays put and the hydrator fetches them.
	closedQueueSize = 1024
	// subscriberBacklog is how many distinct candles a slow subscriber can fall
	// behind by before it loses the oldest.
	subscriberBacklog = 8
)

// LiveCandle is the newest update of a candle and when it arrived here.
type LiveCandle struct {
	ws.Candle
	ReceivedAt time.Time
}

// LiveTick is a price and when it arrived here.
type LiveTick struct {
	ws.Tick
	ReceivedAt time.Time
}

// Quote is a coin's latest price and, while the stream gives it, its change over 24 hours.
type Quote struct {
	Price        float64
	Change24hPct *float64
}

type candleKey struct{ venue, symbol, interval string }
type tickKey struct{ venue, symbol string }

// Hub holds the latest of everything the live feeds report, passes finished
// candles to the writer and updates subscribers. Nothing downstream can hold it
// up: the writer's queue drops when full, and subscribers keep only what is new.
type Hub struct {
	now func() time.Time

	mu      sync.RWMutex
	candles map[candleKey]LiveCandle
	prices  map[tickKey]LiveTick
	marks   map[tickKey]LiveTick
	subs    map[candleKey]map[*CandleSubscription]struct{}

	closed        chan ws.Candle
	droppedClosed atomic.Uint64
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{
		now:     time.Now,
		candles: map[candleKey]LiveCandle{},
		prices:  map[tickKey]LiveTick{},
		marks:   map[tickKey]LiveTick{},
		subs:    map[candleKey]map[*CandleSubscription]struct{}{},
		closed:  make(chan ws.Candle, closedQueueSize),
	}
}

// Run applies events until ctx ends.
func (h *Hub) Run(ctx context.Context, events <-chan ws.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			h.apply(event)
		}
	}
}

func (h *Hub) apply(event ws.Event) {
	now := h.now()
	switch {
	case event.Candle != nil:
		h.applyCandle(event.Venue, *event.Candle, now)
	case event.Prices != nil:
		h.applyTicks(h.prices, event.Venue, event.Prices, now)
	case event.Marks != nil:
		h.applyTicks(h.marks, event.Venue, event.Marks, now)
	}
}

func (h *Hub) applyCandle(venue string, candle ws.Candle, now time.Time) {
	key := candleKey{venue, candle.Symbol, candle.Interval}
	live := LiveCandle{Candle: candle, ReceivedAt: now}

	h.mu.Lock()
	// A late update of an older candle must not hide the current one.
	if held, seen := h.candles[key]; !seen || candle.OpenTime >= held.OpenTime {
		h.candles[key] = live
	}
	for subscription := range h.subs[key] {
		subscription.offer(live)
	}
	h.mu.Unlock()

	if candle.Closed && venue == binance.Venue {
		select {
		case h.closed <- candle:
		default:
			h.droppedClosed.Add(1)
		}
	}
}

func (h *Hub) applyTicks(into map[tickKey]LiveTick, venue string, ticks []ws.Tick, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, tick := range ticks {
		into[tickKey{venue, tick.Symbol}] = LiveTick{Tick: tick, ReceivedAt: now}
	}
}

// Latest returns the newest update of a candle series, finished or not.
func (h *Hub) Latest(venue, symbol, interval string) (LiveCandle, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	candle, ok := h.candles[candleKey{venue, symbol, interval}]
	return candle, ok
}

// Mark returns a symbol's latest mark price.
func (h *Hub) Mark(venue, symbol string) (LiveTick, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	tick, ok := h.marks[tickKey{venue, symbol}]
	return tick, ok
}

// Prices returns the last trade price of every symbol a venue has reported.
func (h *Hub) Prices(venue string) map[string]Quote {
	h.mu.RLock()
	defer h.mu.RUnlock()
	prices := make(map[string]Quote, len(h.prices))
	for key, tick := range h.prices {
		if key.venue != venue {
			continue
		}
		quote := Quote{Price: tick.Price}
		if tick.Open24h > 0 {
			change := (tick.Price/tick.Open24h - 1) * 100
			quote.Change24hPct = &change
		}
		prices[key.symbol] = quote
	}
	return prices
}

// Subscribe follows one candle series until the subscription is closed.
func (h *Hub) Subscribe(venue, symbol, interval string) *CandleSubscription {
	key := candleKey{venue, symbol, interval}
	subscription := &CandleSubscription{ready: make(chan struct{}, 1)}
	subscription.stop = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs[key], subscription)
		if len(h.subs[key]) == 0 {
			delete(h.subs, key)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[key] == nil {
		h.subs[key] = map[*CandleSubscription]struct{}{}
	}
	h.subs[key][subscription] = struct{}{}
	return subscription
}

// CandleSubscription is a follower of one candle series. It holds the newest
// update of each candle not yet read, so a slow reader skips intermediate updates
// but still sees every candle's last one, unless it falls very far behind.
type CandleSubscription struct {
	mu      sync.Mutex
	pending []LiveCandle
	ready   chan struct{}
	stop    func()
}

func (s *CandleSubscription) offer(candle LiveCandle) {
	s.mu.Lock()
	if last := len(s.pending) - 1; last >= 0 && s.pending[last].OpenTime == candle.OpenTime {
		s.pending[last] = candle
	} else {
		s.pending = append(s.pending, candle)
		if len(s.pending) > subscriberBacklog {
			s.pending = s.pending[1:]
		}
	}
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// Ready signals that Drain has something to return.
func (s *CandleSubscription) Ready() <-chan struct{} { return s.ready }

// Drain returns what arrived since the last call, oldest first.
func (s *CandleSubscription) Drain() []LiveCandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.pending
	s.pending = nil
	return pending
}

// Close stops the subscription.
func (s *CandleSubscription) Close() { s.stop() }
