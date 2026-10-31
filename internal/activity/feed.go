package activity

import (
	"context"
	"sort"
	"sync"
	"time"

	"vergex/internal/safe"
)

const (
	pollEvery = 3 * time.Second
	// keepEvents bounds the list. Totals are kept apart, so they stay exact past it.
	keepEvents = 500
	window     = 24 * time.Hour
	// maxCatchUp bounds how many reads one round makes while rows keep coming.
	maxCatchUp = 50
)

// stableAssets are counted as dollars in a route's total. A fee in anything else
// still counts as an event but adds nothing to the amount.
var stableAssets = map[string]bool{"": true, "USD": true, "USDT": true, "USDC": true}

// Feed holds recent events in memory, rebuilt from the tables when it starts.
type Feed struct {
	reader *reader
	now    func() time.Time

	mu      sync.Mutex
	events  []Event // Oldest first.
	seq     int64
	seen    map[string]int64
	minutes map[string]map[int64]*Total
	waiters map[chan struct{}]struct{}
}

// New creates a feed over the given sources.
func New(sources Sources) *Feed {
	feed := &Feed{
		now:     time.Now,
		seen:    map[string]int64{},
		minutes: map[string]map[int64]*Total{},
		waiters: map[chan struct{}]struct{}{},
	}
	feed.reader = newReader(sources, feed.now)
	return feed
}

// Start rebuilds the feed from the tables, then follows them until ctx ends.
func (f *Feed) Start(ctx context.Context) {
	safe.GoNamed("activity-feed", func() {
		f.poll(ctx)
		ticker := time.NewTicker(pollEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f.poll(ctx)
			}
		}
	})
}

// poll reads until the tables have nothing more, so a backlog arrives in one round.
func (f *Feed) poll(ctx context.Context) {
	for round := 0; round < maxCatchUp && ctx.Err() == nil; round++ {
		events, more := f.reader.read()
		f.add(events)
		if !more {
			return
		}
	}
}

func (f *Feed) add(events []Event) {
	if len(events) == 0 {
		return
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At < events[j].At })

	f.mu.Lock()
	defer f.mu.Unlock()
	oldest := f.now().Add(-window).UnixMilli()
	added := false
	for _, event := range events {
		if _, seen := f.seen[event.ID]; seen || event.At < oldest {
			continue
		}
		f.seen[event.ID] = event.At
		f.seq++
		event.Seq = f.seq
		f.events = append(f.events, event)
		f.count(event)
		added = true
	}
	if !added {
		return
	}
	if extra := len(f.events) - keepEvents; extra > 0 {
		f.events = append([]Event(nil), f.events[extra:]...)
	}
	for id, at := range f.seen {
		if at < oldest {
			delete(f.seen, id)
		}
	}
	for waiter := range f.waiters {
		select {
		case waiter <- struct{}{}:
		default:
		}
	}
}

func (f *Feed) count(event Event) {
	route := event.Route()
	minutes := f.minutes[route]
	if minutes == nil {
		minutes = map[int64]*Total{}
		f.minutes[route] = minutes
	}
	minute := event.At / time.Minute.Milliseconds()
	total := minutes[minute]
	if total == nil {
		total = &Total{}
		minutes[minute] = total
	}
	total.Count++
	total.Tokens += int64(event.Tokens)
	total.Items += int64(event.Items)
	if stableAssets[event.Asset] {
		total.Amount += event.Amount
	}
}

// After returns the events past a sequence number, oldest first, and current totals.
func (f *Feed) After(seq int64) Update {
	f.mu.Lock()
	defer f.mu.Unlock()
	start := sort.Search(len(f.events), func(i int) bool { return f.events[i].Seq > seq })
	return Update{
		Seq:    f.seq,
		At:     f.now().UnixMilli(),
		Events: append([]Event{}, f.events[start:]...),
		Totals: f.totals(),
	}
}

func (f *Feed) totals() map[string]RouteTotal {
	now := f.now()
	day := now.Add(-window).UnixMilli() / time.Minute.Milliseconds()
	hour := now.Add(-time.Hour).UnixMilli() / time.Minute.Milliseconds()
	out := make(map[string]RouteTotal, len(f.minutes))
	for route, minutes := range f.minutes {
		var row RouteTotal
		for minute, total := range minutes {
			if minute <= day {
				delete(minutes, minute)
				continue
			}
			row.H24 = row.H24.plus(*total)
			if minute > hour {
				row.H1 = row.H1.plus(*total)
			}
		}
		out[route] = row
	}
	return out
}

func (t Total) plus(other Total) Total {
	t.Count += other.Count
	t.Tokens += other.Tokens
	t.Items += other.Items
	t.Amount += other.Amount
	return t
}

// Subscribe returns a channel that wakes when new events arrive, and how to stop.
func (f *Feed) Subscribe() (<-chan struct{}, func()) {
	wake := make(chan struct{}, 1)
	f.mu.Lock()
	f.waiters[wake] = struct{}{}
	f.mu.Unlock()
	return wake, func() {
		f.mu.Lock()
		delete(f.waiters, wake)
		f.mu.Unlock()
	}
}
