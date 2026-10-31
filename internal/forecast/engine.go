package forecast

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"vergex/internal/logger"
	"vergex/internal/market"
	"vergex/internal/marketdata"
	"vergex/internal/safe"
	"vergex/internal/servicestate"
)

const (
	engineTick       = 15 * time.Second
	verifyBatch      = 2000
	calibrateEvery   = time.Hour
	maxCatchUpBars   = 8 // After downtime, only the latest candles get a forecast.
	derivativesSpan  = 8 * 24 * time.Hour
	recentRecordRows = 50
)

// Engine records forecasts as candles close, judges them when their target
// candle closes and sends threshold alerts. It reads only local data.
type Engine struct {
	Store  *Store
	market *marketdata.Service
	now    func() time.Time
	notify func(ctx context.Context, text string) error

	mu          sync.Mutex
	skipped     map[int64]string
	backtesting bool
	calibrated  time.Time
}

// NewEngine creates an engine over the market data service. notify sends alert
// text and may be nil.
func NewEngine(store *Store, service *marketdata.Service, notify func(ctx context.Context, text string) error) *Engine {
	return &Engine{Store: store, market: service, now: time.Now, notify: notify, skipped: map[int64]string{}}
}

// Start runs the engine in the background until ctx ends.
func (e *Engine) Start(ctx context.Context) {
	if err := e.Store.failInterruptedBacktests(e.now()); err != nil {
		logger.Warnf("Forecast: %v", err)
	}
	safe.GoNamed("forecast-engine", func() {
		ticker := time.NewTicker(engineTick)
		defer ticker.Stop()
		for {
			// Candles from the stream are forecast the moment they are stored; the
			// ticker catches those the hydrator fetched.
			select {
			case <-ctx.Done():
				return
			case <-e.market.CandlesWritten():
				e.Tick(ctx)
			case <-ticker.C:
				e.Tick(ctx)
			}
		}
	})
}

// Tick does one round of work.
func (e *Engine) Tick(ctx context.Context) {
	strategy, err := e.Store.ActiveStrategy(e.now())
	if err != nil {
		logger.Warnf("Forecast: read the active strategy: %v", err)
		servicestate.Set(servicestate.Forecast, servicestate.Failed, err.Error())
		return
	}
	servicestate.Set(servicestate.Forecast, servicestate.Running, "")
	created := e.record(strategy)
	e.verify()
	e.calibrateIfDue(strategy)
	e.alert(ctx, created)
}

// record forecasts every candle that closed since the last round.
func (e *Engine) record(strategy *Strategy) []Record {
	series, err := e.market.Store.ListSeries()
	if err != nil {
		logger.Warnf("Forecast: list series: %v", err)
		return nil
	}
	latest, err := e.Store.LatestOpenTimes(strategy.ID)
	if err != nil {
		logger.Warnf("Forecast: read latest records: %v", err)
		return nil
	}
	curves, err := e.Store.Calibrations(strategy.ID)
	if err != nil {
		logger.Warnf("Forecast: read calibrations: %v", err)
	}
	var created []Record
	for _, row := range series {
		// Trading keeps series of its own at intervals Forecast has no settings for.
		if row.Status != marketdata.StatusActive || row.LastClosedTime == 0 || intervalMillis(row.Interval) == 0 {
			continue
		}
		step := marketdata.IntervalMillis(row.Interval)
		start := row.LastClosedTime
		if last, seen := latest[row.ID]; seen {
			if last >= row.LastClosedTime {
				continue
			}
			start = max(last+step, row.LastClosedTime-(maxCatchUpBars-1)*step)
		}
		for at := start; at <= row.LastClosedTime; at += step {
			record, err := e.forecast(strategy, row, at, curves[row.Interval])
			if err != nil {
				e.setSkipped(row.ID, err.Error())
				continue
			}
			e.setSkipped(row.ID, "")
			isNew, err := e.Store.SaveRecord(record)
			if err != nil {
				logger.Warnf("Forecast: save %s %s: %v", row.Symbol, row.Interval, err)
				continue
			}
			if isNew {
				created = append(created, *record)
			}
		}
	}
	return created
}

