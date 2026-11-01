package feed

import (
	"context"
	"testing"
	"time"

	"vergex/internal/exchange/binance"
	"vergex/internal/market/wsfeed"
)

func candleEvent(openTime int64, close float64, closed bool) wsfeed.Event {
	return wsfeed.Event{Venue: binance.Venue, Candle: &wsfeed.Candle{
		Symbol: "BTCUSDT", Interval: "1h", OpenTime: openTime, Open: close, High: close, Low: close, Close: close, Closed: closed,
	}}
}

func TestHubKeepsTheNewestCandleEvenIfUpdatesArriveOutOfOrder(t *testing.T) {
	hub := NewHub()
	hub.apply(candleEvent(2*hour, 200, false))
	hub.apply(candleEvent(hour, 100, false))
	live, ok := hub.Latest(binance.Venue, "BTCUSDT", "1h")
	if !ok || live.OpenTime != 2*hour || live.Close != 200 {
		t.Fatalf("latest = %+v, %v", live, ok)
	}
	hub.apply(candleEvent(2*hour, 210, false))
	if live, _ := hub.Latest(binance.Venue, "BTCUSDT", "1h"); live.Close != 210 {
		t.Fatalf("close = %v, want the newest update", live.Close)
	}
	if _, ok := hub.Latest("okx", "BTCUSDT", "1h"); ok {
		t.Fatal("venues must not share candles")
	}
}

func TestHubQueuesOnlyFinishedBinanceCandles(t *testing.T) {
	hub := NewHub()
	hub.apply(candleEvent(hour, 100, false))
	hub.apply(candleEvent(hour, 101, true))
	okx := candleEvent(hour, 1, true)
	okx.Venue = "okx"
	hub.apply(okx)
	if len(hub.closed) != 1 {
		t.Fatalf("queued = %d, want 1", len(hub.closed))
	}
	if candle := <-hub.closed; candle.Close != 101 {
		t.Fatalf("queued close = %v", candle.Close)
	}
}

func TestHubDropsFinishedCandlesRatherThanWaitForTheWriter(t *testing.T) {
	hub := NewHub()
	done := make(chan struct{})
	go func() {
		for i := range closedQueueSize + 5 {
			hub.apply(candleEvent(int64(i)*hour, 100, true))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the hub waited for a writer nobody is running")
	}
	if dropped := hub.droppedClosed.Load(); dropped != 5 {
		t.Fatalf("dropped = %d, want 5", dropped)
	}
}

func TestSlowSubscriberNeverBlocksAndSeesEachCandlesLastUpdate(t *testing.T) {
	hub := NewHub()
	slow := hub.Subscribe(binance.Venue, "BTCUSDT", "1h")
	defer slow.Close()
	quick := hub.Subscribe(binance.Venue, "BTCUSDT", "1h")

	hub.apply(candleEvent(hour, 100, false))
	<-quick.Ready()
	if got := quick.Drain(); len(got) != 1 || got[0].Close != 100 {
		t.Fatalf("quick got %+v", got)
	}
	quick.Close()

	// The slow one reads nothing while many updates pass.
	done := make(chan struct{})
	go func() {
		for i := range 10000 {
			hub.apply(candleEvent(hour, float64(i), false))
		}
		hub.apply(candleEvent(hour, 999999, true))
		hub.apply(candleEvent(2*hour, 5, false))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a slow subscriber held up the hub")
	}

	got := slow.Drain()
	if len(got) != 2 || got[0].Close != 999999 || !got[0].Closed || got[1].OpenTime != 2*hour {
		t.Fatalf("slow got %+v", got)
	}
	if len(slow.Drain()) != 0 {
		t.Fatal("a drain should empty the backlog")
	}
}

func TestClosedSubscriptionStopsReceiving(t *testing.T) {
	hub := NewHub()
	subscription := hub.Subscribe(binance.Venue, "BTCUSDT", "1h")
	subscription.Close()
	hub.apply(candleEvent(hour, 100, false))
	if got := subscription.Drain(); len(got) != 0 {
		t.Fatalf("got %+v after closing", got)
	}
	if len(hub.subs) != 0 {
		t.Fatal("the subscriber list should be empty")
	}
}

func TestHubReportsPricesAndMarksByVenue(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan wsfeed.Event)
	go hub.Run(ctx, events)
	events <- wsfeed.Event{Venue: binance.Venue, Prices: []wsfeed.Tick{{Symbol: "BTCUSDT", Price: 70000}, {Symbol: "ETHUSDT", Price: 3000}}}
	events <- wsfeed.Event{Venue: "okx", Prices: []wsfeed.Tick{{Symbol: "BTC-USDT-SWAP", Price: 70001}}}
	events <- wsfeed.Event{Venue: binance.Venue, Marks: []wsfeed.Tick{{Symbol: "BTCUSDT", Price: 69999}}}
	// A second unbuffered send proves the first three were taken; this one is applied by the time it returns.
	events <- wsfeed.Event{Venue: binance.Venue, Marks: []wsfeed.Tick{{Symbol: "BTCUSDT", Price: 69998}}}

	deadline := time.Now().Add(5 * time.Second)
	for {
		prices := hub.Prices(binance.Venue)
		mark, ok := hub.Mark(binance.Venue, "BTCUSDT")
		if len(prices) == 2 && prices["ETHUSDT"] == 3000 && ok && mark.Price == 69998 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("prices = %v, mark = %+v", prices, mark)
		}
		time.Sleep(time.Millisecond)
	}
	if _, ok := hub.Mark("okx", "BTCUSDT"); ok {
		t.Fatal("venues must not share marks")
	}
}
