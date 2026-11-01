package forecast

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"vergex/internal/market"
)

// RoundTripFeePct is the taker fee assumed for entering and leaving a position.
const RoundTripFeePct = 0.10

// BacktestOptions says what to test.
type BacktestOptions struct {
	Symbols   []string `json:"symbols"`
	Intervals []string `json:"intervals"`
	From      int64    `json:"from"` // Unix milliseconds.
	To        int64    `json:"to"`
}

// Metrics measures forecasts against what happened. Samples do not overlap: each
// series is tested every horizon bars, so one move is not counted several times.
type Metrics struct {
	Samples int `json:"samples"`
	Signals int `json:"signals"`
	Hits    int `json:"hits"`
	Misses  int `json:"misses"`
	// HitRate is hits over directional signals that moved.
	HitRate *float64 `json:"hit_rate"`
	// BaselineUp is how often price simply rose, over the samples where it moved.
	BaselineUp float64 `json:"baseline_up"`
	Brier      float64 `json:"brier"`
	// BaselineBrier is the Brier score of always predicting BaselineUp. Lift is the
	// improvement on it; zero or below means no skill beyond the base rate.
	BaselineBrier float64 `json:"baseline_brier"`
	Lift          float64 `json:"lift"`
	// AvgReturnPct is the average return of acting on each signal, before fees.
	AvgReturnPct      float64 `json:"avg_return_pct"`
	AvgReturnAfterFee float64 `json:"avg_return_after_fee_pct"`
	// MoveCoverage is how often the actual move stayed within the expected move.
	MoveCoverage float64 `json:"move_coverage"`
	// DerivativeCoverage is the share of samples that had derivatives data.
	DerivativeCoverage float64 `json:"derivative_coverage"`
}

// ReliabilityBucket compares what a range of probabilities said with what happened.
type ReliabilityBucket struct {
	From    float64 `json:"from"`
	To      float64 `json:"to"`
	Samples int     `json:"samples"`
	AvgProb float64 `json:"avg_prob"`
	UpRate  float64 `json:"up_rate"`
}

// Report is a backtest's result.
type Report struct {
	FeePct      float64             `json:"fee_pct"`
	Overall     Metrics             `json:"overall"`
	ByInterval  map[string]Metrics  `json:"by_interval"`
	ByRegime    map[string]Metrics  `json:"by_regime"`
	BySymbol    map[string]Metrics  `json:"by_symbol"`
	Reliability []ReliabilityBucket `json:"reliability"`
	FirstTime   int64               `json:"first_time"`
	LastTime    int64               `json:"last_time"`
	Skipped     []string            `json:"skipped,omitempty"`
}

// BacktestSource reads local history. It never reaches the exchange.
type BacktestSource interface {
	Candles(symbol, interval string, from, to int64) ([]market.Kline, error)
	Readings(symbol string, from, to int64) (Readings, error)
}

type accumulator struct {
	samples, signals, hits, misses, ups, moved, withinMove, withDerivatives int
	brier, probSum, returnSum                                               float64
}

func (a *accumulator) add(sample backtestSample) {
	a.samples++
	if sample.up {
		a.ups++
	}
	if sample.movePct != 0 {
		a.moved++
	}
	outcome := 0.0
	if sample.up {
		outcome = 1
	}
	a.brier += (sample.prob - outcome) * (sample.prob - outcome)
	a.probSum += sample.prob
	if math.Abs(sample.movePct) <= sample.expectedPct {
		a.withinMove++
	}
	if sample.hasDerivatives {
		a.withDerivatives++
	}
	if sample.direction == DirectionNeutral {
		return
	}
	a.signals++
	signed := sample.movePct
	if sample.direction == DirectionDown {
		signed = -signed
	}
	a.returnSum += signed
	switch {
	case signed > 0:
		a.hits++
	case signed < 0:
		a.misses++
	}
}

func (a *accumulator) metrics() Metrics {
	m := Metrics{Samples: a.samples, Signals: a.signals, Hits: a.hits, Misses: a.misses}
	if a.hits+a.misses > 0 {
		rate := float64(a.hits) / float64(a.hits+a.misses)
		m.HitRate = &rate
	}
	if a.samples == 0 {
		return m
	}
	n := float64(a.samples)
	if a.moved > 0 {
		m.BaselineUp = float64(a.ups) / float64(a.moved)
	}
	m.Brier = a.brier / n
	// A flat sample counts as not up here, as it does in the Brier score itself.
	rise := float64(a.ups) / n
	m.BaselineBrier = rise * (1 - rise)
	if m.BaselineBrier > 0 {
		m.Lift = 1 - m.Brier/m.BaselineBrier
	}
	m.MoveCoverage = float64(a.withinMove) / n
	m.DerivativeCoverage = float64(a.withDerivatives) / n
	if a.signals > 0 {
		m.AvgReturnPct = a.returnSum / float64(a.signals)
		m.AvgReturnAfterFee = m.AvgReturnPct - RoundTripFeePct
	}
	return m
}