func (e *Engine) setSkipped(seriesID int64, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if reason == "" {
		delete(e.skipped, seriesID)
	} else {
		e.skipped[seriesID] = reason
	}
}

// evaluateAt forecasts a series at one candle from local data.
func (e *Engine) evaluateAt(strategy *Strategy, series *marketdata.Series, at int64) (Result, error) {
	candles, err := e.market.Store.Window(series, at, WindowBars)
	if err != nil {
		if errors.Is(err, marketdata.ErrNotEnoughHistory) {
			return Result{}, ErrNotEnoughHistory
		}
		return Result{}, err
	}
	readings, err := readingsFrom(e.market.Store, series.Symbol, at-derivativesSpan.Milliseconds(), at)
	if err != nil {
		return Result{}, err
	}
	step := marketdata.IntervalMillis(series.Interval)
	return Evaluate(strategy.Config, series.Interval, marketdata.ToKlines(candles, series.Interval), DerivativesAt(readings, at, step))
}

func readingsFrom(store *marketdata.Store, symbol string, from, to int64) (Readings, error) {
	var readings Readings
	for _, source := range []struct {
		kind string
		dest *[]Reading
	}{
		{marketdata.KindFunding, &readings.Funding},
		{marketdata.KindOpenValue, &readings.OpenValue},
		{marketdata.KindLongShort, &readings.LongShort},
	} {
		rows, err := store.Derivatives(symbol, source.kind, from, to)
		if err != nil {
			return readings, err
		}
		*source.dest = make([]Reading, len(rows))
		for i, row := range rows {
			(*source.dest)[i] = Reading{Time: row.Time, Value: row.Value}
		}
	}
	return readings, nil
}

func (e *Engine) forecast(strategy *Strategy, series *marketdata.Series, at int64, curve []Knot) (*Record, error) {
	result, err := e.evaluateAt(strategy, series, at)
	if err != nil {
		return nil, err
	}
	step := marketdata.IntervalMillis(series.Interval)
	display := result.ProbFull
	calibrated := len(curve) > 0
	if calibrated {
		display = ApplyCalibration(curve, result.ProbFull)
	}
	return &Record{
		StrategyID: strategy.ID, SeriesID: series.ID, OpenTime: at, Symbol: series.Symbol, Interval: series.Interval,
		Close: result.Close, ProbCandles: result.ProbCandles, ProbFull: result.ProbFull, DisplayProb: display, Calibrated: calibrated,
		Direction: result.Direction, Confidence: result.Confidence, ExpectedMovePct: result.ExpectedMovePct,
		Support: result.Support, Resistance: result.Resistance, Regime: result.Regime, Horizon: result.Horizon,
		TargetTime: at + int64(result.Horizon)*step, Status: StatusOpen,
	}, nil
}

// verify judges records whose target candle has closed.
func (e *Engine) verify() {
	now := e.now().UnixMilli()
	records, err := e.Store.DueRecords(now, verifyBatch)
	if err != nil {
		logger.Warnf("Forecast: read due records: %v", err)
		return
	}
	series, err := e.market.Store.ListSeries()
	if err != nil {
		return
	}
	byID := make(map[int64]*marketdata.Series, len(series))
	for _, row := range series {
		byID[row.ID] = row
	}
	for i := range records {
		record := &records[i]
		row := byID[record.SeriesID]
		if row == nil || row.LastClosedTime < record.TargetTime {
			continue // Not there yet.
		}
		candles, err := e.market.Store.Range(record.SeriesID, record.TargetTime, record.TargetTime)
		if err != nil {
			logger.Warnf("Forecast: read target candle: %v", err)
			continue
		}
		if len(candles) == 0 {
			record.Status = StatusVoid
			record.VerifiedAt = now
		} else {
			Judge(record, candles[0].Close, now)
		}
		if err := e.Store.saveVerification(record); err != nil {
			logger.Warnf("Forecast: save verification: %v", err)
		}
	}
}

