package binance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testLimiter(now *time.Time) *Limiter {
	l := NewLimiter()
	l.now = func() time.Time { return *now }
	return l
}

func TestLimiterStaysWithinTheMinuteBudget(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 10, 0, time.UTC)
	l := testLimiter(&now)
	if wait := l.reserve(BudgetWeight, DefaultWeightBudget); wait != 0 {
		t.Fatalf("first reservation waited %v", wait)
	}
	if wait := l.reserve(BudgetWeight, 1); wait != 50*time.Second {
		t.Fatalf("over budget should wait for the next minute, got %v", wait)
	}
	// Other budgets are counted apart.
	if wait := l.reserve(BudgetData, 1); wait != 0 {
		t.Fatalf("data budget waited %v", wait)
	}
	now = now.Add(time.Minute)
	if wait := l.reserve(BudgetWeight, 1); wait != 0 {
		t.Fatalf("new minute waited %v", wait)
	}
}

func TestLimiterStepsAsideWhenTheIPIsBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 30, 0, time.UTC)
	l := testLimiter(&now)
	header := http.Header{}
	header.Set("X-MBX-USED-WEIGHT-1M", "1300")
	l.Observe(http.StatusOK, header)
	if wait := l.reserve(BudgetWeight, 1); wait != 30*time.Second {
		t.Fatalf("heavy use should pause to the next minute, got %v", wait)
	}
	if status := l.Status(); status.UsedWeight != 1300 || status.PausedUntil.IsZero() {
		t.Fatalf("status = %+v", status)
	}
}

func TestLimiterBacksOffOnRateLimits(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := testLimiter(&now)
	l.Observe(http.StatusTooManyRequests, http.Header{})
	if wait := l.reserve(BudgetWeight, 1); wait != time.Minute {
		t.Fatalf("first 429 = %v", wait)
	}
	now = now.Add(time.Minute)
	l.Observe(http.StatusTooManyRequests, http.Header{})
	if wait := l.reserve(BudgetWeight, 1); wait != 2*time.Minute {
		t.Fatalf("second 429 should double, got %v", wait)
	}

	header := http.Header{}
	header.Set("Retry-After", "7")
	now = now.Add(2 * time.Minute)
	l.Observe(http.StatusTooManyRequests, header)
	if wait := l.reserve(BudgetWeight, 1); wait != 7*time.Second {
		t.Fatalf("Retry-After should win, got %v", wait)
	}

	now = now.Add(time.Minute)
	l.Observe(http.StatusOK, http.Header{})
	l.Observe(http.StatusTooManyRequests, http.Header{})
	if wait := l.reserve(BudgetWeight, 1); wait != time.Minute {
		t.Fatalf("a success should reset the backoff, got %v", wait)
	}
}

func TestLimiterWaitsAtLeastAnHourOnABan(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := testLimiter(&now)
	header := http.Header{}
	header.Set("Retry-After", "120")
	l.Observe(http.StatusTeapot, header)
	if wait := l.reserve(BudgetWeight, 1); wait != time.Hour {
		t.Fatalf("418 pause = %v", wait)
	}
	if !l.Status().Banned {
		t.Fatal("status should report the ban")
	}
}

func TestLimiterDoesNotPauseEveryoneForAServerError(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := testLimiter(&now)
	l.Observe(http.StatusBadGateway, http.Header{})
	if wait := l.reserve(BudgetWeight, 1); wait != 0 {
		t.Fatalf("5xx should not pause, got %v", wait)
	}
}

func TestKlineWeights(t *testing.T) {
	for limit, weight := range map[int]int{99: 1, 100: 2, 499: 2, 500: 5, 1000: 5, 1500: 10} {
		if got := klineWeight(limit); got != weight {
			t.Errorf("limit %d = %d, want %d", limit, got, weight)
		}
	}
}

func TestDataClientParsesKlinesAndReportsRateLimits(t *testing.T) {
	limited := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("X-MBX-USED-WEIGHT-1M", "5")
		_ = json.NewEncoder(w).Encode([][]any{{1000, "1", "3", "0.5", "2", "10", 1999, "20", 7, "4", "8", "0"}})
	}))
	defer srv.Close()
	c := NewDataClient(NewLimiter()).WithBaseURL(srv.URL)

	candles, err := c.Klines(context.Background(), "BTCUSDT", "1h", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := Candle{OpenTime: 1000, Open: 1, High: 3, Low: 0.5, Close: 2, Volume: 10, QuoteVolume: 20, Trades: 7, TakerBuyVolume: 4, TakerBuyQuote: 8}
	if len(candles) != 1 || candles[0] != want {
		t.Fatalf("candles = %+v", candles)
	}
	if c.Limiter().Status().UsedWeight != 5 {
		t.Fatal("weight header not read")
	}

	limited = true
	_, err = c.Klines(context.Background(), "BTCUSDT", "1h", 0, 2)
	var status *StatusError
	if !errors.As(err, &status) || !RateLimited(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestTradingReadsPassABusyIPAndAFullBackgroundBudget(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 30, 0, time.UTC)
	l := testLimiter(&now)
	if wait := l.reserve(BudgetWeight, DefaultWeightBudget); wait != 0 {
		t.Fatalf("background waited %v", wait)
	}
	if wait := l.reserve(BudgetTrading, 2); wait != 0 {
		t.Fatalf("a long catch-up queued a trading read for %v", wait)
	}

	header := http.Header{}
	header.Set("X-MBX-USED-WEIGHT-1M", "1300")
	l.Observe(http.StatusOK, header)
	if wait := l.reserve(BudgetData, 1); wait != 30*time.Second {
		t.Fatalf("background should wait out the minute, waited %v", wait)
	}
	if wait := l.reserve(BudgetTrading, 2); wait != 0 {
		t.Fatalf("heavy use queued a trading read for %v", wait)
	}
	if status := l.Status(); status.PausedUntil.IsZero() || status.Reason == "" {
		t.Fatalf("status = %+v, should report the pause", status)
	}

	// When the exchange itself says to stop, everyone stops.
	l.Observe(http.StatusTooManyRequests, http.Header{})
	if wait := l.reserve(BudgetTrading, 2); wait <= 0 {
		t.Fatal("a trading read went out through a 429")
	}
}
