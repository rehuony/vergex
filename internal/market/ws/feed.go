// Package ws keeps one WebSocket connection to an exchange, subscribed to a
// set of topics that can change at any time, and passes what arrives to a channel.
//
// The read loop never waits for whoever consumes that channel: when it is full the
// message is dropped and counted. Nothing here is the only copy of anything, since
// consumers fill holes over REST.
package ws

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"vergex/internal/logger"
	"vergex/internal/service"

	"github.com/gorilla/websocket"
)

// Candle is one update of a candle. It repeats until Closed says it is final.
type Candle struct {
	Symbol         string
	Interval       string
	OpenTime       int64 // Unix milliseconds
	Open           float64
	High           float64
	Low            float64
	Close          float64
	Volume         float64
	QuoteVolume    float64
	Trades         int64
	TakerBuyVolume float64
	TakerBuyQuote  float64
	Closed         bool
}

// Tick is a price at a time, in Unix milliseconds.
type Tick struct {
	Symbol string
	Price  float64
	// Open24h is the price 24 hours ago, where the stream gives one.
	Open24h float64
	Time    int64
}

// Event is what one message said. Besides Venue, exactly one field is set.
type Event struct {
	Venue  string
	Candle *Candle
	Prices []Tick // Last trade prices.
	Marks  []Tick // Mark prices.
}

// Protocol is what differs between exchanges.
type Protocol interface {
	// Control builds the message that subscribes to or unsubscribes from topics.
	Control(topics []string, subscribe bool) []byte
	// Parse reads one message. Acknowledgements and the like return no events.
	Parse(raw []byte) ([]Event, error)
}

// Config describes one connection.
type Config struct {
	Venue    string
	URL      string
	Protocol Protocol
	// MaxTopics caps the connection. Topics past it are left out, so they should
	// be passed to Set most important first.
	MaxTopics int
	// BatchTopics is how many topics go in one control message, and ControlEvery
	// how often one may be sent.
	BatchTopics  int
	ControlEvery time.Duration
	// Silence is how long nothing may arrive before the connection counts as dead.
	Silence time.Duration
	// MaxAge replaces the connection before the exchange would close it.
	MaxAge time.Duration
	// Ping, sent every PingEvery, is for exchanges that want a message rather than
	// answering protocol pings.
	Ping      []byte
	PingEvery time.Duration
}

const (
	minBackoff  = time.Second
	maxBackoff  = time.Minute
	stableAfter = time.Minute // A connection that lasted this long resets the backoff.
	dialTimeout = 15 * time.Second
	maxMessage  = 8 << 20
)

// Status is what the feed last knew, for status displays.
type Status struct {
	Connected   bool      `json:"connected"`
	Topics      int       `json:"topics"`
	LeftOut     int       `json:"left_out"`
	LastMessage time.Time `json:"last_message,omitempty"`
	Messages    uint64    `json:"messages"`
	Bytes       uint64    `json:"bytes"`
	Dropped     uint64    `json:"dropped"`
	ParseErrors uint64    `json:"parse_errors"`
	Reconnects  uint64    `json:"reconnects"`
	Subscribes  uint64    `json:"subscribes"`
	LastError   string    `json:"last_error,omitempty"`
}

// Feed is one connection and the topics it should be subscribed to.
type Feed struct {
	config Config

	mu        sync.Mutex
	topics    []string
	leftOut   int
	lastError string
	changed   chan struct{}

	connected   atomic.Bool
	lastMessage atomic.Int64
	messages    atomic.Uint64
	bytes       atomic.Uint64
	dropped     atomic.Uint64
	parseErrors atomic.Uint64
	reconnects  atomic.Uint64
	subscribes  atomic.Uint64
}

// New creates a feed. It does nothing until Run.
func New(config Config) *Feed {
	return &Feed{config: config, changed: make(chan struct{}, 1)}
}

// Set replaces the topics, most important first. The connection catches up on its
// own; with no topics at all it closes.
func (f *Feed) Set(topics []string) {
	leftOut := 0
	if f.config.MaxTopics > 0 && len(topics) > f.config.MaxTopics {
		leftOut = len(topics) - f.config.MaxTopics
		topics = topics[:f.config.MaxTopics]
	}
	f.mu.Lock()
	warn := leftOut > 0 && leftOut != f.leftOut
	f.topics, f.leftOut = append([]string(nil), topics...), leftOut
	f.mu.Unlock()
	if warn {
		logger.Warnf("Market stream %s: %d topics over the limit are left out; REST covers them", f.config.URL, leftOut)
	}
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

// Topics returns the topics the feed should be subscribed to.
func (f *Feed) Topics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.topics
}

// Healthy reports a connection that has heard something recently.
func (f *Feed) Healthy() bool {
	if !f.connected.Load() {
		return false
	}
	return time.Since(time.UnixMilli(f.lastMessage.Load())) < f.config.Silence
}

