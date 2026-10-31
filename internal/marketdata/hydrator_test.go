package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"vergex/internal/provider/binance"
)

const hour = int64(time.Hour / time.Millisecond)

// fakeExchange serves klines from a set of existing open times, like Binance:
// from startTime onwards (or the latest ones), up to limit, including whichever
// candle is still open at the current time.
type fakeExchange struct {
	mu       sync.Mutex
	now      int64
	times    map[int64]bool
	limits   []int
	starts   []int64
	status   int
	requests int
}

func (f *fakeExchange) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	start, _ := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
	f.limits = append(f.limits, limit)
	f.starts = append(f.starts, start)
	var all []int64
	for at := range f.times {
		if at <= f.now {
			all = append(all, at)
		}
	}
	sortInts(all)
	if start > 0 {
		kept := all[:0]
		for _, at := range all {
			if at >= start {
				kept = append(kept, at)
			}
		}
		all = kept
		if len(all) > limit {
			all = all[:limit]
		}
	} else if len(all) > limit {
		all = all[len(all)-limit:]
	}
	rows := make([][]any, 0, len(all))
	for _, at := range all {
		rows = append(rows, []any{at, "100", "110", "90", "105", "5", at + hour - 1, "500", 10, "2", "200", "0"})
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func sortInts(values []int64) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (f *fakeExchange) fill(from, to int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.times == nil {
		f.times = map[int64]bool{}
	}
	for at := from; at <= to; at += hour {
		f.times[at] = true
	}
}

func hydratorFixture(t *testing.T, path string, exchange *fakeExchange) (*Hydrator, *Store, *time.Time) {
	t.Helper()
	srv := httptest.NewServer(exchange)
	t.Cleanup(srv.Close)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	client := binance.NewDataClient(binance.NewLimiter()).WithBaseURL(srv.URL)
	h := NewHydrator(store, client, nil)
	clock := time.UnixMilli(exchange.now + 10*1000)
	h.now = func() time.Time { return clock }
	return h, store, &clock
}

func marketPath(t *testing.T) string { return filepath.Join(t.TempDir(), "market.db") }

func base() int64 { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli() }

func TestBootstrapStoresOnlyClosedCandles(t *testing.T) {
	exchange := &fakeExchange{now: base() + 500*hour + 60*1000}
	exchange.fill(base(), base()+500*hour)
	h, store, _ := hydratorFixture(t, marketPath(t), exchange)
	series, err := store.EnsureSeries("BTCUSDT", "1h", TierWatch, h.nowMillis())
	if err != nil {
		t.Fatal(err)
	}

	h.Sync(context.Background(), series)
	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.LastClosedTime != base()+499*hour {
		t.Fatalf("water mark = %d, want the last closed candle", got.LastClosedTime-base())
	}
	count, _ := store.CandleCount(got.ID)
	if count != bootstrapBars-1 {
		t.Fatalf("stored %d, want %d without the open candle", count, bootstrapBars-1)
	}
	if got.FirstOpenTime != base()+(500-bootstrapBars+1)*hour {
		t.Fatalf("first open = %d", (got.FirstOpenTime-base())/hour)
	}
	if got.NextDueAt != got.LastClosedTime+2*hour+settleDelay.Milliseconds() {
		t.Fatal("next turn should be just after the next close")
	}
}

func TestTailSyncUsesTheCheapestRequestAndResumesAfterRestart(t *testing.T) {
	exchange := &fakeExchange{now: base() + 10*hour + 1000}
	exchange.fill(base(), base()+10*hour)
	path := marketPath(t)
	h, store, clock := hydratorFixture(t, path, exchange)
	series, _ := store.EnsureSeries("BTCUSDT", "1h", TierWatch, h.nowMillis())
	h.Sync(context.Background(), series)
	exchange.limits = nil
	exchange.starts = nil

	// Two more candles close; a new hydrator on the same database carries on.
	exchange.fill(base()+11*hour, base()+12*hour)
	exchange.now = base() + 12*hour + 1000
	*clock = time.UnixMilli(exchange.now)
	again := NewHydrator(store, h.client, nil)
	again.now = h.now
	series, _ = store.FindSeries("BTCUSDT", "1h")
	again.Sync(context.Background(), series)

	if len(exchange.starts) != 1 || exchange.starts[0] != base()+10*hour {
		t.Fatalf("should continue from the water mark, asked from %v", exchange.starts)
	}
	if exchange.limits[0] >= 100 {
		t.Fatalf("limit %d costs more than weight 1", exchange.limits[0])
	}
	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.LastClosedTime != base()+11*hour {
		t.Fatalf("water mark = %d", (got.LastClosedTime-base())/hour)
	}
}

func TestLongCatchUpPagesAtWeightTwo(t *testing.T) {
	exchange := &fakeExchange{now: base() + hour + 1000}
	exchange.fill(base(), base()+hour)
	h, store, clock := hydratorFixture(t, marketPath(t), exchange)
	series, _ := store.EnsureSeries("BTCUSDT", "1h", TierWatch, h.nowMillis())
	h.Sync(context.Background(), series)
	exchange.limits = nil

	exchange.fill(base()+2*hour, base()+1200*hour)
	exchange.now = base() + 1200*hour + 1000
	*clock = time.UnixMilli(exchange.now)
	for i := 0; i < 5; i++ {
		series, _ = store.FindSeries("BTCUSDT", "1h")
		if series.NextDueAt > h.nowMillis() {
			break
		}
		h.Sync(context.Background(), series)
	}
	if exchange.limits[0] != pageLimit {
		t.Fatalf("limit = %d, want %d", exchange.limits[0], pageLimit)
	}
	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.LastClosedTime != base()+1199*hour {
		t.Fatalf("water mark = %d", (got.LastClosedTime-base())/hour)
	}
	if count, _ := store.CandleCount(got.ID); count != 1200 {
		t.Fatalf("stored %d", count)
	}
}

func TestAHoleIsConfirmedThenRecordedAndAccepted(t *testing.T) {
	exchange := &fakeExchange{now: base() + 5*hour + 1000}
	exchange.fill(base(), base()+5*hour)
	h, store, clock := hydratorFixture(t, marketPath(t), exchange)
	series, _ := store.EnsureSeries("BTCUSDT", "1h", TierWatch, h.nowMillis())
	h.Sync(context.Background(), series)

	// Candles 6, 7 and 8 never existed; 5 was still open when last read.
	exchange.fill(base()+9*hour, base()+12*hour)
	exchange.now = base() + 12*hour + 1000
	*clock = time.UnixMilli(exchange.now)
	for i := 0; i < 4; i++ {
		series, _ = store.FindSeries("BTCUSDT", "1h")
		if series.NextDueAt > h.nowMillis() {
			break
		}
		h.Sync(context.Background(), series)
	}
	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.LastClosedTime != base()+11*hour {
		t.Fatalf("water mark = %d", (got.LastClosedTime-base())/hour)
	}
	gaps, _ := store.Gaps(got.ID)
	if len(gaps) != 1 || gaps[0].FromTime != base()+6*hour || gaps[0].ToTime != base()+8*hour {
		t.Fatalf("gaps = %+v", gaps)
	}
	if _, err := store.Window(got, base()+11*hour, 8); err != nil {
		t.Fatalf("a confirmed gap should not break a window: %v", err)
	}
}

func TestAnUnconfirmedHoleBreaksTheWindow(t *testing.T) {
	candles := []Candle{{OpenTime: 0}, {OpenTime: hour}, {OpenTime: 3 * hour}}
	err := CheckContinuous(candles, hour, nil)
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v", err)
	}
	if err := CheckContinuous(candles, hour, []Gap{{FromTime: 2 * hour, ToTime: 2 * hour}}); err != nil {
		t.Fatalf("known gap: %v", err)
	}
}

