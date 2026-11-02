package feed

import (
	"context"
	"slices"
	"sync"
	"time"

	"vergex/internal/logger"
	"vergex/internal/market/ws"
	"vergex/internal/venue/binance"
)

// subscriptionCheck is how often expired interest is noticed.
const subscriptionCheck = 30 * time.Second

// subscriptions decides what the streams carry: Binance's watchlist candles, and
// whatever trading and open charts asked for. The top coins are left out on
// purpose: their candles matter only after each close, when the hydrator fetches
// them for a fraction of a percent of the rate limit, whereas streaming 200 coins
// at three intervals costs about 10 GB a day.
type subscriptions struct {
	store   *Store
	binance *ws.Feed
	now     func() time.Time
	wake    chan struct{}

	mu      sync.Mutex
	leases  map[leaseKey]*lease
	feeds   []*ws.Feed
	settled Settings
}

type leaseKey struct {
	feed  *ws.Feed
	topic string
}

// lease is interest in a topic: held by followers, or until a time.
type lease struct {
	followers int
	until     time.Time
}

// newSubscriptions manages the Binance stream and any others, which carry only what
// has been asked for.
func newSubscriptions(store *Store, stream *ws.Feed, others ...*ws.Feed) *subscriptions {
	return &subscriptions{
		store: store, binance: stream, now: time.Now, wake: make(chan struct{}, 1),
		leases: map[leaseKey]*lease{}, feeds: append([]*ws.Feed{stream}, others...),
	}
}

// keep holds a topic for at least this long.
func (s *subscriptions) keep(feed *ws.Feed, topic string, keep time.Duration) {
	s.mu.Lock()
	key := leaseKey{feed, topic}
	held, known := s.leases[key]
	if !known {
		held = &lease{}
		s.leases[key] = held
	}
	held.until = s.now().Add(keep)
	s.mu.Unlock()
	if !known {
		s.changed()
	}
}

// retain holds a topic until the returned function is called.
func (s *subscriptions) retain(feed *ws.Feed, topic string) (release func()) {
	s.mu.Lock()
	key := leaseKey{feed, topic}
	held, known := s.leases[key]
	if !known {
		held = &lease{}
		s.leases[key] = held
	}
	held.followers++
	s.mu.Unlock()
	if !known {
		s.changed()
	}
	return sync.OnceFunc(func() {
		s.mu.Lock()
		held.followers--
		s.mu.Unlock()
		s.changed()
	})
}

// changed asks for the topics to be worked out again.
func (s *subscriptions) changed() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run keeps the feeds subscribed to what is wanted until ctx ends.
func (s *subscriptions) Run(ctx context.Context) {
	ticker := time.NewTicker(subscriptionCheck)
	defer ticker.Stop()
	for {
		s.apply()
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

// apply hands each feed its topics, what trading and charts asked for first, so a
// long watchlist cannot crowd them out of a connection's limit.
func (s *subscriptions) apply() {
	settings, err := s.store.LoadSettings()
	if err != nil {
		logger.Warnf("Market data: read settings for the stream: %v", err)
		settings = s.settled
	}
	s.settled = settings

	now := s.now()
	topics := map[*ws.Feed][]string{}
	s.mu.Lock()
	for key, held := range s.leases {
		if held.followers > 0 || now.Before(held.until) {
			topics[key.feed] = append(topics[key.feed], key.topic)
		} else {
			delete(s.leases, key)
		}
	}
	feeds := s.feeds
	s.mu.Unlock()
	for _, list := range topics {
		slices.Sort(list)
	}

	// The price of every coin costs little and shows the connection is alive, but
	// with nothing at all to follow there is no reason to hold one open.
	wantsBinance := settings.Enabled || len(topics[s.binance]) > 0
	var list []string
	if wantsBinance {
		list = append(list, binance.AllPricesTopic)
		list = append(list, topics[s.binance]...)
	}
	if settings.Enabled {
		for _, symbol := range settings.Watchlist {
			for _, interval := range WatchIntervals {
				list = append(list, binance.KlineTopic(symbol, interval))
			}
		}
	}
	topics[s.binance] = unique(list)

	for _, feed := range feeds {
		if !slices.Equal(feed.Topics(), topics[feed]) {
			feed.Set(topics[feed])
		}
	}
}

// unique drops repeats and keeps the first of each.
func unique(list []string) []string {
	seen := make(map[string]bool, len(list))
	kept := list[:0]
	for _, item := range list {
		if !seen[item] {
			seen[item] = true
			kept = append(kept, item)
		}
	}
	return kept
}
