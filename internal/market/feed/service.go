package feed

import (
	"context"
	"errors"
	"sync"
	"time"

	"vergex/internal/logger"
	"vergex/internal/market/ws"
	"vergex/internal/service"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
)

const (
	priceCacheTTL = 10 * time.Second
	// priceWait is how long a page waits for prices over REST. They are a nicety
	// next to local data, and the request may be queueing behind a long catch-up.
	priceWait = 2 * time.Second
	// eventQueueSize is how many stream events may wait for the hub.
	eventQueueSize = 4096
	// markKeep is how long a mark price stays subscribed after it was asked for, and
	// markFresh how old one may be before it no longer counts: they arrive every second.
	markKeep  = 5 * time.Minute
	markFresh = 5 * time.Second
)

// Status is what the settings page and Dashboard show about collection.
type Status struct {
	Enabled      bool                  `json:"enabled"`
	Series       int                   `json:"series"`
	Coins        int                   `json:"coins"`
	Ready        int                   `json:"ready"`
	Behind       int                   `json:"behind"`
	Failing      int                   `json:"failing"`
	LastError    string                `json:"last_error,omitempty"`
	WatchSymbols int                   `json:"watch_symbols"`
	TopSymbols   int                   `json:"top_symbols"`
	Limiter      binance.LimiterStatus `json:"limiter"`
	Backfill     BackfillProgress      `json:"backfill"`
	Stream       ws.Status             `json:"stream"`
}

// Service ties collection together for the rest of the application.
type Service struct {
	Store      *Store
	client     *binance.DataClient
	hydrator   *Hydrator
	collector  *DerivativeCollector
	backfiller *Backfiller
	hub        *Hub
	writer     *Writer
	stream     *ws.Feed
	okxCandles *ws.Feed
	okxMarks   *ws.Feed
	subs       *subscriptions
	written    chan struct{}
	now        func() time.Time

	mu       sync.Mutex
	backfill BackfillProgress
	prices   map[string]Quote
	pricedAt time.Time
}

// NewService creates the service over a market store.
func NewService(store *Store) *Service {
	client := binance.NewDataClient(binance.NewLimiter())
	hydrator := NewHydrator(store, client, binance.Default())
	hub := NewHub()
	stream := binance.NewStream("")
	hydrator.streamDown = func() bool { return len(stream.Topics()) > 0 && !stream.Healthy() }
	okxCandles, okxMarks := okx.NewCandleStream(""), okx.NewMarkStream("")
	written := make(chan struct{}, 1)
	return &Service{
		Store:      store,
		client:     client,
		hydrator:   hydrator,
		collector:  NewDerivativeCollector(store, client),
		backfiller: NewBackfiller(store, client),
		hub:        hub,
		writer:     NewWriter(store, hub.closed, hydrator.Wake, func() { notify(written) }),
		written:    written,
		stream:     stream,
		okxCandles: okxCandles,
		okxMarks:   okxMarks,
		subs:       newSubscriptions(store, stream, okxCandles, okxMarks),
		now:        time.Now,
	}
}

// Start begins collecting in the background until ctx ends.
func (s *Service) Start(ctx context.Context) {
	events := make(chan ws.Event, eventQueueSize)
	service.Go("market-stream", func() { s.stream.Run(ctx, events) })
	service.Go("market-stream-okx-candles", func() { s.okxCandles.Run(ctx, events) })
	service.Go("market-stream-okx-marks", func() { s.okxMarks.Run(ctx, events) })
	service.Go("market-subscriptions", func() { s.subs.Run(ctx) })
	service.Go("market-hub", func() { s.hub.Run(ctx, events) })
	service.Go("market-writer", func() { s.writer.Run(ctx) })
	service.Go("market-data-hydrator", func() { s.hydrator.Run(ctx) })
	service.Go("market-data-derivatives", func() { s.collector.Run(ctx) })
}

// Client returns the rate-limited exchange client.
func (s *Service) Client() *binance.DataClient { return s.client }

// CandlesWritten signals that finished candles from the stream were just stored.
func (s *Service) CandlesWritten() <-chan struct{} { return s.written }

