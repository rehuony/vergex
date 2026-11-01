package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"vergex/internal/exchange/binance"
	"vergex/internal/market/wsfeed"
	"vergex/internal/store/database/testdb"

	"github.com/gorilla/websocket"
)

// recentFixture is a service with a fake exchange behind both its REST client and
// its stream.
type recentFixture struct {
	service  *Service
	exchange *fakeExchange
	push     func(openTime int64, closePrice string)
	series   *Series
	clock    time.Time
}

func newRecentFixture(t *testing.T) *recentFixture {
	t.Helper()
	store := NewStore(testdb.Open(t))
	if err := store.SaveSettings(false, nil, 0); err != nil {
		t.Fatal(err)
	}
	now := base() + 100*hour + 1000
	exchange := &fakeExchange{now: now, times: map[int64]bool{}}
	for at := base() + 90*hour; at <= base()+100*hour; at += hour {
		exchange.times[at] = true
	}
	rest := httptest.NewServer(exchange)
	t.Cleanup(rest.Close)

	connected := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connected <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(stream.Close)

	service := NewService(store)
	service.client = binance.NewDataClient(binance.NewLimiter()).WithBaseURL(rest.URL)
	service.stream = binance.NewStream("ws" + strings.TrimPrefix(stream.URL, "http"))
	service.subs = newSubscriptions(store, service.stream)
	fixture := &recentFixture{service: service, exchange: exchange, clock: time.UnixMilli(now)}
	service.now = func() time.Time { return fixture.clock }

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	events := make(chan wsfeed.Event, 64)
	service.stream.Set([]string{"btcusdt@kline_1h"})
	go service.stream.Run(ctx, events)
	go service.hub.Run(ctx, events)
	conn := <-connected
	t.Cleanup(func() { conn.Close() })
	waitUntil(t, "the stream to connect", service.stream.Healthy)

	fixture.push = func(openTime int64, closePrice string) {
		t.Helper()
		message := fmt.Sprintf(`{"stream":"btcusdt@kline_1h","data":{"e":"kline","E":1,"s":"BTCUSDT","k":{"t":%d,"T":0,"s":"BTCUSDT","i":"1h","o":"100","c":%q,"h":"200000","l":"90","v":"5","n":1,"x":false,"q":"1","V":"1","Q":"1"}}}`, openTime, closePrice)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "the update to reach the hub", func() bool {
			live, ok := service.hub.Latest(binance.Venue, "BTCUSDT", "1h")
			return ok && live.OpenTime == openTime && fmt.Sprint(live.Close) == closePrice
		})
	}

	series, err := store.EnsureSeries("BTCUSDT", "1h", TierOnDemand, now)
	if err != nil {
		t.Fatal(err)
	}
	var history []binance.Candle
	for at := base() + 90*hour; at < base()+100*hour; at += hour {
		history = append(history, closedBar(at))
	}
	series.FirstOpenTime, series.LastClosedTime = history[0].OpenTime, history[len(history)-1].OpenTime
	if err := store.WriteCandlesForTest(series, history); err != nil {
		t.Fatal(err)
	}
	fixture.series = series
	return fixture
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (f *recentFixture) restCalls() int {
	f.exchange.mu.Lock()
	defer f.exchange.mu.Unlock()
	return f.exchange.requests
}

func TestRecentAnswersFromHistoryAndTheStreamWithoutARequest(t *testing.T) {
	f := newRecentFixture(t)
	f.push(base()+100*hour, "77425.5")

	klines, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 5)
	if err != nil {
		t.Fatal(err)
	}
	if f.restCalls() != 0 {
		t.Fatalf("made %d requests while the stream could answer", f.restCalls())
	}
	if len(klines) != 5 {
		t.Fatalf("got %d candles", len(klines))
	}
	last := klines[len(klines)-1]
	if last.OpenTime != base()+100*hour || last.Close != 77425.5 {
		t.Fatalf("last = %+v, want the forming candle", last)
	}
	if last.CloseTime != base()+101*hour-1 || klines[0].CloseTime != base()+97*hour-1 {
		t.Fatalf("close times = %d, %d", klines[0].CloseTime, last.CloseTime)
	}
	if klines[0].OpenTime != base()+96*hour {
		t.Fatalf("first = %d", klines[0].OpenTime)
	}
}

