package forecast

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"vergex/internal/marketdata"
	"vergex/internal/provider/binance"
	"vergex/internal/testdb"
)

func engineFixture(t *testing.T) (*Engine, *marketdata.Series, *[]string, *time.Time) {
	t.Helper()
	marketStore, err := marketdata.NewStore(testdb.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := marketStore.SaveSettings(true, []string{"BTCUSDT"}, 0); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(marketStore.DB())
	if err != nil {
		t.Fatal(err)
	}
	sent := &[]string{}
	engine := NewEngine(store, marketdata.NewService(marketStore), func(_ context.Context, text string) error {
		*sent = append(*sent, text)
		return nil
	})
	clock := time.UnixMilli(1000 * hour)
	engine.now = func() time.Time { return clock }

	series, err := marketStore.EnsureSeries("BTCUSDT", "1h", marketdata.TierWatch, clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	return engine, series, sent, &clock
}

// fill stores candles up to and including the bar and moves the water mark.
func fill(t *testing.T, engine *Engine, series *marketdata.Series, bars int, drift float64) {
	t.Helper()
	klines := walk(bars, drift)
	candles := make([]binance.Candle, len(klines))
	for i, k := range klines {
		candles[i] = binance.Candle{OpenTime: k.OpenTime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume, TakerBuyVolume: k.TakerBuyBaseVolume}
	}
	series.FirstOpenTime = candles[0].OpenTime
	series.LastClosedTime = candles[len(candles)-1].OpenTime
	if err := engine.market.Store.WriteCandlesForTest(series, candles); err != nil {
		t.Fatal(err)
	}
}

func TestEngineRecordsEachCandleOnceAndJudgesItLater(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	*clock = time.UnixMilli(int64(WindowBars) * hour)

	engine.Tick(context.Background())
	engine.Tick(context.Background())
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	records, _ := engine.Store.RecentRecords(strategy.ID, series.ID, 10)
	if len(records) != 1 {
		t.Fatalf("records = %d, want one per candle", len(records))
	}
	record := records[0]
	if record.OpenTime != int64(WindowBars-1)*hour || record.Status != StatusOpen {
		t.Fatalf("record = %+v", record)
	}
	if record.TargetTime != record.OpenTime+int64(record.Horizon)*hour {
		t.Fatalf("target = %d", record.TargetTime)
	}

	// The target candle closes.
	fill(t, engine, series, WindowBars+record.Horizon, 0.3)
	*clock = time.UnixMilli(int64(WindowBars+record.Horizon) * hour)
	engine.Tick(context.Background())

	records, _ = engine.Store.RecentRecords(strategy.ID, series.ID, 20)
	oldest := records[len(records)-1]
	if oldest.OpenTime != record.OpenTime {
		t.Fatalf("oldest = %d", oldest.OpenTime)
	}
	if oldest.Status != StatusVerified || oldest.Outcome == "" || oldest.TargetClose == 0 {
		t.Fatalf("not judged: %+v", oldest)
	}
	if len(records) != record.Horizon+1 {
		t.Fatalf("records = %d, should catch up on the candles in between", len(records))
	}
}

func TestJudgeCallsHitsMissesAndNeutrals(t *testing.T) {
	cases := []struct {
		direction string
		target    float64
		outcome   string
	}{
		{DirectionUp, 110, OutcomeHit},
		{DirectionUp, 90, OutcomeMiss},
		{DirectionDown, 90, OutcomeHit},
		{DirectionDown, 110, OutcomeMiss},
		{DirectionNeutral, 110, OutcomeNeutral},
		{DirectionUp, 100, OutcomeNeutral},
	}
	for _, c := range cases {
		record := Record{Direction: c.direction, Close: 100}
		Judge(&record, c.target, 1)
		if record.Outcome != c.outcome {
			t.Errorf("%s to %.0f = %s, want %s", c.direction, c.target, record.Outcome, c.outcome)
		}
		if want := c.target - 100; math.Abs(record.ActualMovePct-want) > 1e-9 {
			t.Errorf("actual move = %f, want %f", record.ActualMovePct, want)
		}
	}
}

func TestStrategiesCannotBeChangedAndActivationSwitchesRecords(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	*clock = time.UnixMilli(int64(WindowBars) * hour)
	engine.Tick(context.Background())
	first, _ := engine.Store.ActiveStrategy(*clock)

	changed := DefaultConfig()
	changed.Slope = 9
	second, err := engine.Store.CreateStrategy(changed, "steeper", *clock)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("saving should make a new strategy")
	}
	reread, _ := engine.Store.StrategyByID(first.ID)
	if reread.Config.Slope != DefaultConfig().Slope {
		t.Fatal("the old strategy changed")
	}
	if err := engine.Store.Activate(second.ID); err != nil {
		t.Fatal(err)
	}
	engine.Tick(context.Background())
	own, _ := engine.Store.RecentRecords(second.ID, series.ID, 10)
	old, _ := engine.Store.RecentRecords(first.ID, series.ID, 10)
	if len(own) != 1 || len(old) != 1 {
		t.Fatalf("each strategy keeps its own records: %d and %d", len(own), len(old))
	}
	if own[0].ProbFull == old[0].ProbFull {
		t.Fatal("a steeper slope should change the probability")
	}
}

func TestAlertsFireOnceUntilTheScoreComesBack(t *testing.T) {
	engine, _, sent, _ := engineFixture(t)
	engine.Store.LoadSettings(engine.now())
	series := int64(7)
	record := func(prob float64) []Record {
		return []Record{{SeriesID: series, Symbol: "BTCUSDT", Interval: "1h", DisplayProb: prob, Horizon: 4}}
	}
	ctx := context.Background()
	engine.alert(ctx, record(0.75))
	engine.alert(ctx, record(0.80))
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "BTCUSDT 1h: Up, Score 75") {
		t.Fatalf("sent = %v", *sent)
	}
	engine.alert(ctx, record(0.55))
	engine.alert(ctx, record(0.22))
	if len(*sent) != 2 || !strings.Contains((*sent)[1], "Down, Score 22") {
		t.Fatalf("should re-arm inside the threshold: %v", *sent)
	}
	engine.alert(ctx, []Record{{SeriesID: 8, Symbol: "OTHERUSDT", Interval: "1h", DisplayProb: 0.9, Horizon: 4}})
	if len(*sent) != 2 {
		t.Fatalf("coins off the watchlist should not alert: %v", *sent)
	}
}

