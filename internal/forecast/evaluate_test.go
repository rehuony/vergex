package forecast

import (
	"context"
	"math"
	"testing"

	"vergex/internal/market"
)

// walk builds candles from a repeatable price path, so tests have real-looking
// data without randomness.
func walk(n int, drift float64) []market.Kline {
	klines := make([]market.Kline, n)
	price := 100.0
	for i := range klines {
		wiggle := math.Sin(float64(i)/3)*0.8 + math.Sin(float64(i)/11)*1.5
		open := price
		price = math.Max(1, price+drift+wiggle*0.3)
		high, low := math.Max(open, price)+0.4, math.Min(open, price)-0.4
		klines[i] = market.Kline{
			OpenTime: int64(i+1) * hour, Open: open, High: high, Low: low, Close: price,
			Volume: 100 + 20*math.Abs(wiggle), TakerBuyBaseVolume: 50 + 10*wiggle,
		}
	}
	return klines
}

const hour = int64(3600 * 1000)

func TestRisingAndFallingMarketsPointOppositeWays(t *testing.T) {
	up, err := Evaluate(DefaultConfig(), "1h", walk(WindowBars, 0.5), Derivatives{})
	if err != nil {
		t.Fatal(err)
	}
	down, err := Evaluate(DefaultConfig(), "1h", walk(WindowBars, -0.2), Derivatives{})
	if err != nil {
		t.Fatal(err)
	}
	if up.ProbFull <= down.ProbFull {
		t.Fatalf("rising %.3f should score above falling %.3f", up.ProbFull, down.ProbFull)
	}
	if up.Regime != RegimeTrending {
		t.Fatalf("a steady climb should read as trending, ADX %.1f", up.ADX)
	}
	if up.Direction != DirectionUp {
		t.Fatalf("direction = %s at %.3f", up.Direction, up.ProbFull)
	}
	for _, factor := range up.Factors {
		if factor.Value < -1 || factor.Value > 1 {
			t.Fatalf("%s out of range: %f", factor.Name, factor.Value)
		}
	}
	if up.Support >= up.Close || up.Resistance <= 0 {
		t.Fatalf("levels: support %.2f resistance %.2f close %.2f", up.Support, up.Resistance, up.Close)
	}
	if up.ExpectedMovePct <= 0 {
		t.Fatal("expected move should be positive")
	}
}

func TestTooLittleHistoryIsRefused(t *testing.T) {
	if _, err := Evaluate(DefaultConfig(), "1h", walk(MinBars-1, 0.1), Derivatives{}); err != ErrNotEnoughHistory {
		t.Fatalf("err = %v", err)
	}
}

func TestChangingFutureCandlesCannotChangeAForecast(t *testing.T) {
	candles := walk(WindowBars+50, 0.2)
	at := WindowBars - 1
	before, err := Evaluate(DefaultConfig(), "1h", candles[at-WindowBars+1:at+1], Derivatives{})
	if err != nil {
		t.Fatal(err)
	}
	for i := at + 1; i < len(candles); i++ {
		candles[i].Close *= 3
		candles[i].High *= 3
	}
	after, err := Evaluate(DefaultConfig(), "1h", candles[at-WindowBars+1:at+1], Derivatives{})
	if err != nil {
		t.Fatal(err)
	}
	if before.ProbFull != after.ProbFull || before.Close != after.Close {
		t.Fatal("future candles changed the forecast")
	}
}

func TestDerivativesAtReadsNothingLater(t *testing.T) {
	readings := Readings{
		Funding:   []Reading{{Time: 10 * hour, Value: 0.0001}, {Time: 20 * hour, Value: 0.9}},
		OpenValue: []Reading{{Time: 0, Value: 100}, {Time: 12 * hour, Value: 110}, {Time: 20 * hour, Value: 999}},
	}
	got := DerivativesAt(readings, 12*hour, hour)
	if got.Funding == nil || *got.Funding != 0.0001 {
		t.Fatalf("funding = %v", got.Funding)
	}
	if got.OIChangePct == nil || math.Abs(*got.OIChangePct-10) > 1e-9 {
		t.Fatalf("OI change = %v", got.OIChangePct)
	}
	if DerivativesAt(readings, 40*hour, hour).Funding != nil {
		t.Fatal("old funding should not count as current")
	}
}

func TestDerivativesMoveTheFullProbabilityOnly(t *testing.T) {
	klines := walk(WindowBars, 0.1)
	bare, _ := Evaluate(DefaultConfig(), "1h", klines, Derivatives{})
	high := 0.003
	crowded, err := Evaluate(DefaultConfig(), "1h", klines, Derivatives{Funding: &high})
	if err != nil {
		t.Fatal(err)
	}
	if bare.HasDerivatives || !crowded.HasDerivatives {
		t.Fatal("has_derivatives is wrong")
	}
	if crowded.ProbCandles != bare.ProbCandles {
		t.Fatal("derivatives should not touch the candle-only probability")
	}
	if crowded.ProbFull >= bare.ProbFull {
		t.Fatalf("high funding should lean down: %.4f vs %.4f", crowded.ProbFull, bare.ProbFull)
	}
}

func TestConfigValidation(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	off := DefaultConfig()
	for name, factor := range off.Factors {
		factor.Enabled = false
		off.Factors[name] = factor
	}
	if off.Validate() == nil {
		t.Fatal("a config with no factors should be refused")
	}
	bad := DefaultConfig()
	bad.Horizons["1h"] = 0
	if bad.Validate() == nil {
		t.Fatal("a zero horizon should be refused")
	}
	unknown := DefaultConfig()
	unknown.Scorer = "other"
	if unknown.Validate() == nil {
		t.Fatal("an unknown scorer should be refused")
	}
}

