package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fakeBinance(t *testing.T, oiCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			fmt.Fprint(w, `{"symbols":[
				{"symbol":"BTCUSDT","status":"TRADING","contractType":"PERPETUAL","quoteAsset":"USDT","marginAsset":"USDT"},
				{"symbol":"ETHUSDT","status":"TRADING","contractType":"PERPETUAL","quoteAsset":"USDT","marginAsset":"USDT"},
				{"symbol":"DOGEUSDT","status":"TRADING","contractType":"PERPETUAL","quoteAsset":"USDT","marginAsset":"USDT"},
				{"symbol":"TINYUSDT","status":"TRADING","contractType":"PERPETUAL","quoteAsset":"USDT","marginAsset":"USDT"},
				{"symbol":"TSLAUSDT","status":"TRADING","contractType":"TRADIFI_PERPETUAL","quoteAsset":"USDT","marginAsset":"USDT"},
				{"symbol":"DEADUSDT","status":"TRADING","contractType":"PERPETUAL","quoteAsset":"USDT","marginAsset":"USDT"}
			]}`)
		case "/fapi/v1/ticker/24hr":
			fmt.Fprint(w, `[
				{"symbol":"BTCUSDT","lastPrice":"100000","priceChangePercent":"1.5","quoteVolume":"9000000000"},
				{"symbol":"ETHUSDT","lastPrice":"3000","priceChangePercent":"-4.0","quoteVolume":"5000000000"},
				{"symbol":"DOGEUSDT","lastPrice":"0.2","priceChangePercent":"12.0","quoteVolume":"800000000"},
				{"symbol":"TINYUSDT","lastPrice":"1","priceChangePercent":"40.0","quoteVolume":"1000"},
				{"symbol":"TSLAUSDT","lastPrice":"350","priceChangePercent":"2.0","quoteVolume":"7000000000"},
				{"symbol":"BTCUSDT_261225","lastPrice":"101000","priceChangePercent":"1.0","quoteVolume":"50000000"},
				{"symbol":"DEADUSDT","lastPrice":"1","priceChangePercent":"0","quoteVolume":"0"}
			]`)
		case "/futures/data/openInterestHist":
			oiCalls.Add(1)
			if r.URL.Query().Get("period") != "1h" || r.URL.Query().Get("limit") != "5" {
				http.Error(w, "unexpected query", http.StatusBadRequest)
				return
			}
			switch r.URL.Query().Get("symbol") {
			case "BTCUSDT":
				fmt.Fprint(w, `[{"sumOpenInterestValue":"1000"},{"sumOpenInterestValue":"1100"}]`)
			case "ETHUSDT":
				fmt.Fprint(w, `[{"sumOpenInterestValue":"1000"},{"sumOpenInterestValue":"800"}]`)
			case "DOGEUSDT":
				fmt.Fprint(w, `[{"sumOpenInterestValue":"100"},{"sumOpenInterestValue":"150"}]`)
			default:
				http.Error(w, "unknown", http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestRanking(srv *httptest.Server) *Ranking {
	return NewRanking(&Client{baseURL: srv.URL, http: srv.Client()})
}

func TestTickersKeepOnlyTradedUSDTCryptoPerpetuals(t *testing.T) {
	var calls atomic.Int32
	srv := fakeBinance(t, &calls)
	defer srv.Close()

	top, err := newTestRanking(srv).TopVolume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var symbols []string
	for _, ticker := range top {
		symbols = append(symbols, ticker.Symbol)
	}
	want := []string{"BTCUSDT", "ETHUSDT", "DOGEUSDT", "TINYUSDT"}
	if fmt.Sprint(symbols) != fmt.Sprint(want) {
		t.Fatalf("symbols = %v, want %v", symbols, want)
	}
}

func TestPriceMoversRespectMinimumVolume(t *testing.T) {
	var calls atomic.Int32
	srv := fakeBinance(t, &calls)
	defer srv.Close()
	r := newTestRanking(srv)

	gainers, err := r.PriceMovers(context.Background(), true, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if gainers[0].Symbol != "DOGEUSDT" {
		t.Fatalf("top gainer = %s, want DOGEUSDT (TINYUSDT is below the volume floor)", gainers[0].Symbol)
	}
	losers, _ := r.PriceMovers(context.Background(), false, 1_000_000)
	if losers[0].Symbol != "ETHUSDT" {
		t.Fatalf("top loser = %s, want ETHUSDT", losers[0].Symbol)
	}
}

func TestOIChangesRankAndCache(t *testing.T) {
	var calls atomic.Int32
	srv := fakeBinance(t, &calls)
	defer srv.Close()
	r := newTestRanking(srv)

	changes, err := r.OIChanges(context.Background(), "4h", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Symbol != "DOGEUSDT" || changes[2].Symbol != "ETHUSDT" {
		t.Fatalf("unexpected ranking: %+v", changes)
	}
	if changes[0].OIChangePct != 50 || changes[2].OIChangeValue != -200 || changes[0].Window != "4h" {
		t.Fatalf("unexpected change values: %+v", changes)
	}

	first := calls.Load()
	if _, err := r.OIChanges(context.Background(), "4h", 3); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != first {
		t.Fatalf("second call within the cache TTL hit Binance again")
	}

	now := time.Now()
	r.now = func() time.Time { return now.Add(oiCacheTTL + time.Second) }
	if _, err := r.OIChanges(context.Background(), "4h", 3); err != nil {
		t.Fatal(err)
	}
	if calls.Load() == first {
		t.Fatalf("expired cache was not refreshed")
	}
}

func TestWindowHoursDefaultsToFourHours(t *testing.T) {
	for in, want := range map[string]int{"1h": 1, "24h": 24, "": 4, "7d": 4} {
		if _, got := WindowHours(in); got != want {
			t.Fatalf("WindowHours(%q) = %d, want %d", in, got, want)
		}
	}
}