func TestCalibrationNeedsEnoughNonOverlappingSamples(t *testing.T) {
	records := []Record{}
	for i := 0; i < 500; i++ {
		close := 100.0
		target := 101.0
		if i%2 == 0 {
			target = 99
		}
		records = append(records, Record{SeriesID: 1, OpenTime: int64(i) * hour, Horizon: 4, Close: close, TargetClose: target, ProbFull: 0.5})
	}
	if got := nonOverlapping(records, hour); len(got) != 125 {
		t.Fatalf("kept %d of 500 with a 4-bar horizon, want 125", len(got))
	}
	engine, series, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	for i, record := range records {
		record.StrategyID, record.SeriesID, record.Symbol, record.Interval = strategy.ID, series.ID, "BTCUSDT", "1h"
		record.Status, record.Outcome, record.Direction = StatusVerified, OutcomeHit, DirectionUp
		record.OpenTime = int64(i) * hour
		if _, err := engine.Store.SaveRecord(&record); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.Calibrate(strategy, "1h"); err != nil {
		t.Fatal(err)
	}
	if curves, _ := engine.Store.Calibrations(strategy.ID); len(curves["1h"]) != 0 {
		t.Fatal("125 samples should not be enough")
	}
}

func TestDetailOnlyShowsAFormingCandleTheStreamVouchesFor(t *testing.T) {
	engine, series, _, _ := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	strategy, err := engine.Store.ActiveStrategy(engine.now())
	if err != nil {
		t.Fatal(err)
	}
	detail, err := engine.Detail(strategy, series.Symbol, series.Interval, nil, DetailOptions{Indicators: true})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Forming != nil {
		t.Fatalf("forming = %+v with no stream running", detail.Forming)
	}
	if len(detail.Projection) != detail.Result.Horizon {
		t.Fatalf("projection = %d candles, horizon = %d", len(detail.Projection), detail.Result.Horizon)
	}
	if detail.Indicators == nil || len(detail.Indicators.RSI) == 0 {
		t.Fatalf("indicators = %+v, they were asked for", detail.Indicators)
	}
}

func TestRecordStatsCountHitsAndHowOftenPriceRose(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range []struct {
		direction string
		target    float64
	}{{DirectionUp, 110}, {DirectionUp, 90}, {DirectionDown, 90}, {DirectionNeutral, 110}, {DirectionUp, 100}} {
		record := Record{StrategyID: strategy.ID, SeriesID: series.ID, OpenTime: int64(i) * hour, Symbol: "BTCUSDT", Interval: "1h", Close: 100, Direction: row.direction, TargetTime: int64(i) * hour, Status: StatusOpen}
		Judge(&record, row.target, 1)
		if _, err := engine.Store.SaveRecord(&record); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := engine.Store.RecordStats(strategy.ID, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (RecordStats{Hits: 2, Misses: 1, Moved: 4, Rose: 2}) {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestAStrategyNeedsANameOfItsOwn(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	if _, err := engine.Store.CreateStrategy(DefaultConfig(), "  ", *clock); err == nil {
		t.Fatal("a blank name should be refused")
	}
	if _, err := engine.Store.CreateStrategy(DefaultConfig(), "Fast", *clock); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.CreateStrategy(DefaultConfig(), "Fast", *clock); err == nil {
		t.Fatal("a repeated name should be refused")
	}
}

func TestDeletingAStrategyTakesItsRecordsAndBacktestsButNotTheOneInUse(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	inUse, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteStrategy(inUse.ID, *clock); !errors.Is(err, ErrStrategyInUse) {
		t.Fatalf("err = %v, want it refused", err)
	}

	other, err := engine.Store.CreateStrategy(DefaultConfig(), "Other", *clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Activate(other.ID); err != nil {
		t.Fatal(err)
	}
	engine.Tick(context.Background())
	records, err := engine.Store.RecentRecords(other.ID, series.ID, 5)
	if err != nil || len(records) == 0 {
		t.Fatalf("records = %d, err = %v", len(records), err)
	}
	run, err := engine.Store.CreateBacktest(other.ID, BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}}, *clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Activate(inUse.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteStrategy(other.ID, *clock); !errors.Is(err, ErrStrategyBusy) {
		t.Fatalf("err = %v, a running backtest should hold it", err)
	}
	if err := engine.Store.finishBacktest(run.ID, nil, "", *clock); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteStrategy(other.ID, *clock); err != nil {
		t.Fatal(err)
	}
	if records, _ := engine.Store.RecentRecords(other.ID, series.ID, 5); len(records) != 0 {
		t.Fatalf("records = %d after deleting", len(records))
	}
	if runs, _ := engine.Store.Backtests(10); len(runs) != 0 {
		t.Fatalf("backtests = %d after deleting", len(runs))
	}
	if err := engine.Store.DeleteStrategy(other.ID, *clock); !errors.Is(err, ErrStrategyAbsent) {
		t.Fatalf("err = %v deleting twice", err)
	}
}

func TestOverlayDrawsTheCallOnlyWhenTheCandlesAreCurrent(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing stored yet.
	overlay, err := engine.OverlayFor(strategy, series.Symbol, series.Interval)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Pending != PendingFetching || len(overlay.Projection) != 0 {
		t.Fatalf("overlay = %+v", overlay)
	}

	fill(t, engine, series, WindowBars, 0.3)
	last := int64(WindowBars-1) * hour
	*clock = time.UnixMilli(last + 2*hour)
	engine.Tick(context.Background())
	overlay, err = engine.OverlayFor(strategy, series.Symbol, series.Interval)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Pending != "" || overlay.Direction == "" {
		t.Fatalf("overlay = %+v", overlay)
	}
	if want := DefaultConfig().Horizons["1h"]; len(overlay.Projection) != want {
		t.Fatalf("projection = %d, want %d", len(overlay.Projection), want)
	}
	if first := overlay.Projection[0].Time; first != (last+hour)/1000 {
		t.Fatalf("first projected candle at %d, want the one after the last closed", first)
	}

	// Hours later with no new candles, drawing it would start in the past.
	*clock = time.UnixMilli(last + 10*hour)
	overlay, err = engine.OverlayFor(strategy, series.Symbol, series.Interval)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Pending != PendingFetching || len(overlay.Projection) != 0 {
		t.Fatalf("overlay = %+v", overlay)
	}
}
