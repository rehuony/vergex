package forecast

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"vergex/internal/database/testdb"
	"vergex/internal/market/feed"
	"vergex/internal/venue/binance"
)

func engineFixture(t *testing.T) (*Engine, *feed.Series, *[]Alert, *time.Time) {
	t.Helper()
	marketStore := feed.NewStore(testdb.Open(t))
	if err := marketStore.SaveSettings(true, []string{"BTCUSDT"}, 0); err != nil {
		t.Fatal(err)
	}
	store := NewStore(marketStore.DB())
	sent := &[]Alert{}
	engine := NewEngine(store, feed.NewService(marketStore), func(_ context.Context, alert Alert) error {
		*sent = append(*sent, alert)
		return nil
	})
	clock := time.UnixMilli(1000 * hour)
	engine.now = func() time.Time { return clock }

	series, err := marketStore.EnsureSeries("BTCUSDT", "1h", feed.TierWatch, clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	return engine, series, sent, &clock
}

// fill stores candles up to and including the bar and moves the water mark.
func fill(t *testing.T, engine *Engine, series *feed.Series, bars int, drift float64) {
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
	*clock = time.UnixMilli(int64(WindowBars+1) * hour)

	engine.Tick(context.Background())
	engine.Tick(context.Background())
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	records, _ := engine.Store.RecentSignals(strategy.ID, series.ID, 10)
	if len(records) != 1 {
		t.Fatalf("records = %d, want one per candle", len(records))
	}
	record := records[0]
	if record.OpenTime != int64(WindowBars)*hour || record.Status != StatusOpen {
		t.Fatalf("record = %+v", record)
	}
	if record.TargetTime != record.OpenTime+int64(record.Horizon)*hour {
		t.Fatalf("target = %d", record.TargetTime)
	}

	// The target candle closes.
	fill(t, engine, series, WindowBars+record.Horizon, 0.3)
	*clock = time.UnixMilli(int64(WindowBars+1+record.Horizon) * hour)
	engine.Tick(context.Background())

	records, _ = engine.Store.RecentSignals(strategy.ID, series.ID, 20)
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

func TestDueSignalsLeaveOutWhatCannotBeJudged(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	stopped, err := engine.market.Store.EnsureSeries("ETHUSDT", "1h", feed.TierWatch, clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	fill(t, engine, stopped, WindowBars-10, 0.3)
	strategy, _ := engine.Store.ActiveStrategy(*clock)

	save := func(seriesID, target int64) {
		t.Helper()
		_, err := engine.Store.SaveSignal(&Signal{StrategyID: strategy.ID, SeriesID: seriesID, OpenTime: target - 4*hour, TargetTime: target, Horizon: 4, Close: 100, Direction: DirectionUp, Regime: RegimeTrending, Status: StatusOpen})
		if err != nil {
			t.Fatal(err)
		}
	}
	// The stopped series' signal is the oldest, and its target candle never closed.
	save(stopped.ID, int64(WindowBars-5)*hour)
	// The target candle of this one is still forming.
	save(series.ID, int64(WindowBars+1)*hour)
	// This one can be judged.
	ready := int64(WindowBars) * hour
	save(series.ID, ready)

	due, err := engine.Store.DueSignals(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].SeriesID != series.ID || due[0].TargetTime != ready {
		t.Fatalf("due = %+v, want only the one that can be judged", due)
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
		record := Signal{Direction: c.direction, Close: 100}
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
	*clock = time.UnixMilli(int64(WindowBars+1) * hour)
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
	own, _ := engine.Store.RecentSignals(second.ID, series.ID, 10)
	old, _ := engine.Store.RecentSignals(first.ID, series.ID, 10)
	if len(own) != 1 || len(old) != 1 {
		t.Fatalf("each strategy keeps its own records: %d and %d", len(own), len(old))
	}
	if own[0].ProbFull == old[0].ProbFull {
		t.Fatal("a steeper slope should change the probability")
	}
}

func TestAnEngineChangeRetiresTheStrategyAndACopyTakesOver(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	old, err := engine.Store.CreateStrategy(DefaultConfig(), "Fast", *clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Activate(old.ID); err != nil {
		t.Fatal(err)
	}
	// Nothing to do while the engine has not changed.
	if err := engine.Store.carryForward(*clock); err != nil {
		t.Fatal(err)
	}
	if active, _ := engine.Store.ActiveStrategy(*clock); active.ID != old.ID {
		t.Fatalf("active = %s, want the strategy left alone", active.Name)
	}

	if err := engine.Store.db.Model(&Strategy{}).Where("id = ?", old.ID).Update("engine_version", EngineVersion-1).Error; err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.carryForward(*clock); err != nil {
		t.Fatal(err)
	}
	active, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID == old.ID || active.Retired || active.Name != fmt.Sprintf("Fast (%d)", EngineVersion) {
		t.Fatalf("active = %+v, want a copy under the running engine", active)
	}
	if active.Config.Slope != old.Config.Slope {
		t.Fatal("the copy should keep the rules")
	}

	retired, _ := engine.Store.StrategyByID(old.ID)
	if !retired.Retired {
		t.Fatal("the old strategy should read as retired")
	}
	if err := engine.Store.Activate(old.ID); !errors.Is(err, ErrStrategyRetired) {
		t.Fatalf("activate = %v, want it refused", err)
	}
	if _, err := engine.StartBacktest(context.Background(), old.ID, "", BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}, To: 1}); !errors.Is(err, ErrStrategyRetired) {
		t.Fatalf("backtest = %v, want it refused", err)
	}
}

func TestAlertsFireOnceUntilTheScoreComesBack(t *testing.T) {
	engine, watched, sent, clock := engineFixture(t)
	engine.Store.LoadSettings(engine.now())
	series := watched.ID
	justClosed := clock.UnixMilli() - hour
	record := func(direction string, prob float64) []Signal {
		return []Signal{{SeriesID: series, Symbol: "BTCUSDT", Interval: "1h", OpenTime: justClosed, Direction: direction, DisplayProb: prob, Horizon: 4}}
	}
	ctx := context.Background()
	engine.alert(ctx, "Default", record(DirectionUp, 0.75))
	engine.alert(ctx, "Default", record(DirectionUp, 0.80))
	if len(*sent) != 1 || len((*sent)[0].Signals) != 1 || (*sent)[0].Signals[0].DisplayProb != 0.75 {
		t.Fatalf("sent = %v", *sent)
	}
	engine.alert(ctx, "Default", record(DirectionUp, 0.55))
	engine.alert(ctx, "Default", record(DirectionDown, 0.22))
	if len(*sent) != 2 || (*sent)[1].Signals[0].Direction != DirectionDown || (*sent)[1].Signals[0].DisplayProb != 0.22 {
		t.Fatalf("should re-arm inside the threshold: %v", *sent)
	}
	engine.alert(ctx, "Default", []Signal{{SeriesID: 8, Symbol: "OTHERUSDT", Interval: "1h", OpenTime: justClosed, Direction: DirectionUp, DisplayProb: 0.9, Horizon: 4}})
	if len(*sent) != 2 {
		t.Fatalf("coins off the watchlist should not alert: %v", *sent)
	}
}

func TestAlertsFollowTheCallAndOnlyTheNewestCandle(t *testing.T) {
	engine, watched, sent, clock := engineFixture(t)
	engine.Store.LoadSettings(engine.now())
	ctx := context.Background()
	justClosed := clock.UnixMilli() - hour
	signal := func(openTime int64, direction string, prob float64) Signal {
		return Signal{SeriesID: watched.ID, Symbol: "BTCUSDT", Interval: "1h", OpenTime: openTime, Direction: direction, DisplayProb: prob, Horizon: 4}
	}

	// A neutral call has no side, and a curve can put a score against the call.
	engine.alert(ctx, "Default", []Signal{signal(justClosed, DirectionNeutral, 0.9)})
	engine.alert(ctx, "Default", []Signal{signal(justClosed, DirectionUp, 0.2)})
	if len(*sent) != 0 {
		t.Fatalf("sent = %v", *sent)
	}

	// Catching up: old candles say nothing, and the newest speaks once.
	engine.alert(ctx, "Default", []Signal{signal(justClosed-2*hour, DirectionUp, 0.9), signal(justClosed-hour, DirectionUp, 0.85), signal(justClosed, DirectionUp, 0.8)})
	if len(*sent) != 1 || len((*sent)[0].Signals) != 1 || (*sent)[0].Signals[0].DisplayProb != 0.8 {
		t.Fatalf("sent = %v, want one line about the newest candle", *sent)
	}

	// A newest candle that is itself old stays silent, and does not arm the series.
	if err := engine.Store.setAlertArm(watched.ID, false); err != nil {
		t.Fatal(err)
	}
	engine.alert(ctx, "Default", []Signal{signal(justClosed-3*hour, DirectionUp, 0.9)})
	if len(*sent) != 1 || engine.Store.alertArm(watched.ID) {
		t.Fatalf("an old candle alerted: %v", *sent)
	}
}

func TestAlertsGoByIntervalAndCarryTheirStrategyAndLanguage(t *testing.T) {
	engine, watched, sent, clock := engineFixture(t)
	settings, err := engine.Store.LoadSettings(engine.now())
	if err != nil || settings.AlertThreshold != 56 || !slices.Equal(settings.AlertIntervals, []string{"1h", "4h"}) {
		t.Fatalf("settings = %+v, err = %v", settings, err)
	}
	short, err := feed.NewStore(engine.Store.db).EnsureSeries("BTCUSDT", "15m", feed.TierWatch, clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	justClosed := clock.UnixMilli() - hour
	quarter := clock.UnixMilli() - hour/4
	signal := func(seriesID int64, interval string, openTime int64) Signal {
		return Signal{SeriesID: seriesID, Symbol: "BTCUSDT", Interval: interval, OpenTime: openTime, Direction: DirectionUp, DisplayProb: 0.8, Horizon: 4}
	}

	// 15m is left out until asked for, but still comes back inside the threshold.
	engine.alert(ctx, "Default", []Signal{signal(short.ID, "15m", quarter)})
	if len(*sent) != 0 {
		t.Fatalf("15m alerted: %v", *sent)
	}
	if err := engine.Store.SetAlerts(70, []string{"15m", "1h"}); err != nil {
		t.Fatal(err)
	}
	// One round is one message, however many calls it holds.
	engine.alert(ctx, "Momentum", []Signal{signal(short.ID, "15m", quarter), signal(watched.ID, "1h", justClosed)})
	if len(*sent) != 1 || (*sent)[0].Strategy != "Momentum" || len((*sent)[0].Signals) != 2 ||
		(*sent)[0].Signals[0].Interval != "15m" || (*sent)[0].Signals[0].Span() != time.Hour ||
		(*sent)[0].Signals[1].Interval != "1h" || (*sent)[0].Signals[1].Span() != 4*time.Hour {
		t.Fatalf("sent = %+v", *sent)
	}

	if err := engine.Store.SetAlerts(70, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.setAlertArm(watched.ID, false); err != nil {
		t.Fatal(err)
	}
	engine.alert(ctx, "Default", []Signal{signal(watched.ID, "1h", justClosed)})
	if len(*sent) != 1 || engine.Store.alertArm(watched.ID) {
		t.Fatalf("no intervals should alert none, and arm none: %v", *sent)
	}

	if err := engine.Store.SetAlerts(70, []string{"1h"}); err != nil {
		t.Fatal(err)
	}
	down := signal(watched.ID, "1h", justClosed)
	down.Direction, down.DisplayProb, down.Calibrated, down.Horizon = DirectionDown, 0.2, true, 72
	engine.alert(ctx, "默认", []Signal{down})
	if len(*sent) != 2 || (*sent)[1].Strategy != "默认" || (*sent)[1].Signals[0].Span() != 72*time.Hour {
		t.Fatalf("sent = %+v", (*sent)[1:])
	}
}

func TestACurveThatStopsEarningItsPlaceIsRemoved(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	if err := engine.Store.saveCalibration(Calibration{StrategyID: strategy.ID, Interval: "1h", Knots: encodeKnots([]Knot{{X: 0.5, Y: 0.5}}), BuiltAt: clock.UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Calibrate(strategy, "1h"); err != nil {
		t.Fatal(err)
	}
	if curves, _ := engine.Store.Calibrations(strategy.ID); len(curves["1h"]) != 0 {
		t.Fatalf("curve = %v, want it gone with no points behind it", curves["1h"])
	}
}

func TestCalibrationNeedsEnoughNonOverlappingSamples(t *testing.T) {
	records := []Signal{}
	for i := 0; i < 500; i++ {
		close := 100.0
		target := 101.0
		if i%2 == 0 {
			target = 99
		}
		records = append(records, Signal{SeriesID: 1, OpenTime: int64(i) * hour, Horizon: 4, Close: close, TargetClose: target, ProbFull: 0.5})
	}
	if got := nonOverlapping(records, hour); len(got) != 125 {
		t.Fatalf("kept %d of 500 with a 4-bar horizon, want 125", len(got))
	}
	engine, series, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	for i, record := range records {
		record.StrategyID, record.SeriesID, record.Interval = strategy.ID, series.ID, "1h"
		record.Direction, record.OpenTime = DirectionUp, int64(i+1)*hour
		judgeAndSave(t, engine, &record, record.TargetClose)
	}
	points, err := engine.Store.IndependentPoints(strategy.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 125 {
		t.Fatalf("stored %d independent signals, want the same 125", len(points))
	}
	if err := engine.Calibrate(strategy, "1h"); err != nil {
		t.Fatal(err)
	}
	if curves, _ := engine.Store.Calibrations(strategy.ID); len(curves["1h"]) != 0 {
		t.Fatal("125 samples should not be enough")
	}
}

// nonOverlapping is what independent signals should be: the ones whose forecast
// windows do not overlap within a series, and that had a real move, walking from its
// start. Signals must be ordered by series and time.
func nonOverlapping(records []Signal, step int64) []CalibrationPoint {
	var points []CalibrationPoint
	var series, next int64
	for _, record := range records {
		if record.SeriesID != series {
			series, next = record.SeriesID, 0
		}
		if record.OpenTime < next || record.TargetClose == record.Close {
			continue
		}
		next = record.OpenTime + int64(record.Horizon)*step
		points = append(points, CalibrationPoint{Prob: record.ProbFull, Up: record.TargetClose > record.Close})
	}
	return points
}

// judgeAndSave records a forecast as open, then judges it the way the engine does.
func judgeAndSave(t *testing.T, engine *Engine, record *Signal, targetClose float64) {
	t.Helper()
	record.Status = StatusOpen
	record.TargetTime = record.OpenTime + int64(record.Horizon)*hour
	record.TargetClose, record.Outcome = 0, ""
	if record.Regime == "" {
		record.Regime = RegimeRanging
	}
	if _, err := engine.Store.SaveSignal(record); err != nil {
		t.Fatal(err)
	}
	Judge(record, targetClose, record.TargetTime)
	if err := engine.Store.saveVerification(record, hour); err != nil {
		t.Fatal(err)
	}
}

func TestStatsAgreeWithTheSignalsTheyAddUp(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	var signals []Signal
	for i := range 240 {
		direction := []string{DirectionUp, DirectionDown, DirectionNeutral}[i%3]
		target := []float64{110, 90, 100, 101, 99}[i%5]
		record := Signal{StrategyID: strategy.ID, SeriesID: series.ID, Interval: "1h", OpenTime: int64(i+1) * hour, Horizon: 4, Close: 100, Direction: direction, Regime: []string{RegimeTrending, RegimeRanging}[i%2], Confidence: i % 101}
		judgeAndSave(t, engine, &record, target)
		signals = append(signals, record)
	}

	var want outcomeTotals
	var independentHits, independentMisses int64
	for _, signal := range signals {
		want.judged++
		if signal.Independent {
			want.moved++
			if signal.TargetClose > signal.Close {
				want.rose++
			}
			switch signal.Outcome {
			case OutcomeHit:
				want.hits++
				independentHits++
			case OutcomeMiss:
				want.misses++
				independentMisses++
			}
			if signal.Outcome == OutcomeHit || signal.Outcome == OutcomeMiss {
				if signal.Direction == DirectionUp {
					want.upCalls++
					want.signedMove += signal.ActualMovePct
				} else {
					want.downCalls++
					want.signedMove -= signal.ActualMovePct
				}
			}
		}
	}
	got, err := engine.Store.SignalStats(strategy.ID, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	wanted := want.stats()
	if got.Judged != wanted.Judged || got.Samples != wanted.Samples || got.Hits != wanted.Hits || got.Misses != wanted.Misses {
		t.Fatalf("stats = %+v, counted %+v", got, wanted)
	}
	if math.Abs(*got.ChanceRate-*wanted.ChanceRate) > 1e-9 || math.Abs(*got.AvgReturnAfterFee-*wanted.AvgReturnAfterFee) > 1e-9 {
		t.Fatalf("rates = %v, %v, counted %v, %v", *got.ChanceRate, *got.AvgReturnAfterFee, *wanted.ChanceRate, *wanted.AvgReturnAfterFee)
	}
	if independent := nonOverlapping(signals, hour); int64(len(independent)) != want.moved {
		t.Fatalf("marked %d independent, nonOverlapping keeps %d", want.moved, len(independent))
	}

	performance, err := engine.Store.Performance(strategy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if performance.Overall.Samples != wanted.Samples || performance.ByInterval["1h"].Samples != wanted.Samples {
		t.Fatalf("performance = %+v, want %d samples", performance, wanted.Samples)
	}
	base := float64(want.rose) / float64(want.moved)
	if rate := performance.ByDirection[DirectionUp].ChanceRate; rate == nil || math.Abs(*rate-base) > 1e-9 {
		t.Fatalf("up calls by chance = %v, want how often price rose overall, %f", rate, base)
	}
	if rate := performance.ByDirection[DirectionDown].ChanceRate; rate == nil || math.Abs(*rate-(1-base)) > 1e-9 {
		t.Fatalf("down calls by chance = %v, want how often price fell overall, %f", rate, 1-base)
	}
	var byRegime, byConfidence int64
	for _, stats := range performance.ByRegime {
		byRegime += stats.Samples
	}
	for _, stats := range performance.ByConfidence {
		byConfidence += stats.Samples
	}
	if byRegime != wanted.Samples || byConfidence != wanted.Samples {
		t.Fatalf("breakdowns hold %d and %d of %d samples", byRegime, byConfidence, wanted.Samples)
	}
	if len(performance.Calibration) != len(Intervals) || performance.Calibration[1].Interval != "1h" || performance.Calibration[1].Needed != MinCalibrationMoments || performance.Calibration[1].Moments == 0 || performance.Calibration[1].Active {
		t.Fatalf("calibration = %+v", performance.Calibration)
	}
}

func TestStatsGiveTheReturnOfActingOnACall(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	record := Signal{StrategyID: strategy.ID, SeriesID: series.ID, Interval: "1h", OpenTime: hour, Horizon: 4, Close: 100, Direction: DirectionUp, Regime: RegimeTrending, Confidence: 30}
	judgeAndSave(t, engine, &record, 90)
	got, err := engine.Store.SignalStats(strategy.ID, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Samples != 1 || got.Misses != 1 || got.AvgReturnAfterFee == nil || math.Abs(*got.AvgReturnAfterFee-(-10-RoundTripFeePct)) > 1e-9 {
		t.Fatalf("stats = %+v, want one miss of 10%%", got)
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
	if detail.Latest == nil || detail.Latest.Horizon != detail.Result.Horizon || detail.Latest.ExpectedMovePct <= 0 {
		t.Fatalf("latest = %+v, want what the band is drawn from", detail.Latest)
	}
	if detail.Indicators == nil || len(detail.Indicators.RSI) == 0 {
		t.Fatalf("indicators = %+v, they were asked for", detail.Indicators)
	}
}

func TestSignalStatsCountOnlyWhatDoesNotOverlap(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range []struct {
		direction string
		target    float64
	}{{DirectionUp, 110}, {DirectionUp, 90}, {DirectionDown, 90}, {DirectionNeutral, 110}, {DirectionUp, 100}} {
		record := Signal{StrategyID: strategy.ID, SeriesID: series.ID, OpenTime: int64(i+1) * hour, Interval: "1h", Horizon: 4, Close: 100, Direction: row.direction, Regime: RegimeRanging, Status: StatusOpen}
		judgeAndSave(t, engine, &record, row.target)
	}
	stats, err := engine.Store.SignalStats(strategy.ID, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Only the first signal is independent: the rest fall inside its window.
	if stats.Judged != 5 || stats.Samples != 1 || stats.Hits != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

// A hundred coins over one stretch of time are one moment, not a hundred answers.
func TestPerformanceCountsEachStretchOfTimeOnce(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	other, err := feed.NewStore(engine.Store.db).EnsureSeries("ETHUSDT", "1h", feed.TierWatch, clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	call := func(seriesID, openTime int64, direction, regime string, confidence int, target float64) {
		t.Helper()
		judgeAndSave(t, engine, &Signal{
			StrategyID: strategy.ID, SeriesID: seriesID, OpenTime: openTime, Interval: "1h", Horizon: 4, Close: 100,
			Direction: direction, Regime: regime, Confidence: confidence,
		}, target)
	}
	// Two coins in the same stretch, then one in the next, and a neutral reading in a third.
	call(series.ID, 8*hour, DirectionUp, RegimeTrending, 70, 110)
	call(other.ID, 9*hour, DirectionDown, RegimeRanging, 30, 90)
	call(series.ID, 12*hour, DirectionUp, RegimeTrending, 70, 90)
	call(series.ID, 16*hour, DirectionNeutral, RegimeRanging, 30, 110)

	performance, err := engine.Store.Performance(strategy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if performance.Overall.Samples != 3 || performance.Overall.Moments != 2 {
		t.Fatalf("overall = %+v", performance.Overall)
	}
	if got := performance.ByInterval["1h"]; got.Samples != 3 || got.Moments != 2 {
		t.Fatalf("1h = %+v", got)
	}
	if got := performance.ByDirection[DirectionUp]; got.Samples != 2 || got.Moments != 2 {
		t.Fatalf("up = %+v", got)
	}
	if got := performance.ByDirection[DirectionDown]; got.Samples != 1 || got.Moments != 1 {
		t.Fatalf("down = %+v", got)
	}
	if got := performance.ByRegime[RegimeRanging]; got.Samples != 1 || got.Moments != 1 {
		t.Fatalf("ranging = %+v", got)
	}
	if got := performance.ByConfidence["3"]; got.Samples != 2 || got.Moments != 2 {
		t.Fatalf("confidence 60-79 = %+v", got)
	}
	coin, err := engine.Store.SignalStats(strategy.ID, series.ID)
	if err != nil || coin.Samples != 2 || coin.Moments != 2 {
		t.Fatalf("one coin = %+v, err = %v", coin, err)
	}
}

func TestLatestLookupsFindEachSeriesNewestSignal(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	other, _ := engine.Store.CreateStrategy(DefaultConfig(), "Other", *clock)
	save := func(strategyID string, openTime int64) {
		t.Helper()
		if _, err := engine.Store.SaveSignal(&Signal{StrategyID: strategyID, SeriesID: series.ID, OpenTime: openTime, TargetTime: openTime + hour, Close: 100, Direction: DirectionUp, Regime: RegimeTrending, Status: StatusOpen}); err != nil {
			t.Fatal(err)
		}
	}
	save(strategy.ID, hour)
	save(strategy.ID, 3*hour)
	save(strategy.ID, 2*hour)
	save(other.ID, 9*hour)

	latest, err := engine.Store.LatestOpenTimes(strategy.ID)
	if err != nil || latest[series.ID] != 3*hour || len(latest) != 1 {
		t.Fatalf("latest = %v, err = %v", latest, err)
	}
	signals, err := engine.Store.LatestSignals(strategy.ID, "1h")
	if err != nil || len(signals) != 1 || signals[0].OpenTime != 3*hour || signals[0].Symbol != "BTCUSDT" || signals[0].Interval != "1h" {
		t.Fatalf("signals = %+v, err = %v", signals, err)
	}
	if none, err := engine.Store.LatestSignals(strategy.ID, "4h"); err != nil || len(none) != 0 {
		t.Fatalf("4h = %+v, err = %v", none, err)
	}
	if coins, err := engine.Store.Coins(strategy.ID); err != nil || len(coins) != 1 {
		t.Fatalf("coins = %v, err = %v", coins, err)
	}
}

func TestBacktestListsSummariesAndAFailedRunHasNoReport(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	options := BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}}
	finished, err := engine.Store.CreateBacktest(strategy.ID, "", options, *clock)
	if err != nil {
		t.Fatal(err)
	}
	report := Report{Overall: Metrics{Samples: 7}, ByInterval: map[string]Metrics{"1h": {Samples: 7}}}
	if err := engine.Store.finishBacktest(finished.ID, &report, "", *clock); err != nil {
		t.Fatal(err)
	}
	failed, err := engine.Store.CreateBacktest(strategy.ID, "", options, *clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.finishBacktest(failed.ID, nil, BacktestCanceled, *clock); err != nil {
		t.Fatal(err)
	}

	runs, err := engine.Store.Backtests(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[1].Summary == nil || runs[1].Summary.Samples != 7 || runs[1].Report != nil {
		t.Fatalf("runs = %+v, want the summary and not the report", runs)
	}
	if runs[0].Summary != nil || runs[0].Error != BacktestCanceled || runs[0].Status != BacktestFailed {
		t.Fatalf("failed run = %+v", runs[0])
	}
	whole, err := engine.Store.BacktestByID(finished.ID)
	if err != nil || whole.Report == nil || whole.Report.ByInterval["1h"].Samples != 7 {
		t.Fatalf("run = %+v, err = %v", whole, err)
	}
}

func TestABacktestCanBeCanceled(t *testing.T) {
	engine, _, _, _ := engineFixture(t)
	if engine.CancelBacktest(1) {
		t.Fatal("nothing is running to cancel")
	}
	canceled := false
	engine.mu.Lock()
	engine.backtestRun, engine.cancelBacktest = 5, func() { canceled = true }
	engine.mu.Unlock()
	if engine.CancelBacktest(4) || canceled {
		t.Fatal("another run should be left alone")
	}
	if !engine.CancelBacktest(5) || !canceled {
		t.Fatal("the running one should stop")
	}
}

func TestAStrategyCanBeRenamedButKeepsItsRulesAndARealName(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	first, err := engine.Store.CreateStrategy(DefaultConfig(), "First", *clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.CreateStrategy(DefaultConfig(), "Second", *clock); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.RenameStrategy(first.ID, "  Renamed  "); err != nil {
		t.Fatal(err)
	}
	reread, _ := engine.Store.StrategyByID(first.ID)
	if reread.Name != "Renamed" || reread.Config.Slope != first.Config.Slope {
		t.Fatalf("strategy = %+v", reread)
	}
	if err := engine.Store.RenameStrategy(first.ID, "Second"); !errors.Is(err, ErrStrategyNameTaken) {
		t.Fatalf("err = %v, a name must stay its own", err)
	}
	if err := engine.Store.RenameStrategy(first.ID, " "); !errors.Is(err, ErrStrategyName) {
		t.Fatalf("err = %v, a blank name should be refused", err)
	}
	if err := engine.Store.RenameStrategy(first.ID, strings.Repeat("名", maxNameLength+1)); !errors.Is(err, ErrStrategyName) {
		t.Fatalf("err = %v, a long name should be refused", err)
	}
	if err := engine.Store.RenameStrategy("nonsense", "Anything"); !errors.Is(err, ErrStrategyAbsent) {
		t.Fatalf("err = %v", err)
	}
}

func TestABacktestCanBeNamedAndDeletedOnceItIsNotRunning(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	options := BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}}
	run, err := engine.Store.CreateBacktest(strategy.ID, " Tuned on 2025 ", options, *clock)
	if err != nil || run.Name != "Tuned on 2025" {
		t.Fatalf("run = %+v, err = %v", run, err)
	}
	if err := engine.Store.RenameBacktest(run.ID, ""); err != nil {
		t.Fatal(err)
	}
	listed, _ := engine.Store.Backtests(10)
	if len(listed) != 1 || listed[0].Name != "" {
		t.Fatalf("runs = %+v, an empty name goes back to the strategy's", listed)
	}
	if err := engine.Store.RenameBacktest(run.ID, "Holdout"); err != nil {
		t.Fatal(err)
	}
	listed, _ = engine.Store.Backtests(10)
	if listed[0].Name != "Holdout" {
		t.Fatalf("name = %q", listed[0].Name)
	}
	if err := engine.Store.RenameBacktest(run.ID+1, "Other"); !errors.Is(err, ErrBacktestAbsent) {
		t.Fatalf("err = %v", err)
	}
	if err := engine.Store.DeleteBacktest(run.ID); !errors.Is(err, ErrBacktestRunning) {
		t.Fatalf("err = %v, a running backtest should stay", err)
	}
	if err := engine.Store.finishBacktest(run.ID, nil, "", *clock); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteBacktest(run.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteBacktest(run.ID); !errors.Is(err, ErrBacktestAbsent) {
		t.Fatalf("err = %v deleting twice", err)
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

func TestANewStrategyHasToBeAbleToCallBothMarketsAndNotLookTooFar(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	oneSided := DefaultConfig()
	oneSided.Factors = map[string]FactorConfig{FactorTrend: {Enabled: true, Trending: 1}}
	if _, err := engine.Store.CreateStrategy(oneSided, "Trend only", *clock); err == nil || !strings.Contains(err.Error(), "both trending and ranging") {
		t.Fatalf("err = %v", err)
	}
	// A weight on a factor that is off is no weight.
	oneSided.Factors[FactorMomentum] = FactorConfig{Ranging: 1}
	if _, err := engine.Store.CreateStrategy(oneSided, "Trend only", *clock); err == nil {
		t.Fatal("saved a strategy whose only ranging weight is on a factor that is off")
	}
	oneSided.Factors[FactorMomentum] = FactorConfig{Enabled: true, Ranging: 1}
	if _, err := engine.Store.CreateStrategy(oneSided, "Both", *clock); err != nil {
		t.Fatal(err)
	}

	for interval, limit := range HorizonLimits {
		config := DefaultConfig()
		config.Horizons[interval] = limit
		if _, err := engine.Store.CreateStrategy(config, "At "+interval, *clock); err != nil {
			t.Fatalf("at the limit for %s: %v", interval, err)
		}
		config.Horizons[interval] = limit + 1
		if _, err := engine.Store.CreateStrategy(config, "Over "+interval, *clock); err == nil {
			t.Fatalf("over the limit for %s", interval)
		}
	}

	// A strategy saved before these rules is carried forward as it is.
	old := DefaultConfig()
	old.Horizons["1d"] = 96
	old.Factors = map[string]FactorConfig{FactorTrend: {Enabled: true, Trending: 1}}
	if _, err := engine.Store.createStrategy(engine.Store.db, old, "Saved before", false, *clock); err != nil {
		t.Fatal(err)
	}
}

func TestTheLastStrategyCannotBeDeleted(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	only, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Deactivate(only.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteStrategy(only.ID, *clock); !errors.Is(err, ErrLastStrategy) {
		t.Fatalf("err = %v", err)
	}
	if _, err := engine.Store.ActiveStrategy(*clock); !errors.Is(err, ErrNoStrategyInUse) {
		t.Fatalf("err = %v, want it still stopped rather than a new default", err)
	}
	other, err := engine.Store.CreateStrategy(DefaultConfig(), "Other", *clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.DeleteStrategy(other.ID, *clock); err != nil {
		t.Fatal(err)
	}
}

func TestStoppingTheStrategyInUseStopsForecastsUntilOneIsUsedAgain(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	*clock = time.UnixMilli(int64(WindowBars+1) * hour)
	engine.Tick(context.Background())
	inUse, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Deactivate(inUse.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.ActiveStrategy(*clock); !errors.Is(err, ErrNoStrategyInUse) {
		t.Fatalf("err = %v, want none in use rather than a new default", err)
	}
	settings, err := engine.Store.LoadSettings(*clock)
	if err != nil || settings.ActiveStrategyID != "" {
		t.Fatalf("settings = %+v, err = %v", settings, err)
	}

	before, _ := engine.Store.RecentSignals(inUse.ID, series.ID, 10)
	fill(t, engine, series, WindowBars+1, 0.3)
	*clock = time.UnixMilli(int64(WindowBars+2) * hour)
	engine.Tick(context.Background())
	after, _ := engine.Store.RecentSignals(inUse.ID, series.ID, 10)
	if len(before) == 0 || len(after) != len(before) {
		t.Fatalf("signals = %d then %d, want them kept and no new ones", len(before), len(after))
	}

	// Stopped, it can be deleted, and it can be used again.
	if err := engine.Store.Activate(inUse.ID); err != nil {
		t.Fatal(err)
	}
	engine.Tick(context.Background())
	if resumed, _ := engine.Store.RecentSignals(inUse.ID, series.ID, 10); len(resumed) <= len(before) {
		t.Fatalf("signals = %d, want forecasts to resume past %d", len(resumed), len(before))
	}
	if err := engine.Store.Deactivate("not-a-uuid"); !errors.Is(err, ErrStrategyAbsent) {
		t.Fatalf("err = %v", err)
	}
}

func TestRevivingARetiredStrategyClearsWhatTheOldCodeRecorded(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	*clock = time.UnixMilli(int64(WindowBars+1) * hour)
	engine.Tick(context.Background())
	old, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	run, err := engine.Store.CreateBacktest(old.ID, "", BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}}, *clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.db.Model(&Strategy{}).Where("id = ?", old.ID).Update("engine_version", EngineVersion-1).Error; err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.carryForward(*clock); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Activate(old.ID); !errors.Is(err, ErrStrategyRetired) {
		t.Fatalf("err = %v, want using it without reviving refused", err)
	}

	if err := engine.Store.Revive(old.ID); !errors.Is(err, ErrStrategyBusy) {
		t.Fatalf("err = %v, a running backtest should hold it", err)
	}
	if err := engine.Store.finishBacktest(run.ID, nil, "", *clock); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Revive(old.ID); err != nil {
		t.Fatal(err)
	}
	active, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != old.ID || active.Retired || active.Name != old.Name {
		t.Fatalf("active = %+v, want the same strategy under this code", active)
	}
	if records, _ := engine.Store.RecentSignals(old.ID, series.ID, 5); len(records) != 0 {
		t.Fatalf("records = %d, want what the old code recorded gone", len(records))
	}
	if runs, _ := engine.Store.Backtests(10); len(runs) != 0 {
		t.Fatalf("backtests = %d, want them gone", len(runs))
	}

	// Its record starts again under this code.
	engine.Tick(context.Background())
	if records, _ := engine.Store.RecentSignals(old.ID, series.ID, 5); len(records) != 1 {
		t.Fatalf("records = %d, want a new forecast", len(records))
	}
	if err := engine.Store.Revive("not-a-uuid"); !errors.Is(err, ErrStrategyAbsent) {
		t.Fatalf("err = %v", err)
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
	records, err := engine.Store.RecentSignals(other.ID, series.ID, 5)
	if err != nil || len(records) == 0 {
		t.Fatalf("records = %d, err = %v", len(records), err)
	}
	run, err := engine.Store.CreateBacktest(other.ID, "", BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}}, *clock)
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
	if records, _ := engine.Store.RecentSignals(other.ID, series.ID, 5); len(records) != 0 {
		t.Fatalf("records = %d after deleting", len(records))
	}
	if runs, _ := engine.Store.Backtests(10); len(runs) != 0 {
		t.Fatalf("backtests = %d after deleting", len(runs))
	}
	if err := engine.Store.DeleteStrategy(other.ID, *clock); !errors.Is(err, ErrStrategyAbsent) {
		t.Fatalf("err = %v deleting twice", err)
	}
}

func TestDetailShowsOneCallForTheNewestCandleAndSaysWhenItIsStale(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars, 0.3)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	last := int64(WindowBars) * hour
	*clock = time.UnixMilli(last + 2*hour)

	// Nothing is stored yet, so the call is worked out the same way.
	detail, err := engine.Detail(strategy, series.Symbol, series.Interval, nil, DetailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Latest == nil || detail.Latest.OpenTime != last || detail.Latest.Status != StatusOpen || detail.Latest.ID != 0 {
		t.Fatalf("latest = %+v, want an unsaved call on the newest candle", detail.Latest)
	}
	if detail.Stale {
		t.Fatal("a series that is keeping up is not stale")
	}

	engine.Tick(context.Background())
	detail, err = engine.Detail(strategy, series.Symbol, series.Interval, nil, DetailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Latest == nil || detail.Latest.ID == 0 || detail.Latest.OpenTime != last {
		t.Fatalf("latest = %+v, want the stored call", detail.Latest)
	}

	*clock = time.UnixMilli(last + 10*hour)
	detail, err = engine.Detail(strategy, series.Symbol, series.Interval, nil, DetailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Stale || detail.Latest == nil {
		t.Fatalf("stale = %v, latest = %+v, want the last call kept and marked", detail.Stale, detail.Latest)
	}
}

func TestAHoleAndARealFailureAreToldApart(t *testing.T) {
	engine, series, _, clock := engineFixture(t)
	fill(t, engine, series, WindowBars+10, 0.3)
	strategy, _ := engine.Store.ActiveStrategy(*clock)
	if err := engine.market.Store.DB().Exec("DELETE FROM market.candles WHERE series_id = ? AND open_at = ?", series.ID, at(int64(WindowBars-50)*hour)).Error; err != nil {
		t.Fatal(err)
	}
	*clock = time.UnixMilli(int64(WindowBars+11) * hour)

	engine.Tick(context.Background())
	signals, err := engine.Coins(strategy, "1h", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Pending != PendingNotContinuous {
		t.Fatalf("signals = %+v, want the hole named", signals)
	}
	detail, err := engine.Detail(strategy, series.Symbol, series.Interval, nil, DetailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Pending != PendingNotContinuous {
		t.Fatalf("pending = %q, want the list and the page to agree", detail.Pending)
	}

	if pendingFor(errors.New("connection refused")) != "" {
		t.Fatal("a database failure is not something to wait for")
	}
	if !engine.noteSkipped(series, errors.New("connection refused")) {
		t.Fatal("a real failure should count as one")
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
	if overlay.Pending != PendingFetching || overlay.Horizon != 0 {
		t.Fatalf("overlay = %+v", overlay)
	}

	fill(t, engine, series, WindowBars, 0.3)
	last := int64(WindowBars) * hour
	*clock = time.UnixMilli(last + 2*hour)
	engine.Tick(context.Background())
	overlay, err = engine.OverlayFor(strategy, series.Symbol, series.Interval)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Pending != "" || overlay.Direction == "" {
		t.Fatalf("overlay = %+v", overlay)
	}
	if want := DefaultConfig().Horizons["1h"]; overlay.Horizon != want || overlay.OpenTime != last || overlay.TargetTime != last+int64(want)*hour || overlay.Close <= 0 || overlay.ExpectedMovePct <= 0 {
		t.Fatalf("overlay = %+v, want the band drawn from the last closed candle to its target", overlay)
	}

	// Hours later with no new candles, drawing it would start in the past.
	*clock = time.UnixMilli(last + 10*hour)
	overlay, err = engine.OverlayFor(strategy, series.Symbol, series.Interval)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Pending != PendingFetching || overlay.Horizon != 0 {
		t.Fatalf("overlay = %+v", overlay)
	}
}

// A list of runs and a run read whole both say what it really tested.
func TestABacktestSaysWhatItReallyTested(t *testing.T) {
	engine, _, _, clock := engineFixture(t)
	strategy, err := engine.Store.ActiveStrategy(*clock)
	if err != nil {
		t.Fatal(err)
	}
	options := BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}, From: 1000, To: 900000}
	run, err := engine.Store.CreateBacktest(strategy.ID, "", options, *clock)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := engine.Store.CreateBacktest(strategy.ID, "", options, *clock)
	if err != nil {
		t.Fatal(err)
	}
	report := &Report{FirstTime: 500000, LastTime: 600000, Overall: Metrics{Samples: 1}}
	if err := engine.Store.finishBacktest(run.ID, report, "", *clock); err != nil {
		t.Fatal(err)
	}
	listed, err := engine.Store.Backtests(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range listed {
		switch row.ID {
		case run.ID:
			if row.TestedFrom != 500000 || row.TestedTo != 600000 {
				t.Fatalf("listed run = %+v", row)
			}
		case pending.ID:
			if row.TestedFrom != 0 || row.TestedTo != 0 {
				t.Fatalf("a run with no report yet = %+v", row)
			}
		}
	}
	whole, err := engine.Store.BacktestByID(run.ID)
	if err != nil || whole.TestedFrom != 500000 || whole.TestedTo != 600000 {
		t.Fatalf("run = %+v, err = %v", whole, err)
	}
}