// Judge fills in how a record turned out, given its target candle's close.
func Judge(record *Record, targetClose float64, now int64) {
	record.Status = StatusVerified
	record.TargetClose = targetClose
	record.VerifiedAt = now
	record.ActualMovePct = (targetClose/record.Close - 1) * 100
	switch {
	case record.Direction == DirectionNeutral || targetClose == record.Close:
		record.Outcome = OutcomeNeutral
	case (record.Direction == DirectionUp) == (targetClose > record.Close):
		record.Outcome = OutcomeHit
	default:
		record.Outcome = OutcomeMiss
	}
}

func (e *Engine) calibrateIfDue(strategy *Strategy) {
	e.mu.Lock()
	due := e.now().Sub(e.calibrated) >= calibrateEvery
	if due {
		e.calibrated = e.now()
	}
	e.mu.Unlock()
	if !due {
		return
	}
	for _, interval := range Intervals {
		if err := e.Calibrate(strategy, interval); err != nil {
			logger.Warnf("Forecast: calibrate %s: %v", interval, err)
		}
	}
}

// Calibrate rebuilds an interval's curve from verified forecasts that do not
// overlap, once there are enough of them.
func (e *Engine) Calibrate(strategy *Strategy, interval string) error {
	records, err := e.Store.VerifiedRecords(strategy.ID, interval)
	if err != nil {
		return err
	}
	points := nonOverlapping(records, intervalMillis(interval))
	if len(points) < MinCalibrationSamples {
		return nil
	}
	return e.Store.saveCalibration(Calibration{
		StrategyID: strategy.ID, Interval: interval, Samples: len(points),
		Knots: encodeKnots(FitCalibration(points)), BuiltAt: e.now().UnixMilli(),
	})
}

// nonOverlapping keeps records whose forecast windows do not overlap within a
// series, and that had a real move. Records must be ordered by series and time.
func nonOverlapping(records []Record, step int64) []CalibrationPoint {
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

// Score is a probability as the 0-100 number shown to the user.
func Score(prob float64) int { return int(math.Round(prob * 100)) }

// alert sends watchlist signals crossing the threshold, once each time: a series
// that alerted is silent until its score comes back inside.
func (e *Engine) alert(ctx context.Context, created []Record) {
	if e.notify == nil || len(created) == 0 {
		return
	}
	settings, err := e.Store.LoadSettings(e.now())
	if err != nil || settings.AlertThreshold <= 50 {
		return
	}
	marketSettings, err := e.market.Store.LoadSettings()
	if err != nil {
		return
	}
	watched := map[string]bool{}
	for _, symbol := range marketSettings.WatchSymbols() {
		watched[symbol] = true
	}
	var lines []string
	var fired []int64
	var rearmed []int64
	for i := range created {
		record := &created[i]
		if !watched[record.Symbol] {
			continue
		}
		score := Score(record.DisplayProb)
		beyond := score >= settings.AlertThreshold || score <= 100-settings.AlertThreshold
		switch {
		case beyond && !e.Store.alertArm(record.SeriesID):
			lines = append(lines, alertLine(record, score))
			fired = append(fired, record.SeriesID)
		case !beyond:
			rearmed = append(rearmed, record.SeriesID)
		}
	}
	for _, seriesID := range rearmed {
		if err := e.Store.setAlertArm(seriesID, false); err != nil {
			logger.Warnf("Forecast: %v", err)
		}
	}
	if len(lines) == 0 {
		return
	}
	if err := e.notify(ctx, strings.Join(lines, "\n")); err != nil {
		logger.Warnf("Forecast: send alert: %v", err)
		return
	}
	for _, seriesID := range fired {
		if err := e.Store.setAlertArm(seriesID, true); err != nil {
			logger.Warnf("Forecast: %v", err)
		}
	}
}

func alertLine(record *Record, score int) string {
	direction := "Up"
	if record.DisplayProb < 0.5 {
		direction = "Down"
	}
	value := fmt.Sprintf("Score %d", score)
	if record.Calibrated {
		value = fmt.Sprintf("%d%%", score)
	}
	return fmt.Sprintf("Forecast %s %s: %s, %s over the next %d bars", record.Symbol, record.Interval, direction, value, record.Horizon)
}

// SeriesSkipReason says why a series has no current forecast, if it is known.
func (e *Engine) SeriesSkipReason(seriesID int64) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.skipped[seriesID]
}