func notify(signal chan<- struct{}) {
	select {
	case signal <- struct{}{}:
	default:
	}
}

// FormingCandle returns the candle now forming on a series, if the stream is
// delivering one that follows on from what is stored.
func (s *Service) FormingCandle(symbol, interval string, lastClosed int64) (LiveCandle, bool) {
	if !s.stream.Healthy() {
		return LiveCandle{}, false
	}
	live, ok := s.hub.Latest(binance.Venue, symbol, interval)
	if !ok || live.Closed || live.OpenTime != lastClosed+IntervalMillis(interval) {
		return LiveCandle{}, false
	}
	return live, true
}

// StreamCounts adds up the market streams: how many messages they have delivered since
// they started, how many topics they follow, and whether the main one is up. It reads
// only their counters.
func (s *Service) StreamCounts() (messages uint64, topics int, connected bool) {
	for _, stream := range []*ws.Feed{s.stream, s.okxCandles, s.okxMarks} {
		status := stream.Status()
		messages += status.Messages
		topics += status.Topics
	}
	return messages, topics, s.stream.Status().Connected
}

// ActiveCoins lists the coins market data follows, each once.
func (s *Service) ActiveCoins() ([]string, error) {
	series, err := s.Store.ListSeries()
	if err != nil {
		return nil, err
	}
	coins := make([]string, 0, len(series))
	for _, row := range series {
		// Listed by symbol, so a coin's series sit together.
		if row.Status == StatusActive && (len(coins) == 0 || coins[len(coins)-1] != row.Symbol) {
			coins = append(coins, row.Symbol)
		}
	}
	return coins, nil
}

// Status summarizes collection.
func (s *Service) Status() (Status, error) {
	settings, err := s.Store.LoadSettings()
	if err != nil {
		return Status{}, err
	}
	series, err := s.Store.ListSeries()
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Enabled:      settings.Enabled,
		WatchSymbols: len(settings.Watchlist),
		TopSymbols:   len(settings.Ranked),
		Limiter:      s.client.Limiter().Status(),
		Stream:       s.stream.Status(),
	}
	now := s.now().UnixMilli()
	coins := map[string]bool{}
	for _, row := range series {
		if row.Status != StatusActive {
			continue
		}
		status.Series++
		coins[row.Symbol] = true
		if row.LastClosedTime > 0 {
			status.Ready++
		}
		if closedBehind(row, now) > 0 {
			status.Behind++
		}
		if row.FailCount > 0 {
			status.Failing++
			status.LastError = row.LastError
		}
	}
	status.Coins = len(coins)
	s.mu.Lock()
	status.Backfill = s.backfill
	s.mu.Unlock()
	return status, nil
}

// SaveSettings stores the settings and replans at once.
func (s *Service) SaveSettings(enabled bool, watchlist []string, topN int) error {
	if err := s.Store.SaveSettings(enabled, watchlist, topN); err != nil {
		return err
	}
	s.hydrator.Replan()
	s.subs.changed()
	return nil
}

// Open is called when the user looks at a series: it starts or refreshes it at
// the front of the queue and returns it, with whatever data exists so far.
func (s *Service) Open(symbol, interval string) (*Series, error) {
	if IntervalMillis(interval) == 0 {
		return nil, errors.New("unsupported interval")
	}
	now := s.now().UnixMilli()
	series, err := s.Store.EnsureSeries(symbol, interval, TierOnDemand, now)
	if err != nil {
		return nil, err
	}
	if err := s.Store.markOpened(series, now); err != nil {
		return nil, err
	}
	s.subs.keep(s.stream, binance.KlineTopic(symbol, interval), onDemandKeep)
	s.hydrator.Wake()
	return series, nil
}

// FollowCandles delivers live updates of a candle series and keeps it subscribed
// until the follower closes. Symbols are as the venue names them. It returns nil
// where there is no stream to follow.
func (s *Service) FollowCandles(venue, symbol, interval string) *Follower {
	var release func()
	switch venue {
	case binance.Venue:
		release = s.subs.retain(s.stream, binance.KlineTopic(symbol, interval))
	case okx.Venue:
		topic := okx.CandleTopic(symbol, interval)
		if topic == "" {
			return nil
		}
		release = s.subs.retain(s.okxCandles, topic)
	default:
		return nil
	}
	return &Follower{CandleSubscription: s.hub.Subscribe(venue, symbol, interval), release: release}
}