func TestAServerErrorBacksOffOnlyThatSeries(t *testing.T) {
	exchange := &fakeExchange{now: base() + hour, status: http.StatusBadGateway}
	h, store, _ := hydratorFixture(t, marketPath(t), exchange)
	series, _ := store.EnsureSeries("BTCUSDT", "1h", TierWatch, h.nowMillis())
	h.Sync(context.Background(), series)

	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.FailCount != 1 || got.LastError == "" || got.NextDueAt <= h.nowMillis() {
		t.Fatalf("series = %+v", got)
	}
	if !h.client.Limiter().Status().PausedUntil.IsZero() {
		t.Fatal("a server error must not pause everything")
	}
}

func TestARateLimitPausesEverythingWithoutBlamingTheSeries(t *testing.T) {
	exchange := &fakeExchange{now: base() + hour, status: http.StatusTooManyRequests}
	h, store, _ := hydratorFixture(t, marketPath(t), exchange)
	series, _ := store.EnsureSeries("BTCUSDT", "1h", TierWatch, h.nowMillis())
	h.Sync(context.Background(), series)

	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.FailCount != 0 {
		t.Fatalf("series was blamed: %+v", got)
	}
	if h.client.Limiter().Status().PausedUntil.IsZero() {
		t.Fatal("a 429 should pause everything")
	}
}

