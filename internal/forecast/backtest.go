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
// series is tested every horizon bars, at candles a whole number of horizons from the
// epoch, so one move is not counted several times and coins line up.
type Metrics struct {
	Samples int `json:"samples"`
	Signals int `json:"signals"`
	// Moments counts the separate stretches of time the calls that moved come from. A
	// report from before this was counted has 0.
	Moments int `json:"moments"`
	Hits    int `json:"hits"`
	Misses  int `json:"misses"`
	// HitRate is hits over directional signals that moved.
	HitRate *float64 `json:"hit_rate"`
	// ChanceRate is what the same mix of up and down calls would hit with no skill. A
	// hit rate has to beat this to mean anything.
	ChanceRate *float64 `json:"chance_rate"`
	// BaselineUp is how often price simply rose, over the samples where it moved.
	BaselineUp float64 `json:"baseline_up"`
	Brier      float64 `json:"brier"`
	// BrierCandles is the Brier score of the same samples on candle factors alone, so
	// what derivatives add can be seen.
	BrierCandles float64 `json:"brier_candles"`
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

// FactorEdge is how one factor's reading lined up with what happened next.
type FactorEdge struct {
	Samples int `json:"samples"`
	// AgreeRate is how often its sign matched the move, when it had one and price moved.
	AgreeRate *float64 `json:"agree_rate"`
	// Correlation is between its reading and the move, as a share of the expected move.
	Correlation *float64 `json:"correlation"`
}

// FactorReport is one factor's record in each kind of market, since each has its own
// weights. It says which factors carry anything, and which way round to read them.
type FactorReport struct {
	Name     string     `json:"name"`
	Trending FactorEdge `json:"trending"`
	Ranging  FactorEdge `json:"ranging"`
}

// Report is a backtest's result.
type Report struct {
	FeePct      float64             `json:"fee_pct"`
	Overall     Metrics             `json:"overall"`
	ByInterval  map[string]Metrics  `json:"by_interval"`
	ByRegime    map[string]Metrics  `json:"by_regime"`
	BySymbol    map[string]Metrics  `json:"by_symbol"`
	Factors     []FactorReport      `json:"factors"`
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

// chanceRate is the hit rate of calling up and down this many times with no skill, in a
// market that rose this share of the time. Always calling down in a falling market hits
// often, and shows no skill, so a hit rate is judged against this. Nil with no calls.
func chanceRate[N int | int64](upCalls, downCalls, rose, moved N) *float64 {
	if upCalls+downCalls == 0 || moved == 0 {
		return nil
	}
	risen := float64(rose) / float64(moved)
	rate := (float64(upCalls)*risen + float64(downCalls)*(1-risen)) / float64(upCalls+downCalls)
	return &rate
}

type accumulator struct {
	samples, signals, hits, misses, ups, moved, withinMove, withDerivatives, upCalls, downCalls int
	brier, brierCandles, probSum, returnSum                                                     float64
	moments                                                                                     map[sampleMoment]bool
}

// sampleMoment is one horizon's stretch of time in one interval.
type sampleMoment struct {
	interval string
	index    int64
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
	a.brierCandles += (sample.probCandles - outcome) * (sample.probCandles - outcome)
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
	if signed != 0 {
		if sample.direction == DirectionUp {
			a.upCalls++
		} else {
			a.downCalls++
		}
		if a.moments == nil {
			a.moments = map[sampleMoment]bool{}
		}
		a.moments[sampleMoment{sample.interval, sample.moment}] = true
	}
}

func (a *accumulator) metrics() Metrics {
	m := Metrics{Samples: a.samples, Signals: a.signals, Moments: len(a.moments), Hits: a.hits, Misses: a.misses}
	if a.hits+a.misses > 0 {
		rate := float64(a.hits) / float64(a.hits+a.misses)
		m.HitRate = &rate
	}
	m.ChanceRate = chanceRate(a.upCalls, a.downCalls, a.ups, a.moved)
	if a.samples == 0 {
		return m
	}
	n := float64(a.samples)
	if a.moved > 0 {
		m.BaselineUp = float64(a.ups) / float64(a.moved)
	}
	m.Brier = a.brier / n
	m.BrierCandles = a.brierCandles / n
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
	symbol, interval, regime, direction     string
	prob, probCandles, movePct, expectedPct float64
	up, hasDerivatives                      bool
	time, moment                            int64
	factors                                 []FactorValue
}

// factorAccumulator adds up one factor in one kind of market.
type factorAccumulator struct {
	samples, agreed, disagreed int
	sumX, sumY, sumXX          float64
	sumYY, sumXY               float64
}

func (f *factorAccumulator) add(value, movePct, expectedPct float64) {
	f.samples++
	switch {
	case value*movePct > 0:
		f.agreed++
	case value*movePct < 0:
		f.disagreed++
	}
	if expectedPct <= 0 {
		return
	}
	move := movePct / expectedPct
	f.sumX += value
	f.sumY += move
	f.sumXX += value * value
	f.sumYY += move * move
	f.sumXY += value * move
}

func (f *factorAccumulator) edge() FactorEdge {
	edge := FactorEdge{Samples: f.samples}
	if called := f.agreed + f.disagreed; called > 0 {
		rate := float64(f.agreed) / float64(called)
		edge.AgreeRate = &rate
	}
	n := float64(f.samples)
	spreadX, spreadY := n*f.sumXX-f.sumX*f.sumX, n*f.sumYY-f.sumY*f.sumY
	if spreadX > 0 && spreadY > 0 {
		correlation := (n*f.sumXY - f.sumX*f.sumY) / math.Sqrt(spreadX*spreadY)
		edge.Correlation = &correlation
	}
	return edge
}

// summary adds samples up as they come, so a long run does not hold them all in memory.
type summary struct {
	overall                        accumulator
	byInterval, byRegime, bySymbol map[string]*accumulator
	factors                        map[string]map[string]*factorAccumulator
	buckets                        [10]struct {
		count, ups int
		prob       float64
	}
	first, last int64
}

func newSummary() *summary {
	return &summary{
		byInterval: map[string]*accumulator{}, byRegime: map[string]*accumulator{}, bySymbol: map[string]*accumulator{},
		factors: map[string]map[string]*factorAccumulator{},
	}
}

func (s *summary) add(sample backtestSample) {
	get := func(group map[string]*accumulator, key string) *accumulator {
		if group[key] == nil {
			group[key] = &accumulator{}
		}
		return group[key]
	}
	s.overall.add(sample)
	get(s.byInterval, sample.interval).add(sample)
	get(s.byRegime, sample.regime).add(sample)
	get(s.bySymbol, sample.symbol).add(sample)
	for _, factor := range sample.factors {
		regimes := s.factors[factor.Name]
		if regimes == nil {
			regimes = map[string]*factorAccumulator{RegimeTrending: {}, RegimeRanging: {}}
			s.factors[factor.Name] = regimes
		}
		regimes[sample.regime].add(factor.Value, sample.movePct, sample.expectedPct)
	}
	bucket := &s.buckets[min(int(sample.prob*10), 9)]
	bucket.count++
	bucket.prob += sample.prob
	if sample.up {
		bucket.ups++
	}
	if s.first == 0 || sample.time < s.first {
		s.first = sample.time
	}
	s.last = max(s.last, sample.time)
}

func (s *summary) fill(report *Report) {
	report.FirstTime, report.LastTime = s.first, s.last
	report.Overall = s.overall.metrics()
	for key, group := range s.byInterval {
		report.ByInterval[key] = group.metrics()
	}
	for key, group := range s.byRegime {
		report.ByRegime[key] = group.metrics()
	}
	for key, group := range s.bySymbol {
		report.BySymbol[key] = group.metrics()
	}
	report.Factors = []FactorReport{}
	for _, name := range append(append([]string{}, CandleFactors...), DerivativeFactors...) {
		if regimes := s.factors[name]; regimes != nil {
			report.Factors = append(report.Factors, FactorReport{Name: name, Trending: regimes[RegimeTrending].edge(), Ranging: regimes[RegimeRanging].edge()})
		}
	}
	for i, bucket := range s.buckets {
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

// RunBacktest tests a config on local history. Progress reports finished series.
func RunBacktest(ctx context.Context, source BacktestSource, config Config, options BacktestOptions, progress func(done, total int)) (Report, error) {
	if len(options.Symbols) == 0 || len(options.Intervals) == 0 || options.To <= options.From {
		return Report{}, errors.New("choose symbols, intervals and a date range")
	}
	added := newSummary()
	report := Report{FeePct: RoundTripFeePct, ByInterval: map[string]Metrics{}, ByRegime: map[string]Metrics{}, BySymbol: map[string]Metrics{}}
	total := len(options.Symbols) * len(options.Intervals)
	done := 0
	var longest int64
	for _, interval := range options.Intervals {
		longest = max(longest, intervalMillis(interval))
	}
	for _, symbol := range options.Symbols {
		readings, err := source.Readings(symbol, options.From-readingsSpan(longest), options.To+longest)
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
			for _, sample := range series {
				added.add(sample)
			}
			done++
			if progress != nil {
				progress(done, total)
			}
		}
	}
	added.fill(&report)
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
	// Samples sit at candles a whole number of horizons from the epoch, so every coin
	// and every run tests the same candles, windows do not overlap, and a moment is the
	// same stretch of time for all of them.
	span := int64(horizon) * step
	var samples []backtestSample
	for i := WindowBars - 1; i+horizon < len(candles); i++ {
		at := candles[i].OpenTime
		if at%span != 0 || at < options.From || at > options.To {
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
			prob: result.ProbFull, probCandles: result.ProbCandles, movePct: (target/result.Close - 1) * 100, expectedPct: result.ExpectedMovePct,
			up: target > result.Close, hasDerivatives: result.HasDerivatives, time: at, moment: at / span, factors: result.Factors,
		})
	}
	return samples, nil
}