// Candles reads local candles as Klines, for backtests.
func (e *Engine) Candles(symbol, interval string, from, to int64) ([]market.Kline, error) {
	series, err := e.market.Store.FindSeries(symbol, interval)
	if err != nil {
		return nil, err
	}
	if series == nil {
		return nil, ErrNotEnoughHistory
	}
	candles, err := e.market.Store.Range(series.ID, from, to)
	if err != nil {
		return nil, err
	}
	return marketdata.ToKlines(candles, interval), nil
}

// Readings reads local derivatives history, for backtests.
func (e *Engine) Readings(symbol string, from, to int64) (Readings, error) {
	return readingsFrom(e.market.Store, symbol, from, to)
}

// StartBacktest runs a backtest in the background, one at a time. It reads only
// the local store and never requests anything from the exchange.
func (e *Engine) StartBacktest(ctx context.Context, strategyID int64, options BacktestOptions) (*Backtest, error) {
	strategy, err := e.Store.StrategyByID(strategyID)
	if err != nil {
		return nil, errors.New("strategy strategy not found")
	}
	e.mu.Lock()
	if e.backtesting {
		e.mu.Unlock()
		return nil, errors.New("a backtest is already running")
	}
	e.backtesting = true
	e.mu.Unlock()

	run, err := e.Store.CreateBacktest(strategyID, options, e.now())
	if err != nil {
		e.mu.Lock()
		e.backtesting = false
		e.mu.Unlock()
		return nil, err
	}
	safe.GoNamed("forecast-backtest", func() {
		defer func() {
			e.mu.Lock()
			e.backtesting = false
			e.mu.Unlock()
		}()
		report, err := RunBacktest(ctx, e, strategy.Config, options, func(done, _ int) {
			if err := e.Store.updateBacktestProgress(run.ID, done); err != nil {
				logger.Warnf("Forecast: %v", err)
			}
		})
		failure := ""
		if err != nil {
			failure = err.Error()
		}
		if err := e.Store.finishBacktest(run.ID, &report, failure, e.now()); err != nil {
			logger.Warnf("Forecast: save backtest: %v", err)
		}
	})
	return run, nil
}

// Why a coin has no forecast yet.
const (
	PendingFetching         = "fetching"
	PendingWaiting          = "waiting"
	PendingNotEnoughHistory = "not_enough_history"
	PendingNotContinuous    = "not_continuous"
	PendingUnavailable      = "unavailable" // The exchange has no such coin.
)

// Signal is one row of the signal table: a series and its latest forecast.
type Signal struct {
	Symbol   string   `json:"symbol"`
	Interval string   `json:"interval"`
	Tier     int      `json:"tier"`
	Price    *float64 `json:"price"`
	Record   *Record  `json:"record"`
	Stale    bool     `json:"stale"`
	// Pending says why there is no forecast yet.
	Pending string `json:"pending,omitempty"`
}

// Signals lists every tracked coin at an interval with its latest forecast.
func (e *Engine) Signals(strategy *Strategy, interval string, prices map[string]float64) ([]Signal, error) {
	series, err := e.market.Store.ListSeries()
	if err != nil {
		return nil, err
	}
	records, err := e.Store.LatestRecords(strategy.ID, interval)
	if err != nil {
		return nil, err
	}
	bySeries := make(map[int64]*Record, len(records))
	for i := range records {
		bySeries[records[i].SeriesID] = &records[i]
	}
	now := e.now().UnixMilli()
	step := intervalMillis(interval)
	signals := []Signal{}
	for _, row := range series {
		if row.Status != marketdata.StatusActive || row.Interval != interval {
			continue
		}
		signal := Signal{Symbol: row.Symbol, Interval: interval, Tier: row.Tier, Record: bySeries[row.ID]}
		if price, ok := prices[row.Symbol]; ok {
			signal.Price = &price
		}
		switch {
		case signal.Record != nil:
			signal.Stale = now-signal.Record.OpenTime > 3*step
		case row.LastClosedTime == 0:
			signal.Pending = PendingFetching
		default:
			signal.Pending = PendingWaiting
			if e.SeriesSkipReason(row.ID) == ErrNotEnoughHistory.Error() {
				signal.Pending = PendingNotEnoughHistory
			}
		}
		signals = append(signals, signal)
	}
	return signals, nil
}

