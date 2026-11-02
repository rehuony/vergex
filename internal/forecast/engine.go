package forecast

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"vergex/internal/logger"
	"vergex/internal/market"
	"vergex/internal/market/feed"
	"vergex/internal/service"
)

const (
	engineTick       = 15 * time.Second
	verifyBatch      = 2000
	calibrateEvery   = time.Hour
	maxCatchUpBars   = 8 // After downtime, only the latest candles get a forecast.
	staleBars        = 3 // A series this far behind is no longer current.
	recentSignalRows = 50
)

// Engine stores forecasts as candles close, judges them when their target
// candle closes and sends threshold alerts. It reads only local data.
type Engine struct {
	Store  *Store
	market *feed.Service
	now    func() time.Time
	notify func(ctx context.Context, text string) error

	mu             sync.Mutex
	skipped        map[int64]skip
	backtesting    bool
	backtestRun    int64
	cancelBacktest context.CancelFunc
	calibrated     time.Time
	// Language gives the language alerts are written in. Nil means English.
	Language func() string
}

// skip is why a series has no forecast for its newest candle.
type skip struct {
	pending string
	reason  string
}

// NewEngine creates an engine over the market data service. notify sends alert
// text and may be nil.
func NewEngine(store *Store, service *feed.Service, notify func(ctx context.Context, text string) error) *Engine {
	return &Engine{Store: store, market: service, now: time.Now, notify: notify, skipped: map[int64]skip{}}
}

