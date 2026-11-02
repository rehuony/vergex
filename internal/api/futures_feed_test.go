package api

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"vergex/internal/futures"
	"vergex/internal/futures/agent"
	"vergex/internal/market/feed"
	"vergex/internal/market/ws"
)

type fakeChartSource struct {
	mu    sync.Mutex
	calls int
	rows  []futures.Candle
}

func (f *fakeChartSource) Candles(context.Context, futures.CandleQuery) ([]futures.Candle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.rows, nil
}

func (f *fakeChartSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeCharts struct {
	hub        *feed.Hub
	up         bool
	subscribed chan struct{}
}

func (f *fakeCharts) follow(venue, symbol, interval string) chartCandles {
	if f.hub == nil {
		return nil
	}
	defer close(f.subscribed)
	return f.hub.Subscribe(venue, symbol, interval)
}

func (f *fakeCharts) healthy(string) bool { return f.up }

// feedRows runs a feed and reports the candles of each event sent.
func feedRows(t *testing.T, source futures.ChartSource, live liveCharts, pollEvery time.Duration, liveOnly bool) chan []futures.Candle {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	rows := make(chan []futures.Candle, 256)
	write := func(event agent.Event) error {
		raw, _ := json.Marshal(event.Data)
		var data struct {
			Candles []futures.Candle `json:"candles"`
		}
		_ = json.Unmarshal(raw, &data)
		select {
		case rows <- data.Candles:
		default:
		}
		return nil
	}
	query := futures.CandleQuery{Venue: "binance", Symbol: "BTCUSDT", Interval: "1h", Limit: 300}
	go func() {
		runChartFeed(ctx, write, source, live, query, pollEvery, liveOnly)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("feed did not stop with its context")
		}
	})
	return rows
}

func nextRows(t *testing.T, rows chan []futures.Candle) []futures.Candle {
	t.Helper()
	select {
	case got := <-rows:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("no candles")
		return nil
	}
}

func TestChartFeedSendsHistoryThenStreamWithoutPolling(t *testing.T) {
	hub := feed.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan ws.Event)
	go hub.Run(ctx, events)

	source := &fakeChartSource{rows: []futures.Candle{{Time: 3600, Close: "100"}}}
	charts := &fakeCharts{hub: hub, up: true, subscribed: make(chan struct{})}
	rows := feedRows(t, source, charts, 10*time.Millisecond, false)

	if first := nextRows(t, rows); len(first) != 1 || first[0].Close != "100" {
		t.Fatalf("history = %+v", first)
	}
	<-charts.subscribed
	events <- ws.Event{Venue: "binance", Candle: &ws.Candle{Symbol: "BTCUSDT", Interval: "1h", OpenTime: 7200000, Open: 1.5, High: 2, Low: 1, Close: 77425.1, Volume: 3}}

	// A quiet market repeats the last candle so the chart knows the feed is alive.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-rows:
			if len(got) == 1 && got[0].Time == 7200 {
				if got[0].Close != "77425.1" || got[0].Open != "1.5" {
					t.Fatalf("row = %+v", got[0])
				}
				if calls := source.callCount(); calls != 1 {
					t.Fatalf("exchange asked %d times while the stream is up", calls)
				}
				return
			}
		case <-deadline:
			t.Fatal("the stream's candle never arrived")
		}
	}
}

func TestChartFeedPollsWhileTheStreamIsDown(t *testing.T) {
	source := &fakeChartSource{rows: []futures.Candle{{Time: 3600, Close: "100"}}}
	charts := &fakeCharts{hub: feed.NewHub(), up: false, subscribed: make(chan struct{})}
	rows := feedRows(t, source, charts, 5*time.Millisecond, false)
	for range 3 {
		nextRows(t, rows)
	}
	if calls := source.callCount(); calls < 3 {
		t.Fatalf("polled %d times", calls)
	}
}

func TestChartFeedPollsForAVenueWithoutAStream(t *testing.T) {
	source := &fakeChartSource{rows: []futures.Candle{{Time: 3600, Close: "100"}}}
	rows := feedRows(t, source, &fakeCharts{}, 5*time.Millisecond, false)
	for range 3 {
		nextRows(t, rows)
	}
	if calls := source.callCount(); calls < 3 {
		t.Fatalf("polled %d times", calls)
	}
}

func TestLiveOnlyChartFeedNeverAsksTheExchange(t *testing.T) {
	hub := feed.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan ws.Event)
	go hub.Run(ctx, events)

	source := &fakeChartSource{rows: []futures.Candle{{Time: 3600, Close: "100"}}}
	charts := &fakeCharts{hub: hub, up: false, subscribed: make(chan struct{})}
	rows := feedRows(t, source, charts, time.Millisecond, true)
	<-charts.subscribed
	time.Sleep(20 * time.Millisecond) // Polling would have run many times by now.
	events <- ws.Event{Venue: "binance", Candle: &ws.Candle{Symbol: "BTCUSDT", Interval: "1h", OpenTime: 7200000, Open: 1, High: 2, Low: 1, Close: 2, Volume: 3}}
	if got := nextRows(t, rows); len(got) != 1 || got[0].Time != 7200 {
		t.Fatalf("rows = %+v", got)
	}
	if calls := source.callCount(); calls != 0 {
		t.Fatalf("exchange asked %d times", calls)
	}
}