// Detail is everything the forecast page shows for one coin at one interval.
type Detail struct {
	Symbol     string      `json:"symbol"`
	Interval   string      `json:"interval"`
	Price      *float64    `json:"price"`
	Ready      bool        `json:"ready"`
	Pending    string      `json:"pending,omitempty"`
	Result     *Result     `json:"result"`
	Latest     *Record     `json:"latest"`
	Records    []Record    `json:"records"`
	Stats      RecordStats `json:"stats"`
	Candles    []CandleRow `json:"candles"`
	LastClosed int64       `json:"last_closed"`
	// Forming is the candle still open, while the stream is delivering it. Forecasts
	// only ever use closed candles.
	Forming *CandleRow `json:"forming"`
	// Projection draws the forecast as candles. See Project.
	Projection []CandleRow `json:"projection"`
	// Indicators is there when asked for.
	Indicators *Indicators `json:"indicators,omitempty"`
}

// CandleRow is a candle for the chart.
type CandleRow struct {
	Time   int64   `json:"time"` // Unix seconds.
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

const detailCandles = 300

// DetailOptions says what extras to work out for a detail.
type DetailOptions struct {
	Indicators bool
}

// Detail opens a coin, putting it first in the queue if it is not ready, and
// works out its factors with the same function that made the forecast.
func (e *Engine) Detail(strategy *Strategy, symbol, interval string, prices map[string]float64, options DetailOptions) (*Detail, error) {
	series, err := e.market.Open(symbol, interval)
	if err != nil {
		return nil, err
	}
	detail := &Detail{Symbol: symbol, Interval: interval, Records: []Record{}, Candles: []CandleRow{}, Projection: []CandleRow{}, LastClosed: series.LastClosedTime}
	if price, ok := prices[symbol]; ok {
		detail.Price = &price
	}
	if series.LastClosedTime == 0 {
		detail.Pending = PendingFetching
		if series.FailCount > 0 {
			detail.Pending = PendingUnavailable
		}
		return detail, nil
	}
	step := marketdata.IntervalMillis(interval)
	candles, err := e.market.Store.Range(series.ID, series.LastClosedTime-(detailCandles-1)*step, series.LastClosedTime)
	if err != nil {
		return nil, err
	}
	for _, candle := range candles {
		detail.Candles = append(detail.Candles, CandleRow{Time: candle.OpenTime / 1000, Open: candle.Open, High: candle.High, Low: candle.Low, Close: candle.Close, Volume: candle.Volume})
	}
	if live, ok := e.market.FormingCandle(symbol, interval, series.LastClosedTime); ok {
		detail.Forming = &CandleRow{Time: live.OpenTime / 1000, Open: live.Open, High: live.High, Low: live.Low, Close: live.Close, Volume: live.Volume}
	}
	result, err := e.evaluateAt(strategy, series, series.LastClosedTime)
	switch {
	case errors.Is(err, ErrNotEnoughHistory):
		detail.Pending = PendingNotEnoughHistory
	case err != nil:
		detail.Pending = PendingNotContinuous
	default:
		detail.Ready = true
		detail.Result = &result
	}
	records, err := e.Store.RecentRecords(strategy.ID, series.ID, recentRecordRows)
	if err != nil {
		return nil, err
	}
	detail.Records = records
	if len(records) > 0 {
		detail.Latest = &records[0]
	}
	detail.Stats, err = e.Store.RecordStats(strategy.ID, series.ID)
	if err != nil {
		return nil, err
	}
	if detail.Result != nil {
		prob := detail.Result.ProbFull
		if detail.Latest != nil && detail.Latest.OpenTime == series.LastClosedTime {
			prob = detail.Latest.DisplayProb
		}
		detail.Projection = Project(*detail.Result, prob, series.LastClosedTime, step)
	}
	if options.Indicators {
		if err := e.addIndicators(detail, series, step); err != nil {
			return nil, err
		}
	}
	return detail, nil
}

// addIndicators reads enough candles before the shown ones to warm up, so the first
// point on the chart means the same as the last.
func (e *Engine) addIndicators(detail *Detail, series *marketdata.Series, step int64) error {
	from := series.LastClosedTime - (detailCandles+WindowBars-1)*step
	candles, err := e.market.Store.Range(series.ID, from, series.LastClosedTime)
	if err != nil {
		return err
	}
	indicators := IndicatorSeries(marketdata.ToKlines(candles, series.Interval), detailCandles)
	detail.Indicators = &indicators
	return nil
}

// Overlay is what another page's chart shows of a coin's forecast under the strategy
// in use. It holds less than a Detail, since that chart has candles of its own.
type Overlay struct {
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	// Pending says why there is nothing to show yet.
	Pending     string      `json:"pending,omitempty"`
	Direction   string      `json:"direction,omitempty"`
	DisplayProb float64     `json:"display_prob"`
	Calibrated  bool        `json:"calibrated"`
	Stats       RecordStats `json:"stats"`
	Projection  []CandleRow `json:"projection"`
}

// OverlayFor opens a coin, starting it if it is not tracked, and works out what to
// draw. A series that has fallen behind shows nothing rather than a projection that
// starts in the past.
func (e *Engine) OverlayFor(strategy *Strategy, symbol, interval string) (*Overlay, error) {
	series, err := e.market.Open(symbol, interval)
	if err != nil {
		return nil, err
	}
	overlay := &Overlay{Symbol: symbol, Interval: interval, Projection: []CandleRow{}}
	step := marketdata.IntervalMillis(interval)
	switch {
	case series.LastClosedTime == 0 && series.FailCount > 0:
		overlay.Pending = PendingUnavailable
		return overlay, nil
	case series.LastClosedTime == 0 || e.now().UnixMilli()-series.LastClosedTime > 3*step:
		overlay.Pending = PendingFetching
		return overlay, nil
	}
	result, err := e.evaluateAt(strategy, series, series.LastClosedTime)
	switch {
	case errors.Is(err, ErrNotEnoughHistory):
		overlay.Pending = PendingNotEnoughHistory
		return overlay, nil
	case err != nil:
		overlay.Pending = PendingNotContinuous
		return overlay, nil
	}
	overlay.Direction = result.Direction
	overlay.DisplayProb = result.ProbFull
	records, err := e.Store.RecentRecords(strategy.ID, series.ID, 1)
	if err != nil {
		return nil, err
	}
	if len(records) > 0 && records[0].OpenTime == series.LastClosedTime {
		overlay.DisplayProb = records[0].DisplayProb
		overlay.Calibrated = records[0].Calibrated
	}
	overlay.Stats, err = e.Store.RecordStats(strategy.ID, series.ID)
	if err != nil {
		return nil, err
	}
	overlay.Projection = Project(result, overlay.DisplayProb, series.LastClosedTime, step)
	return overlay, nil
}

// Coverage is what local history holds for a coin at an interval.
type Coverage struct {
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	Candles  int64  `json:"candles"`
	First    int64  `json:"first"`
	Last     int64  `json:"last"`
}

// Coverage reports local history, so a backtest form can say what is missing.
func (e *Engine) Coverage(symbols, intervals []string) ([]Coverage, error) {
	out := []Coverage{}
	for _, symbol := range symbols {
		for _, interval := range intervals {
			row := Coverage{Symbol: symbol, Interval: interval}
			series, err := e.market.Store.FindSeries(symbol, interval)
			if err != nil {
				return nil, err
			}
			if series != nil {
				if row.Candles, err = e.market.Store.CandleCount(series.ID); err != nil {
					return nil, err
				}
				row.First, row.Last = series.FirstOpenTime, series.LastClosedTime
			}
			out = append(out, row)
		}
	}
	return out, nil
}