type backtestSample struct {
	symbol, interval, regime, direction string
	prob, movePct, expectedPct          float64
	up, hasDerivatives                  bool
	time                                int64
}

// RunBacktest tests a config on local history. Progress reports finished series.
func RunBacktest(ctx context.Context, source BacktestSource, config Config, options BacktestOptions, progress func(done, total int)) (Report, error) {
	if len(options.Symbols) == 0 || len(options.Intervals) == 0 || options.To <= options.From {
		return Report{}, errors.New("choose symbols, intervals and a date range")
	}
	var samples []backtestSample
	report := Report{FeePct: RoundTripFeePct, ByInterval: map[string]Metrics{}, ByRegime: map[string]Metrics{}, BySymbol: map[string]Metrics{}}
	total := len(options.Symbols) * len(options.Intervals)
	done := 0
	for _, symbol := range options.Symbols {
		readings, err := source.Readings(symbol, options.From-8*24*3600*1000, options.To)
		if err != nil {
			return report, err
		}
		for _, interval := range options.Intervals {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			series, err := testSeries(source, config, symbol, interval, readings, options)
			if err != nil {
				report.Skipped = append(report.Skipped, fmt.Sprintf("%s %s: %v", symbol, interval, err))
			}
			samples = append(samples, series...)
			done++
			if progress != nil {
				progress(done, total)
			}
		}
	}
	summarize(&report, samples)
	return report, nil
}

func testSeries(source BacktestSource, config Config, symbol, interval string, readings Readings, options BacktestOptions) ([]backtestSample, error) {
	step := intervalMillis(interval)
	if step == 0 {
		return nil, errors.New("unsupported interval")
	}
	horizon := config.Horizons[interval]
	if horizon <= 0 {
		return nil, errors.New("no horizon set")
	}
	candles, err := source.Candles(symbol, interval, options.From-int64(WindowBars)*step, options.To+int64(horizon)*step)
	if err != nil {
		return nil, err
	}
	if len(candles) < WindowBars+horizon {
		return nil, ErrNotEnoughHistory
	}
	var samples []backtestSample
	for i := WindowBars - 1; i+horizon < len(candles); i += horizon {
		at := candles[i].OpenTime
		if at < options.From || at > options.To {
			continue
		}
		// A window with a hole, or a target that is not where it should be, is
		// skipped rather than tested on data that does not join up.
		window := candles[i-WindowBars+1 : i+1]
		if window[0].OpenTime != at-int64(WindowBars-1)*step || candles[i+horizon].OpenTime != at+int64(horizon)*step {
			continue
		}
		result, err := Evaluate(config, interval, window, DerivativesAt(readings, at, step))
		if err != nil {
			continue
		}
		target := candles[i+horizon].Close
		samples = append(samples, backtestSample{
			symbol: symbol, interval: interval, regime: result.Regime, direction: result.Direction,
			prob: result.ProbFull, movePct: (target/result.Close - 1) * 100, expectedPct: result.ExpectedMovePct,
			up: target > result.Close, hasDerivatives: result.HasDerivatives, time: at,
		})
	}
	return samples, nil
}

func summarize(report *Report, samples []backtestSample) {
	overall := &accumulator{}
	byInterval, byRegime, bySymbol := map[string]*accumulator{}, map[string]*accumulator{}, map[string]*accumulator{}
	get := func(group map[string]*accumulator, key string) *accumulator {
		if group[key] == nil {
			group[key] = &accumulator{}
		}
		return group[key]
	}
	type bucketSum struct {
		count, ups int
		prob       float64
	}
	buckets := make([]bucketSum, 10)
	for _, sample := range samples {
		overall.add(sample)
		get(byInterval, sample.interval).add(sample)
		get(byRegime, sample.regime).add(sample)
		get(bySymbol, sample.symbol).add(sample)
		index := min(int(sample.prob*10), 9)
		buckets[index].count++
		buckets[index].prob += sample.prob
		if sample.up {
			buckets[index].ups++
		}
		if report.FirstTime == 0 || sample.time < report.FirstTime {
			report.FirstTime = sample.time
		}
		if sample.time > report.LastTime {
			report.LastTime = sample.time
		}
	}
	report.Overall = overall.metrics()
	for key, group := range byInterval {
		report.ByInterval[key] = group.metrics()
	}
	for key, group := range byRegime {
		report.ByRegime[key] = group.metrics()
	}
	for key, group := range bySymbol {
		report.BySymbol[key] = group.metrics()
	}
	for i, bucket := range buckets {
		if bucket.count == 0 {
			continue
		}
		report.Reliability = append(report.Reliability, ReliabilityBucket{
			From: float64(i) / 10, To: float64(i+1) / 10, Samples: bucket.count,
			AvgProb: bucket.prob / float64(bucket.count), UpRate: float64(bucket.ups) / float64(bucket.count),
		})
	}
	sort.Strings(report.Skipped)
}