// Start runs the engine in the background until ctx ends.
func (e *Engine) Start(ctx context.Context) {
	if err := e.Store.failInterruptedBacktests(e.now()); err != nil {
		logger.Warnf("Forecast: %v", err)
	}
	if err := e.Store.carryForward(e.now()); err != nil {
		logger.Warnf("Forecast: carry the strategy in use to this engine: %v", err)
	}
	service.Go("forecast-engine", func() {
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
	if errors.Is(err, ErrNoStrategyInUse) {
		// Nothing new is forecast or alerted, but calls already made are still judged.
		if failed := e.verify(); failed > 0 {
			service.Report(service.Forecast, service.Degraded, fmt.Sprintf("%d signals could not be judged", failed))
		} else {
			service.Report(service.Forecast, service.Off, err.Error())
		}
		return
	}
	if err != nil {
		logger.Warnf("Forecast: read the active strategy: %v", err)
		service.Report(service.Forecast, service.Failed, err.Error())
		return
	}
	created, failed := e.record(strategy)
	failed += e.verify()
	if failed > 0 {
		service.Report(service.Forecast, service.Degraded, fmt.Sprintf("%d series or signals could not be worked out", failed))
	} else {
		service.Report(service.Forecast, service.Running, "")
	}
	e.calibrateIfDue(strategy)
	e.alert(ctx, strategy.Name, created)
}

// record forecasts every candle that closed since the last round. It also reports how
// many failed for a reason waiting will not fix.
func (e *Engine) record(strategy *Strategy) (created []Signal, failed int) {
	series, err := e.market.Store.ListSeries()
	if err != nil {
		logger.Warnf("Forecast: list series: %v", err)
		return nil, 1
	}
	latest, err := e.Store.LatestOpenTimes(strategy.ID)
	if err != nil {
		logger.Warnf("Forecast: read latest signals: %v", err)
		return nil, 1
	}
	curves, err := e.Store.Calibrations(strategy.ID)
	if err != nil {
		logger.Warnf("Forecast: read calibrations: %v", err)
		failed++
	}
	for _, row := range series {
		// Trading keeps series of its own at intervals Forecast has no settings for.
		if row.Status != feed.StatusActive || row.LastClosedTime == 0 || intervalMillis(row.Interval) == 0 {
			continue
		}
		step := feed.IntervalMillis(row.Interval)
		start := row.LastClosedTime
		if last, seen := latest[row.ID]; seen {
			if last >= row.LastClosedTime {
				continue
			}
			start = max(last+step, row.LastClosedTime-(maxCatchUpBars-1)*step)
		}
		for at := start; at <= row.LastClosedTime; at += step {
			signal, err := e.forecast(strategy, row, at, curves[row.Interval])
			if err != nil {
				if e.noteSkipped(row, err) {
					failed++
				}
				continue
			}
			e.clearSkipped(row.ID)
			isNew, err := e.Store.SaveSignal(signal)
			if err != nil {
				logger.Warnf("Forecast: save %s %s: %v", row.Symbol, row.Interval, err)
				failed++
				continue
			}
			if isNew {
				created = append(created, *signal)
			}
		}
	}
	return created, failed
}

// pendingFor says which wait an error stands for, or "" if it is a real failure.
func pendingFor(err error) string {
	switch {
	case errors.Is(err, ErrNotEnoughHistory):
		return PendingNotEnoughHistory
	case errors.Is(err, feed.ErrIncomplete):
		return PendingNotContinuous
	case errors.Is(err, ErrNothingToRead):
		return PendingWaiting
	}
	return ""
}

// noteSkipped remembers why a series has no forecast and says whether it is a real
// failure. A real failure is logged once, not on every round.
func (e *Engine) noteSkipped(row *feed.Series, err error) (failed bool) {
	pending := pendingFor(err)
	failed = pending == ""
	if failed {
		pending = PendingWaiting
	}
	e.mu.Lock()
	held := e.skipped[row.ID]
	e.skipped[row.ID] = skip{pending: pending, reason: err.Error()}
	e.mu.Unlock()
	if failed && held.reason != err.Error() {
		logger.Warnf("Forecast: %s %s: %v", row.Symbol, row.Interval, err)
	}
	return failed
}

func (e *Engine) clearSkipped(seriesID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.skipped, seriesID)
}

// pendingOf says why a series has no forecast yet.
func (e *Engine) pendingOf(row *feed.Series) string {
	if row.LastClosedTime == 0 {
		if row.FailCount > 0 {
			return PendingUnavailable
		}
		return PendingFetching
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if held, known := e.skipped[row.ID]; known {
		return held.pending
	}
	return PendingWaiting
}

// isStale says a candle is further behind than a series that is keeping up can be.
func isStale(openTime, step, now int64) bool {
	return now-openTime > staleBars*step
}

// evaluateAt forecasts a series at one candle from local data.
func (e *Engine) evaluateAt(strategy *Strategy, series *feed.Series, at int64) (Result, error) {
	candles, err := e.market.Store.Window(series, at, WindowBars)
	if err != nil {
		if errors.Is(err, feed.ErrNotEnoughHistory) {
			return Result{}, ErrNotEnoughHistory
		}
		return Result{}, err
	}
	step := feed.IntervalMillis(series.Interval)
	moment := readingTime(at, step)
	readings, err := readingsFrom(e.market.Store, series.Symbol, moment-readingsSpan(step), moment)
	if err != nil {
		return Result{}, err
	}
	return Evaluate(strategy.Config, series.Interval, feed.ToKlines(candles, series.Interval), DerivativesAt(readings, at, step))
}

func readingsFrom(store *feed.Store, symbol string, from, to int64) (Readings, error) {
	var readings Readings
	for _, source := range []struct {
		kind string
		dest *[]Reading
	}{
		{feed.KindFunding, &readings.Funding},
		{feed.KindOpenValue, &readings.OpenValue},
		{feed.KindLongShort, &readings.LongShort},
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

func (e *Engine) forecast(strategy *Strategy, series *feed.Series, at int64, curve []Knot) (*Signal, error) {
	result, err := e.evaluateAt(strategy, series, at)
	if err != nil {
		return nil, err
	}
	return signalFrom(strategy, series, at, result, curve), nil
}

func signalFrom(strategy *Strategy, series *feed.Series, at int64, result Result, curve []Knot) *Signal {
	display := result.ProbFull
	calibrated := len(curve) > 0
	if calibrated {
		display = ApplyCalibration(curve, result.ProbFull)
	}
	return &Signal{
		StrategyID: strategy.ID, SeriesID: series.ID, OpenTime: at, Symbol: series.Symbol, Interval: series.Interval,
		Close: result.Close, ProbCandles: result.ProbCandles, ProbFull: result.ProbFull, DisplayProb: display, Calibrated: calibrated,
		Direction: result.Direction, Confidence: result.Confidence, ExpectedMovePct: result.ExpectedMovePct,
		Support: result.Support, Resistance: result.Resistance, Regime: result.Regime, Horizon: result.Horizon,
		TargetTime: at + int64(result.Horizon)*feed.IntervalMillis(series.Interval), Status: StatusOpen,
	}
}

// currentSignal is the forecast of a series' newest closed candle: what was stored, or
// until the next round stores it, the same thing worked out now. It is the one source
// for what a page shows of a call, so a page and what was stored cannot disagree.
func (e *Engine) currentSignal(strategy *Strategy, series *feed.Series, result Result, stored []Signal) (*Signal, error) {
	if len(stored) > 0 && stored[0].OpenTime == series.LastClosedTime {
		return &stored[0], nil
	}
	curves, err := e.Store.Calibrations(strategy.ID)
	if err != nil {
		return nil, err
	}
	return signalFrom(strategy, series, series.LastClosedTime, result, curves[series.Interval]), nil
}

// verify judges signals whose target candle has closed, and says how many it could not.
func (e *Engine) verify() (failed int) {
	now := e.now().UnixMilli()
	due, err := e.Store.DueSignals(verifyBatch)
	if err != nil {
		logger.Warnf("Forecast: read due signals: %v", err)
		return 1
	}
	for i := range due {
		signal := &due[i]
		candles, err := e.market.Store.Range(signal.SeriesID, signal.TargetTime, signal.TargetTime)
		if err != nil {
			logger.Warnf("Forecast: read target candle: %v", err)
			failed++
			continue
		}
		if len(candles) == 0 {
			signal.Status = StatusVoid
			signal.VerifiedAt = now
		} else {
			Judge(signal, candles[0].Close, now)
		}
		if err := e.Store.saveVerification(signal, feed.IntervalMillis(signal.Interval)); err != nil {
			logger.Warnf("Forecast: save verification: %v", err)
			failed++
		}
	}
	return failed
}

// Judge fills in how a signal turned out, given its target candle's close.
func Judge(signal *Signal, targetClose float64, now int64) {
	signal.Status = StatusVerified
	signal.TargetClose = targetClose
	signal.VerifiedAt = now
	signal.ActualMovePct = (targetClose/signal.Close - 1) * 100
	switch {
	case signal.Direction == DirectionNeutral || targetClose == signal.Close:
		signal.Outcome = OutcomeNeutral
	case (signal.Direction == DirectionUp) == (targetClose > signal.Close):
		signal.Outcome = OutcomeHit
	default:
		signal.Outcome = OutcomeMiss
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

// Calibrate rebuilds an interval's curve from verified forecasts that do not overlap,
// or removes it once those no longer earn one.
func (e *Engine) Calibrate(strategy *Strategy, interval string) error {
	points, err := e.Store.IndependentPoints(strategy.ID, interval)
	if err != nil {
		return err
	}
	knots, earned := EarnCalibration(points)
	if !earned {
		return e.Store.removeCalibration(strategy.ID, interval)
	}
	return e.Store.saveCalibration(Calibration{
		StrategyID: strategy.ID, Interval: interval, Samples: len(points),
		Knots: encodeKnots(knots), BuiltAt: e.now().UnixMilli(),
	})
}

// Score is a probability as the 0-100 number shown to the user.
func Score(prob float64) int { return int(math.Round(prob * 100)) }

// alert sends watchlist calls crossing the threshold, once each time: a series that
// alerted is silent until its score comes back inside. After downtime a series catches
// up on several candles, and only the newest can alert, if it has only just closed.
func (e *Engine) alert(ctx context.Context, strategy string, created []Signal) {
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
	for _, symbol := range marketSettings.Watchlist {
		watched[symbol] = true
	}
	language := "en"
	if e.Language != nil {
		language = e.Language()
	}
	newest := map[int64]*Signal{}
	for i := range created {
		if held := newest[created[i].SeriesID]; held == nil || created[i].OpenTime > held.OpenTime {
			newest[created[i].SeriesID] = &created[i]
		}
	}
	now := e.now().UnixMilli()
	lines := []string{alertHeading(language, strategy)}
	var fired, rearmed []int64
	for i := range created {
		signal := &created[i]
		if newest[signal.SeriesID] != signal || !watched[signal.Symbol] {
			continue
		}
		switch {
		case !beyondThreshold(signal, settings.AlertThreshold):
			rearmed = append(rearmed, signal.SeriesID)
		case slices.Contains(settings.AlertIntervals, signal.Interval) && now < signal.OpenTime+2*intervalMillis(signal.Interval) && !e.Store.alertArm(signal.SeriesID):
			lines = append(lines, alertLine(language, signal))
			fired = append(fired, signal.SeriesID)
		}
	}
	for _, seriesID := range rearmed {
		if err := e.Store.setAlertArm(seriesID, false); err != nil {
			logger.Warnf("Forecast: %v", err)
		}
	}
	if len(fired) == 0 {
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

// beyondThreshold says whether a call's score is past the threshold on the call's own
// side. A neutral call has no side, and a curve can put a score on the other one.
func beyondThreshold(signal *Signal, threshold int) bool {
	score := Score(signal.DisplayProb)
	switch signal.Direction {
	case DirectionUp:
		return score >= threshold
	case DirectionDown:
		return score <= 100-threshold
	}
	return false
}

func alertHeading(language, strategy string) string {
	if language == "zh" {
		return "📈 预测信号（策略：" + strategy + "）"
	}
	return "📈 Forecast signals (strategy: " + strategy + ")"
}

func alertLine(language string, signal *Signal) string {
	score := Score(signal.DisplayProb)
	span := int64(signal.Horizon) * intervalMillis(signal.Interval)
	if language == "zh" {
		direction := "上涨"
		if signal.Direction == DirectionDown {
			direction = "下跌"
		}
		value := fmt.Sprintf("评分 %d", score)
		if signal.Calibrated {
			value = fmt.Sprintf("%d%%", score)
		}
		return fmt.Sprintf("%s %s：%s，%s，约 %s内", signal.Symbol, signal.Interval, direction, value, roughSpan(language, span))
	}
	direction := "Up"
	if signal.Direction == DirectionDown {
		direction = "Down"
	}
	value := fmt.Sprintf("Score %d", score)
	if signal.Calibrated {
		value = fmt.Sprintf("%d%%", score)
	}
	return fmt.Sprintf("%s %s: %s, %s, over about %s", signal.Symbol, signal.Interval, direction, value, roughSpan(language, span))
}

// roughSpan says how long a stretch of time is in the largest unit that reads well, since
// nobody works out four 15m bars in their head.
func roughSpan(language string, millis int64) string {
	minutes := int(math.Round(float64(millis) / 60000))
	amount, unit := minutes, "minute"
	switch {
	case minutes >= 48*60:
		amount, unit = int(math.Round(float64(minutes)/(24*60))), "day"
	case minutes >= 60:
		amount, unit = int(math.Round(float64(minutes)/60)), "hour"
	}
	if language == "zh" {
		return fmt.Sprintf("%d %s", amount, map[string]string{"minute": "分钟", "hour": "小时", "day": "天"}[unit])
	}
	if amount != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s", amount, unit)
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
	return feed.ToKlines(candles, interval), nil
}

// Readings reads local derivatives history, for backtests.
func (e *Engine) Readings(symbol string, from, to int64) (Readings, error) {
	return readingsFrom(e.market.Store, symbol, from, to)
}

// ErrBacktestRunning refuses a second backtest while one runs.
var ErrBacktestRunning = errors.New("a backtest is already running")

// StartBacktest runs a backtest in the background, one at a time. It reads only
// the local store and never requests anything from the exchange.
func (e *Engine) StartBacktest(ctx context.Context, strategyID, name string, options BacktestOptions) (*Backtest, error) {
	strategy, err := e.Store.usableStrategy(strategyID)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	if e.backtesting {
		e.mu.Unlock()
		return nil, ErrBacktestRunning
	}
	e.backtesting = true
	e.mu.Unlock()

	run, err := e.Store.CreateBacktest(strategyID, name, options, e.now())
	if err != nil {
		e.mu.Lock()
		e.backtesting = false
		e.mu.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.backtestRun, e.cancelBacktest = run.ID, cancel
	e.mu.Unlock()
	service.Go("forecast-backtest", func() {
		defer func() {
			cancel()
			e.mu.Lock()
			e.backtesting, e.backtestRun, e.cancelBacktest = false, 0, nil
			e.mu.Unlock()
		}()
		report, err := RunBacktest(ctx, e, strategy.Config, options, func(done, _ int) {
			if err := e.Store.updateBacktestProgress(run.ID, done); err != nil {
				logger.Warnf("Forecast: %v", err)
			}
		})
		saved, failure := &report, ""
		if err != nil {
			saved, failure = nil, err.Error()
			if errors.Is(err, context.Canceled) {
				failure = BacktestCanceled
			}
		}
		if err := e.Store.finishBacktest(run.ID, saved, failure, e.now()); err != nil {
			logger.Warnf("Forecast: save backtest: %v", err)
		}
	})
	return run, nil
}

// CancelBacktest stops a running backtest. It reports false if that run is not going.
func (e *Engine) CancelBacktest(id int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.backtestRun != id || e.cancelBacktest == nil {
		return false
	}
	e.cancelBacktest()
	return true
}

// Why a coin has no forecast yet.
const (
	PendingFetching         = "fetching"
	PendingWaiting          = "waiting"
	PendingNotEnoughHistory = "not_enough_history"
	PendingNotContinuous    = "not_continuous"
	PendingUnavailable      = "unavailable" // The exchange has no such coin.
	PendingNoStrategy       = "no_strategy" // Every strategy has been stopped.
)

// Coin is one row of the coin list: a series and its latest forecast.
type Coin struct {
	Symbol   string   `json:"symbol"`
	Interval string   `json:"interval"`
	Tier     int      `json:"tier"`
	Price    *float64 `json:"price"`
	// Change24hPct is known only while the live stream is up.
	Change24hPct *float64 `json:"change_24h_pct"`
	Signal       *Signal  `json:"signal"`
	Stale        bool     `json:"stale"`
	// Pending says why there is no forecast yet.
	Pending string `json:"pending,omitempty"`
}

// Coins lists every tracked coin at an interval with its latest forecast.
func (e *Engine) Coins(strategy *Strategy, interval string, prices map[string]feed.Quote) ([]Coin, error) {
	series, err := e.market.Store.ListSeries()
	if err != nil {
		return nil, err
	}
	signals, err := e.Store.LatestSignals(strategy.ID, interval)
	if err != nil {
		return nil, err
	}
	bySeries := make(map[int64]*Signal, len(signals))
	for i := range signals {
		bySeries[signals[i].SeriesID] = &signals[i]
	}
	now := e.now().UnixMilli()
	step := intervalMillis(interval)
	coins := []Coin{}
	for _, row := range series {
		if row.Status != feed.StatusActive || row.Interval != interval {
			continue
		}
		coin := Coin{Symbol: row.Symbol, Interval: interval, Tier: row.Tier, Signal: bySeries[row.ID]}
		if quote, ok := prices[row.Symbol]; ok {
			coin.Price, coin.Change24hPct = &quote.Price, quote.Change24hPct
		}
		if coin.Signal != nil {
			coin.Stale = isStale(coin.Signal.OpenTime, step, now)
		} else {
			coin.Pending = e.pendingOf(row)
		}
		coins = append(coins, coin)
	}
	return coins, nil
}

// Detail is everything the forecast page shows for one coin at one interval.
type Detail struct {
	Symbol   string   `json:"symbol"`
	Interval string   `json:"interval"`
	Price    *float64 `json:"price"`
	// Change24hPct is known only while the live stream is up.
	Change24hPct *float64 `json:"change_24h_pct"`
	Ready        bool     `json:"ready"`
	Pending      string   `json:"pending,omitempty"`
	// Stale says the newest candle is further behind than a series keeping up can be.
	Stale bool `json:"stale"`
	// Latest is the call to show: the forecast of the newest closed candle, or while
	// there is none, the newest on record.
	Latest *Signal `json:"latest"`
	// Result holds what a signal does not keep, such as its factors.
	Result     *Result     `json:"result"`
	Signals    []Signal    `json:"signals"`
	Stats      SignalStats `json:"stats"`
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
func (e *Engine) Detail(strategy *Strategy, symbol, interval string, prices map[string]feed.Quote, options DetailOptions) (*Detail, error) {
	series, err := e.market.Open(symbol, interval)
	if err != nil {
		return nil, err
	}
	detail := &Detail{Symbol: symbol, Interval: interval, Signals: []Signal{}, Candles: []CandleRow{}, Projection: []CandleRow{}, LastClosed: series.LastClosedTime}
	if quote, ok := prices[symbol]; ok {
		detail.Price, detail.Change24hPct = &quote.Price, quote.Change24hPct
	}
	if series.LastClosedTime == 0 {
		detail.Pending = e.pendingOf(series)
		return detail, nil
	}
	step := feed.IntervalMillis(interval)
	detail.Stale = isStale(series.LastClosedTime, step, e.now().UnixMilli())
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
	signals, err := e.Store.RecentSignals(strategy.ID, series.ID, recentSignalRows)
	if err != nil {
		return nil, err
	}
	detail.Signals = signals
	if len(signals) > 0 {
		detail.Latest = &signals[0]
	}
	detail.Stats, err = e.Store.SignalStats(strategy.ID, series.ID)
	if err != nil {
		return nil, err
	}
	result, err := e.evaluateAt(strategy, series, series.LastClosedTime)
	if pending := pendingFor(err); pending != "" {
		detail.Pending = pending
	} else if err != nil {
		return nil, err
	} else {
		detail.Ready = true
		detail.Result = &result
		if detail.Latest, err = e.currentSignal(strategy, series, result, signals); err != nil {
			return nil, err
		}
		detail.Projection = Project(*detail.Latest, step)
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
func (e *Engine) addIndicators(detail *Detail, series *feed.Series, step int64) error {
	from := series.LastClosedTime - (detailCandles+WindowBars-1)*step
	candles, err := e.market.Store.Range(series.ID, from, series.LastClosedTime)
	if err != nil {
		return err
	}
	indicators := IndicatorSeries(feed.ToKlines(candles, series.Interval), detailCandles)
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
	Stats       SignalStats `json:"stats"`
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
	step := feed.IntervalMillis(interval)
	switch {
	case series.LastClosedTime == 0:
		overlay.Pending = e.pendingOf(series)
		return overlay, nil
	case isStale(series.LastClosedTime, step, e.now().UnixMilli()):
		overlay.Pending = PendingFetching
		return overlay, nil
	}
	result, err := e.evaluateAt(strategy, series, series.LastClosedTime)
	if pending := pendingFor(err); pending != "" {
		overlay.Pending = pending
		return overlay, nil
	} else if err != nil {
		return nil, err
	}
	stored, err := e.Store.RecentSignals(strategy.ID, series.ID, 1)
	if err != nil {
		return nil, err
	}
	latest, err := e.currentSignal(strategy, series, result, stored)
	if err != nil {
		return nil, err
	}
	overlay.Direction, overlay.DisplayProb, overlay.Calibrated = latest.Direction, latest.DisplayProb, latest.Calibrated
	overlay.Stats, err = e.Store.SignalStats(strategy.ID, series.ID)
	if err != nil {
		return nil, err
	}
	overlay.Projection = Project(*latest, step)
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
