package forecast

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vergex/internal/marketdata"
	"vergex/internal/provider/binance"
)

func engineFixture(t *testing.T) (*Engine, *marketdata.Series, *[]string, *time.Time) {
	t.Helper()
	marketStore, err := marketdata.Open(filepath.Join(t.TempDir(), "market.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { marketStore.Close() })
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
	version, _ := engine.Store.ActiveVersion(*clock)
	records, _ := engine.Store.RecentRecords(version.ID, series.ID, 10)
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

	records, _ = engine.Store.RecentRecords(version.ID, series.ID, 20)
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

func TestVersionsCannotBeChangedAndActivationSwitchesRecords(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	*clock = time.UnixMilli(int64(WindowBars) * hour)
	engine.Tick(context.Background())
	first, _ := engine.Store.ActiveVersion(*clock)

	changed := DefaultConfig()
	changed.Slope = 9
	second, err := engine.Store.CreateVersion(changed, "steeper", *clock)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("saving should make a new version")
	}
	reread, _ := engine.Store.VersionByID(first.ID)
	if reread.Config.Slope != DefaultConfig().Slope {
		t.Fatal("the old version changed")
	}
	if err := engine.Store.Activate(second.ID); err != nil {
		t.Fatal(err)
	}
	engine.Tick(context.Background())
	own, _ := engine.Store.RecentRecords(second.ID, series.ID, 10)
	old, _ := engine.Store.RecentRecords(first.ID, series.ID, 10)
	if len(own) != 1 || len(old) != 1 {
		t.Fatalf("each version keeps its own records: %d and %d", len(own), len(old))
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
	version, _ := engine.Store.ActiveVersion(*clock)
	for i, record := range records {
		record.VersionID, record.SeriesID, record.Symbol, record.Interval = version.ID, series.ID, "BTCUSDT", "1h"
		record.Status, record.Outcome, record.Direction = StatusVerified, OutcomeHit, DirectionUp
		record.OpenTime = int64(i) * hour
		if _, err := engine.Store.SaveRecord(&record); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.Calibrate(version, "1h"); err != nil {
		t.Fatal(err)
	}
	if curves, _ := engine.Store.Calibrations(version.ID); len(curves["1h"]) != 0 {
		t.Fatal("125 samples should not be enough")
	}
}