// Status reports the connection and its counters.
func (f *Feed) Status() Status {
	f.mu.Lock()
	topics, leftOut, lastError := len(f.topics), f.leftOut, f.lastError
	f.mu.Unlock()
	status := Status{
		Connected: f.connected.Load(), Topics: topics, LeftOut: leftOut, LastError: lastError,
		Messages: f.messages.Load(), Bytes: f.bytes.Load(), Dropped: f.dropped.Load(), ParseErrors: f.parseErrors.Load(),
		Reconnects: f.reconnects.Load(), Subscribes: f.subscribes.Load(),
	}
	if at := f.lastMessage.Load(); at > 0 {
		status.LastMessage = time.UnixMilli(at)
	}
	return status
}

// Run keeps the connection up until ctx ends, passing events to out.
func (f *Feed) Run(ctx context.Context, out chan<- Event) {
	backoff := minBackoff
	for f.waitForTopics(ctx) {
		started := time.Now()
		err := f.session(ctx, out)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			continue // Replaced on purpose, or no topics left.
		}
		f.mu.Lock()
		f.lastError = err.Error()
		f.mu.Unlock()
		logger.Warnf("Market stream %s: %v", f.config.URL, err)
		f.reconnects.Add(1)
		if time.Since(started) > stableAfter {
			backoff = minBackoff
		}
		if !sleep(ctx, withJitter(backoff)) {
			return
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (f *Feed) waitForTopics(ctx context.Context) bool {
	for len(f.Topics()) == 0 {
		select {
		case <-ctx.Done():
			return false
		case <-f.changed:
		}
	}
	return ctx.Err() == nil
}

func withJitter(wait time.Duration) time.Duration {
	return wait + time.Duration(rand.Int63n(int64(wait)/4+1))
}

func sleep(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// session runs one connection. It returns nil when the connection ended on purpose.
func (f *Feed) session(ctx context.Context, out chan<- Event) error {
	dialer := websocket.Dialer{HandshakeTimeout: dialTimeout, Proxy: websocket.DefaultDialer.Proxy}
	conn, _, err := dialer.DialContext(ctx, f.config.URL, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(maxMessage)

	f.heard(conn)
	f.connected.Store(true)
	defer f.connected.Store(false)
	conn.SetPingHandler(func(data string) error {
		f.heard(conn)
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
	})

	readErr := make(chan error, 1)
	service.Go("market-stream-read", func() {
		readErr <- f.read(conn, out)
	}, func(recovered any) {
		readErr <- fmt.Errorf("read panic: %v", recovered)
	})

	control := time.NewTicker(f.config.ControlEvery)
	defer control.Stop()
	var ping <-chan time.Time
	if f.config.PingEvery > 0 {
		ticker := time.NewTicker(f.config.PingEvery)
		defer ticker.Stop()
		ping = ticker.C
	}
	var replace <-chan time.Time
	if f.config.MaxAge > 0 {
		timer := time.NewTimer(f.config.MaxAge)
		defer timer.Stop()
		replace = timer.C
	}

	subscribed := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return fmt.Errorf("read: %w", err)
		case <-replace:
			return nil
		case <-ping:
			if err := conn.WriteMessage(websocket.TextMessage, f.config.Ping); err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		case <-control.C:
			if len(f.Topics()) == 0 {
				return nil
			}
			if err := f.sendControl(conn, subscribed); err != nil {
				return fmt.Errorf("subscribe: %w", err)
			}
		}
	}
}

// heard notes that the connection is alive.
func (f *Feed) heard(conn *websocket.Conn) {
	now := time.Now()
	f.lastMessage.Store(now.UnixMilli())
	_ = conn.SetReadDeadline(now.Add(f.config.Silence))
}

func (f *Feed) read(conn *websocket.Conn, out chan<- Event) error {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		f.heard(conn)
		f.messages.Add(1)
		f.bytes.Add(uint64(len(raw)))
		events, err := f.config.Protocol.Parse(raw)
		if err != nil {
			if f.parseErrors.Add(1) == 1 {
				logger.Warnf("Market stream %s: unreadable message: %v", f.config.URL, err)
			}
			continue
		}
		for _, event := range events {
			event.Venue = f.config.Venue
			select {
			case out <- event:
			default:
				f.dropped.Add(1)
			}
		}
	}
}

// sendControl moves the connection one message closer to the wanted topics.
// Unsubscribing goes first, so the connection never holds more than its limit.
func (f *Feed) sendControl(conn *websocket.Conn, subscribed map[string]bool) error {
	want := map[string]bool{}
	var add []string
	for _, topic := range f.Topics() {
		want[topic] = true
		if !subscribed[topic] && len(add) < f.config.BatchTopics {
			add = append(add, topic)
		}
	}
	var remove []string
	for topic := range subscribed {
		if !want[topic] && len(remove) < f.config.BatchTopics {
			remove = append(remove, topic)
		}
	}
	switch {
	case len(remove) > 0:
		for _, topic := range remove {
			delete(subscribed, topic)
		}
		return conn.WriteMessage(websocket.TextMessage, f.config.Protocol.Control(remove, false))
	case len(add) > 0:
		for _, topic := range add {
			subscribed[topic] = true
		}
		f.subscribes.Add(1)
		return conn.WriteMessage(websocket.TextMessage, f.config.Protocol.Control(add, true))
	}
	return nil
}