func TestRecentAsksTheExchangeWhenItCannotVouchForLocalData(t *testing.T) {
	t.Run("no forming candle yet", func(t *testing.T) {
		f := newRecentFixture(t)
		if _, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 5); err != nil {
			t.Fatal(err)
		}
		if f.restCalls() != 1 {
			t.Fatalf("requests = %d, want 1", f.restCalls())
		}
	})
	t.Run("the live candle is from an earlier bar", func(t *testing.T) {
		f := newRecentFixture(t)
		f.push(base()+99*hour, "100")
		if _, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 5); err != nil {
			t.Fatal(err)
		}
		if f.restCalls() != 1 {
			t.Fatalf("requests = %d, want 1", f.restCalls())
		}
	})
	t.Run("history stops short of the forming candle", func(t *testing.T) {
		f := newRecentFixture(t)
		f.push(base()+100*hour, "100")
		f.clock = f.clock.Add(time.Hour)
		f.exchange.now += hour
		f.exchange.times[base()+101*hour] = true
		f.push(base()+101*hour, "101")
		klines, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 5)
		if err != nil {
			t.Fatal(err)
		}
		if f.restCalls() != 1 || klines[len(klines)-1].OpenTime != base()+101*hour {
			t.Fatalf("requests = %d, last = %+v", f.restCalls(), klines[len(klines)-1])
		}
	})
	t.Run("not enough history", func(t *testing.T) {
		f := newRecentFixture(t)
		f.push(base()+100*hour, "100")
		if _, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 200); err != nil {
			t.Fatal(err)
		}
		if f.restCalls() != 1 {
			t.Fatalf("requests = %d, want 1", f.restCalls())
		}
	})
	t.Run("an interval that is not kept locally", func(t *testing.T) {
		f := newRecentFixture(t)
		if _, err := f.service.Recent(context.Background(), "BTCUSDT", "1w", 5); err != nil {
			t.Fatal(err)
		}
		if f.restCalls() != 1 {
			t.Fatalf("requests = %d, want 1", f.restCalls())
		}
	})
}

func TestRecentAsksTheExchangeWhenTheStreamIsDown(t *testing.T) {
	f := newRecentFixture(t)
	f.push(base()+100*hour, "100")
	f.service.stream = binance.NewStream("ws://unused") // Never connected.

	if _, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 5); err != nil {
		t.Fatal(err)
	}
	if f.restCalls() != 1 {
		t.Fatalf("requests = %d, want 1", f.restCalls())
	}
}

func TestRecentSurvivesAFailingExchangeWithAnError(t *testing.T) {
	f := newRecentFixture(t)
	f.exchange.mu.Lock()
	f.exchange.status = http.StatusBadGateway
	f.exchange.mu.Unlock()
	if _, err := f.service.Recent(context.Background(), "BTCUSDT", "1h", 5); err == nil {
		t.Fatal("expected the exchange's error")
	}
}

func TestRecentKeepsTheSeriesSubscribedForNextTime(t *testing.T) {
	f := newRecentFixture(t)
	if _, err := f.service.Recent(context.Background(), "ETHUSDT", "3m", 5); err != nil {
		t.Fatal(err)
	}
	found, err := f.service.Store.FindSeries("ETHUSDT", "3m")
	if err != nil || found == nil || found.Status != StatusActive {
		t.Fatalf("series = %+v, %v", found, err)
	}
	f.service.subs.apply()
	if !slices.Contains(f.service.stream.Topics(), "ethusdt@kline_3m") {
		t.Fatalf("topics = %v", f.service.stream.Topics())
	}
}