func TestCalibrationOnlyEverRises(t *testing.T) {
	points := []CalibrationPoint{{0.2, false}, {0.3, true}, {0.4, false}, {0.5, false}, {0.6, true}, {0.7, false}, {0.8, true}, {0.9, true}}
	knots := FitCalibration(points)
	for i := 1; i < len(knots); i++ {
		if knots[i].Y < knots[i-1].Y || knots[i].X <= knots[i-1].X {
			t.Fatalf("not rising: %+v", knots)
		}
	}
	if got := ApplyCalibration(knots, 0.05); got != knots[0].Y {
		t.Fatalf("below the range = %f", got)
	}
	if got := ApplyCalibration(knots, 0.99); got != knots[len(knots)-1].Y {
		t.Fatalf("above the range = %f", got)
	}
	if got := ApplyCalibration(nil, 0.42); got != 0.42 {
		t.Fatalf("no curve should pass through, got %f", got)
	}
}

type fixedSource struct {
	candles []market.Kline
}

func (s fixedSource) Candles(_, _ string, from, to int64) ([]market.Kline, error) {
	var out []market.Kline
	for _, k := range s.candles {
		if k.OpenTime >= from && k.OpenTime <= to {
			out = append(out, k)
		}
	}
	return out, nil
}

func (fixedSource) Readings(string, int64, int64) (Readings, error) { return Readings{}, nil }

func TestBacktestMatchesLiveEvaluationAndDoesNotOverlap(t *testing.T) {
	candles := walk(WindowBars+400, 0.15)
	options := BacktestOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}, From: candles[WindowBars].OpenTime, To: candles[len(candles)-1].OpenTime}
	report, err := RunBacktest(context.Background(), fixedSource{candles}, DefaultConfig(), options, nil)
	if err != nil {
		t.Fatal(err)
	}
	horizon := DefaultConfig().Horizons["1h"]
	if max := 400 / horizon; report.Overall.Samples > max || report.Overall.Samples < max-2 {
		t.Fatalf("samples = %d, want about %d without overlap", report.Overall.Samples, max)
	}
	if report.Overall.HitRate == nil {
		t.Fatal("expected a hit rate")
	}
	if len(report.ByInterval) != 1 || len(report.BySymbol) != 1 || len(report.Reliability) == 0 {
		t.Fatalf("breakdowns: %+v", report)
	}
	total := 0
	for _, bucket := range report.Reliability {
		total += bucket.Samples
	}
	if total != report.Overall.Samples {
		t.Fatalf("buckets hold %d of %d samples", total, report.Overall.Samples)
	}

	// Every tested bar must give what a live forecast of that bar gives.
	samples, err := testSeries(fixedSource{candles}, DefaultConfig(), "BTCUSDT", "1h", Readings{}, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		i := int(sample.time/hour) - 1
		live, err := Evaluate(DefaultConfig(), "1h", candles[i-WindowBars+1:i+1], Derivatives{})
		if err != nil {
			t.Fatal(err)
		}
		if live.ProbFull != sample.prob || live.Direction != sample.direction {
			t.Fatalf("bar %d: live %.6f, backtest %.6f", i, live.ProbFull, sample.prob)
		}
	}
}

func TestMetricsAreWorkedOutAsDescribed(t *testing.T) {
	a := &accumulator{}
	a.add(backtestSample{direction: DirectionUp, prob: 0.8, movePct: 2, expectedPct: 3, up: true})
	a.add(backtestSample{direction: DirectionUp, prob: 0.6, movePct: -1, expectedPct: 0.5, up: false})
	a.add(backtestSample{direction: DirectionDown, prob: 0.3, movePct: -3, expectedPct: 5, up: false, hasDerivatives: true})
	a.add(backtestSample{direction: DirectionNeutral, prob: 0.5, movePct: 4, expectedPct: 1, up: true})
	m := a.metrics()
	if m.Samples != 4 || m.Signals != 3 || m.Hits != 2 || m.Misses != 1 {
		t.Fatalf("counts: %+v", m)
	}
	if *m.HitRate != 2.0/3 {
		t.Fatalf("hit rate = %f", *m.HitRate)
	}
	if m.BaselineUp != 0.5 || m.BaselineBrier != 0.25 {
		t.Fatalf("baseline: %+v", m)
	}
	wantBrier := (0.04 + 0.36 + 0.09 + 0.25) / 4
	if math.Abs(m.Brier-wantBrier) > 1e-9 {
		t.Fatalf("brier = %f, want %f", m.Brier, wantBrier)
	}
	if math.Abs(m.Lift-(1-wantBrier/0.25)) > 1e-9 {
		t.Fatalf("lift = %f", m.Lift)
	}
	if math.Abs(m.AvgReturnPct-(2-1+3)/3.0) > 1e-9 {
		t.Fatalf("avg return = %f", m.AvgReturnPct)
	}
	if math.Abs(m.AvgReturnAfterFee-(m.AvgReturnPct-RoundTripFeePct)) > 1e-9 {
		t.Fatalf("after fees = %f", m.AvgReturnAfterFee)
	}
	if m.MoveCoverage != 0.5 || m.DerivativeCoverage != 0.25 {
		t.Fatalf("coverage: %+v", m)
	}
}