func TestImpossibleCandlesAreNotStored(t *testing.T) {
	bad := binance.Candle{OpenTime: 0, Open: 100, High: 90, Low: 95, Close: 100}
	if _, _, err := contiguous([]binance.Candle{bad}, 0, hour); err == nil {
		t.Fatal("expected an error")
	}
}

type fixedRanker []binance.Ticker

func (r fixedRanker) TopVolume(context.Context) ([]binance.Ticker, error) { return r, nil }

func TestRankingEntersAtNAndLeavesPastNPlus30(t *testing.T) {
	tickers := make([]binance.Ticker, 40)
	for i := range tickers {
		tickers[i].Symbol = "C" + strconv.Itoa(i+1)
	}
	got := RankSymbols(tickers, []string{"C25", "C39"}, 5)
	want := map[string]bool{"C1": true, "C2": true, "C3": true, "C4": true, "C5": true, "C25": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, symbol := range got {
		if !want[symbol] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestPlanTracksListsAndKeepsDataOfDroppedSeries(t *testing.T) {
	exchange := &fakeExchange{now: base() + hour}
	h, store, _ := hydratorFixture(t, marketPath(t), exchange)
	h.ranker = fixedRanker{{Symbol: "AAAUSDT"}, {Symbol: "BTCUSDT"}}
	if err := store.SaveSettings(true, []string{"BTCUSDT"}, 2); err != nil {
		t.Fatal(err)
	}
	if enabled, err := h.Plan(context.Background()); err != nil || !enabled {
		t.Fatalf("plan: %v %v", enabled, err)
	}
	all, _ := store.ListSeries()
	counts := map[int]int{}
	for _, series := range all {
		counts[series.Tier]++
	}
	// BTC is on the watchlist (4 intervals) and AAA comes from the ranking (3).
	if counts[TierWatch] != 4 || counts[TierTop] != 3 {
		t.Fatalf("tiers = %v", counts)
	}

	if err := store.SaveSettings(true, nil, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Plan(context.Background()); err != nil {
		t.Fatal(err)
	}
	all, _ = store.ListSeries()
	if len(all) != 7 {
		t.Fatalf("series should be kept, have %d", len(all))
	}
	for _, series := range all {
		if series.Status != StatusInactive {
			t.Fatalf("%s %s still active", series.Symbol, series.Interval)
		}
	}

	if err := store.SaveSettings(false, []string{"BTCUSDT"}, 0); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := h.Plan(context.Background()); enabled {
		t.Fatal("collection should be off")
	}
}

func TestWorkOrderPutsTheWaitingUserFirst(t *testing.T) {
	exchange := &fakeExchange{now: base() + 100*hour}
	h, store, _ := hydratorFixture(t, marketPath(t), exchange)
	now := h.nowMillis()
	newSeries, _ := store.EnsureSeries("NEWUSDT", "1h", TierTop, now)
	watch, _ := store.EnsureSeries("BTCUSDT", "1h", TierWatch, now)
	watch.LastClosedTime = base() + 98*hour
	watch.Priority = seriesPriority(watch, now)
	_ = store.saveSeries(watch)

	first, _ := store.NextDue(now)
	if first.ID != watch.ID {
		t.Fatalf("the watchlist's tail should beat a new series, got %s", first.Symbol)
	}
	if err := store.markOpened(newSeries, now); err != nil {
		t.Fatal(err)
	}
	first, _ = store.NextDue(now)
	if first.ID != newSeries.ID {
		t.Fatalf("what the user opened should go first, got %s", first.Symbol)
	}
}