// Follower is a candle subscription that also holds the stream open.
type Follower struct {
	*CandleSubscription
	release func()
}

// Close stops following.
func (f *Follower) Close() {
	f.CandleSubscription.Close()
	f.release()
}

// StreamHealthy reports whether a venue's candle stream is up and heard from recently.
func (s *Service) StreamHealthy(venue string) bool {
	switch venue {
	case binance.Venue:
		return s.stream.Healthy()
	case okx.Venue:
		return s.okxCandles.Healthy()
	}
	return false
}

// WatchMark keeps a symbol's mark price coming for a few minutes. Call it again
// while the price is still needed.
func (s *Service) WatchMark(exchange, symbol string) {
	switch exchange {
	case binance.Venue:
		s.subs.keep(s.stream, binance.MarkTopic(symbol), markKeep)
	case okx.Venue:
		s.subs.keep(s.okxMarks, okx.MarkTopic(symbol), markKeep)
	}
}

// Mark returns a symbol's mark price if a stream delivered it just now. Symbols are
// the generic ones traders use.
func (s *Service) Mark(exchange, symbol string) (float64, bool) {
	var tick LiveTick
	var ok bool
	switch exchange {
	case binance.Venue:
		if !s.stream.Healthy() {
			return 0, false
		}
		tick, ok = s.hub.Mark(binance.Venue, symbol)
	case okx.Venue:
		if !s.okxMarks.Healthy() {
			return 0, false
		}
		tick, ok = s.hub.Mark(okx.Venue, okx.InstrumentID(symbol))
	}
	if !ok || s.now().Sub(tick.ReceivedAt) > markFresh {
		return 0, false
	}
	return tick.Price, true
}

// Prices returns the latest prices: from the live stream while it is up, otherwise
// from the exchange at most every 10 seconds, without keeping a page waiting. Only the
// stream gives the 24-hour change.
func (s *Service) Prices(ctx context.Context) (map[string]Quote, error) {
	if s.stream.Healthy() {
		if prices := s.hub.Prices(binance.Venue); len(prices) > 0 {
			return prices, nil
		}
	}
	s.mu.Lock()
	if s.prices != nil && s.now().Sub(s.pricedAt) < priceCacheTTL {
		defer s.mu.Unlock()
		return s.prices, nil
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, priceWait)
	defer cancel()
	latest, err := s.client.TickerPrices(ctx)
	if err != nil {
		return nil, err
	}
	prices := make(map[string]Quote, len(latest))
	for symbol, price := range latest {
		prices[symbol] = Quote{Price: price}
	}
	s.mu.Lock()
	s.prices, s.pricedAt = prices, s.now()
	s.mu.Unlock()
	return prices, nil
}

// StartBackfill runs one backfill in the background. A second one is refused
// while the first is running.
func (s *Service) StartBackfill(ctx context.Context, options BackfillOptions) error {
	s.mu.Lock()
	if s.backfill.Running {
		s.mu.Unlock()
		return errors.New("a backfill is already running")
	}
	s.backfill = BackfillProgress{Running: true}
	s.mu.Unlock()
	service.Go("market-data-backfill", func() {
		err := s.backfiller.Run(ctx, options, func(progress BackfillProgress) {
			s.mu.Lock()
			s.backfill = progress
			s.mu.Unlock()
		})
		s.mu.Lock()
		s.backfill.Running = false
		s.backfill.Current = ""
		s.backfill.FinishedAt = s.now().UnixMilli()
		if err != nil {
			s.backfill.Error = err.Error()
			logger.Warnf("Market data: backfill: %v", err)
		}
		s.mu.Unlock()
		s.hydrator.Wake()
	})
	return nil
}

// RunBackfill runs a backfill and waits for it, for the command line.
func (s *Service) RunBackfill(ctx context.Context, options BackfillOptions, report func(BackfillProgress)) error {
	return s.backfiller.Run(ctx, options, report)
}
