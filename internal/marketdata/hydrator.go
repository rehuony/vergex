package marketdata

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"vergex/internal/logger"
	"vergex/internal/provider/binance"
	"vergex/internal/servicestate"
)

const (
	bootstrapBars = 300 // Enough for every factor, at weight 2.
	tailLimit     = 99  // The most candles a request of weight 1 returns.
	pageLimit     = 499 // The most candles a request of weight 2 returns.
	settleDelay   = 3 * time.Second

	planInterval  = time.Minute
	rankInterval  = 24 * time.Hour
	purgeInterval = 24 * time.Hour
	rankExitExtra = 30 // A coin leaves the ranking only past N+30, so the sample stays steady.
	// onDemandKeep is how long a series opened by the user stays up to date.
	onDemandKeep = time.Hour
	maxBackoff   = 30 * time.Minute
)

// Ranker supplies coins by volume, highest first.
type Ranker interface {
	TopVolume(ctx context.Context) ([]binance.Ticker, error)
}

// Hydrator keeps the wanted series filled. It is the only thing here that asks
// the exchange for candles, and it works through one series at a time.
type Hydrator struct {
	store  *Store
	client *binance.DataClient
	ranker Ranker
	now    func() time.Time
	wake   chan struct{}
	replan atomic.Bool
	// streamDown says the live stream should be up and is not. Collection carries on
	// over REST, so this only shows as degraded.
	streamDown func() bool
}

// NewHydrator creates a hydrator.
func NewHydrator(store *Store, client *binance.DataClient, ranker Ranker) *Hydrator {
	return &Hydrator{store: store, client: client, ranker: ranker, now: time.Now, wake: make(chan struct{}, 1), streamDown: func() bool { return false }}
}

func (h *Hydrator) nowMillis() int64 { return h.now().UnixMilli() }

// Wake asks the hydrator to look for work now.
func (h *Hydrator) Wake() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// Replan asks the hydrator to work out its series again, after a settings change.
func (h *Hydrator) Replan() {
	h.replan.Store(true)
	h.Wake()
}

// Run works until ctx ends.
func (h *Hydrator) Run(ctx context.Context) {
	var lastPlan, lastPurge time.Time
	active := false
	for ctx.Err() == nil {
		if h.replan.Swap(false) || h.now().Sub(lastPlan) >= planInterval {
			var err error
			active, err = h.Plan(ctx)
			if err != nil {
				logger.Warnf("Market data: plan: %v", err)
			}
			lastPlan = h.now()
		}
		if h.now().Sub(lastPurge) >= purgeInterval {
			if err := h.store.purgeShortCandles(h.now().Add(-shortCandleKeep).UnixMilli()); err != nil {
				logger.Warnf("Market data: purge short candles: %v", err)
			}
			lastPurge = h.now()
		}
		if !active {
			servicestate.Set(servicestate.MarketData, servicestate.Off, "")
			h.sleep(ctx, planInterval)
			lastPlan = time.Time{} // What woke it may be a series just opened.
			continue
		}
		h.reportState()

		series, err := h.store.NextDue(h.nowMillis())
		if err != nil {
			logger.Warnf("Market data: read due series: %v", err)
			h.sleep(ctx, 10*time.Second)
			continue
		}
		if series == nil {
			h.sleep(ctx, h.idleWait())
			continue
		}
		h.Sync(ctx, series)
	}
}

func (h *Hydrator) sleep(ctx context.Context, wait time.Duration) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-h.wake:
	case <-timer.C:
	}
}

// idleWait sleeps until the next series falls due, but no longer than a plan.
func (h *Hydrator) idleWait() time.Duration {
	wait := planInterval
	if earliest, err := h.store.EarliestDue(); err == nil && earliest > 0 {
		if until := time.Duration(earliest-h.nowMillis()) * time.Millisecond; until < wait {
			wait = until
		}
	}
	if wait < time.Second {
		wait = time.Second
	}
	return wait
}

func (h *Hydrator) reportState() {
	status := h.client.Limiter().Status()
	switch {
	case status.Banned:
		servicestate.Set(servicestate.MarketData, servicestate.Failed, status.Reason)
	case !status.PausedUntil.IsZero():
		servicestate.Set(servicestate.MarketData, servicestate.Degraded, status.Reason)
	case h.streamDown():
		servicestate.Set(servicestate.MarketData, servicestate.Degraded, "live stream disconnected, using REST")
	default:
		servicestate.Set(servicestate.MarketData, servicestate.Running, "")
	}
}

// Plan works out which series should exist and brings the table in line. Series
// no longer wanted turn inactive and keep their data. Turning collection off stops
// the lists, not what trading or the user has open. It reports whether any series
// needs work.
func (h *Hydrator) Plan(ctx context.Context) (bool, error) {
	settings, err := h.store.LoadSettings()
	if err != nil {
		return false, err
	}
	now := h.nowMillis()

	wanted := map[string]int{}
	key := func(symbol, interval string) string { return symbol + "/" + interval }
	if settings.Enabled {
		for _, symbol := range h.ranking(ctx, settings, now) {
			for _, interval := range TopIntervals {
				wanted[key(symbol, interval)] = TierTop
			}
		}
		for _, symbol := range settings.WatchSymbols() {
			for _, interval := range WatchIntervals {
				wanted[key(symbol, interval)] = TierWatch
			}
		}
	}

	existing, err := h.store.ListSeries()
	if err != nil {
		return settings.Enabled, err
	}
	active := settings.Enabled
	seen := map[string]bool{}
	for _, series := range existing {
		id := key(series.Symbol, series.Interval)
		seen[id] = true
		tier, want := wanted[id]
		if !want && now-series.LastOpenedAt < onDemandKeep.Milliseconds() {
			tier, want = TierOnDemand, true
		}
		status := StatusInactive
		if want {
			status = StatusActive
			active = true
		} else {
			tier = series.Tier
		}
		if tier == series.Tier && status == series.Status {
			continue
		}
		series.Tier, series.Status = tier, status
		if status == StatusActive && series.NextDueAt < now {
			series.NextDueAt = now
		}
		series.Priority = seriesPriority(series, now)
		series.UpdatedAt = now
		if err := h.store.saveSeries(series); err != nil {
			return active, err
		}
	}
	for id, tier := range wanted {
		if seen[id] {
			continue
		}
		symbol, interval := splitKey(id)
		if _, err := h.store.EnsureSeries(symbol, interval, tier, now); err != nil {
			return active, err
		}
	}
	return active, nil
}

func splitKey(id string) (string, string) {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '/' {
			return id[:i], id[i+1:]
		}
	}
	return id, ""
}

// ranking returns the high-volume coins, refreshed at most once a day. A failure
// keeps the last known list rather than dropping everything.
func (h *Hydrator) ranking(ctx context.Context, settings Settings, now int64) []string {
	current := settings.RankedSymbols()
	if settings.TopN <= 0 {
		return nil
	}
	if settings.RankedAt > 0 && now-settings.RankedAt < rankInterval.Milliseconds() {
		return current
	}
	tickers, err := h.ranker.TopVolume(ctx)
	if err != nil {
		logger.Warnf("Market data: rank coins: %v", err)
		return current
	}
	next := RankSymbols(tickers, current, settings.TopN)
	if err := h.store.saveRanking(next, now); err != nil {
		logger.Warnf("Market data: save ranking: %v", err)
	}
	return next
}

// RankSymbols applies the entry and exit thresholds: a coin enters within the
// top N and leaves only past N+30.
func RankSymbols(tickers []binance.Ticker, current []string, n int) []string {
	held := map[string]bool{}
	for _, symbol := range current {
		held[symbol] = true
	}
	next := []string{}
	for i, ticker := range tickers {
		rank := i + 1
		if rank <= n || (rank <= n+rankExitExtra && held[ticker.Symbol]) {
			next = append(next, ticker.Symbol)
		}
	}
	sort.Strings(next)
	return next
}

// intervalRank orders a list's intervals, shorter first.
var intervalRank = map[string]int{"30m": 1, "1h": 1, "2h": 2, "4h": 2, "6h": 2, "8h": 2, "12h": 2, "1d": 3}

// seriesPriority orders work; lower goes first: what the user is waiting for,
// the watchlist's latest candles, the high-volume coins' latest candles, new
// series, then long catch-ups.
func seriesPriority(series *Series, now int64) int {
	rank := intervalRank[series.Interval]
	step := IntervalMillis(series.Interval)
	waiting := series.LastOpenedAt > 0 && now-series.LastOpenedAt < (2*time.Minute).Milliseconds()
	if waiting && (series.LastClosedTime == 0 || closedBehind(series, now) > 0) {
		return 0
	}
	if series.LastClosedTime == 0 {
		return 30 + rank
	}
	if step > 0 && closedBehind(series, now) > tailLimit {
		return 40 + rank
	}
	if series.Tier == TierWatch {
		return 10 + rank
	}
	return 20 + rank
}

// closedBehind counts final candles past the water mark. See dropUnclosed.
func closedBehind(series *Series, now int64) int64 {
	step := IntervalMillis(series.Interval)
	if step == 0 || series.LastClosedTime == 0 {
		return 0
	}
	behind := (now-settleDelay.Milliseconds()-series.LastClosedTime)/step - 1
	if behind < 0 {
		return 0
	}
	return behind
}

// Sync fetches what one series is missing and schedules its next turn. The
// stored row changes only after the candles are safely written, so a failure
// anywhere leaves the water mark where it was.
func (h *Hydrator) Sync(ctx context.Context, series *Series) {
	step := IntervalMillis(series.Interval)
	now := h.nowMillis()
	if step == 0 {
		series.Status = StatusInactive
		series.LastError = "unsupported interval"
		_ = h.store.saveSeries(series)
		return
	}

	var (
		candles []binance.Candle
		err     error
		start   int64
	)
	if series.LastClosedTime == 0 {
		candles, err = h.client.Klines(ctx, series.Symbol, series.Interval, 0, bootstrapBars)
	} else {
		start = series.LastClosedTime + step
		behind := closedBehind(series, now)
		if behind <= 0 {
			h.schedule(series, now)
			return
		}
		limit := pageLimit
		if behind < tailLimit {
			limit = int(behind) + 1 // One more, for the candle still open.
		}
		candles, err = h.client.Klines(ctx, series.Symbol, series.Interval, start, limit)
	}
	if err != nil {
		h.fail(ctx, series, err, now)
		return
	}
	closed := dropUnclosed(candles, step, now)
	if len(closed) == 0 {
		// A new listing, or the exchange has nothing yet.
		h.retryAfterClose(series, now)
		return
	}

	updated := *series
	if start == 0 {
		start = closed[0].OpenTime
		updated.FirstOpenTime = start
	}
	keep, resume, err := contiguous(closed, start, step)
	if err != nil {
		h.fail(ctx, series, err, now)
		return
	}
	if len(keep) > 0 {
		updated.LastClosedTime = keep[len(keep)-1].OpenTime
	}
	updated.FailCount, updated.LastError = 0, ""

	var gap *Gap
	if resume > 0 {
		// A hole in the answer: ask once more before believing it.
		from := start + int64(len(keep))*step
		confirmed, err := h.confirmGap(ctx, series, from, resume, step)
		if err != nil {
			h.fail(ctx, series, err, now)
			return
		}
		if confirmed {
			gap = &Gap{SeriesID: series.ID, FromTime: from, ToTime: resume - step}
			updated.LastClosedTime = resume - step
		} else if len(keep) == 0 {
			h.fail(ctx, series, errors.New("candles came back with a hole that did not repeat"), now)
			return
		}
	}
	scheduleNext(&updated, now)
	if err := h.store.appendCandles(&updated, keep, gap); err != nil {
		h.fail(ctx, series, err, now)
	}
}

func (h *Hydrator) confirmGap(ctx context.Context, series *Series, expected, next, step int64) (bool, error) {
	again, err := h.client.Klines(ctx, series.Symbol, series.Interval, expected, 2)
	if err != nil {
		return false, err
	}
	again = dropUnclosed(again, step, h.nowMillis())
	return len(again) > 0 && again[0].OpenTime == next, nil
}

// scheduleNext sets a series' next turn: now if it is behind, otherwise just after
// the next close. A live stream normally moves it on before then, so a series on
// the stream never costs a request.
func scheduleNext(series *Series, now int64) {
	step := IntervalMillis(series.Interval)
	if closedBehind(series, now) > 0 {
		series.NextDueAt = now
	} else {
		series.NextDueAt = series.LastClosedTime + 2*step + settleDelay.Milliseconds()
	}
	series.Priority = seriesPriority(series, now)
	series.UpdatedAt = now
}

func (h *Hydrator) schedule(series *Series, now int64) {
	scheduleNext(series, now)
	if err := h.store.saveSeries(series); err != nil {
		logger.Warnf("Market data: save %s %s: %v", series.Symbol, series.Interval, err)
	}
}

// retryAfterClose handles a series with nothing new: try after the next close.
func (h *Hydrator) retryAfterClose(series *Series, now int64) {
	step := IntervalMillis(series.Interval)
	series.NextDueAt = (now/step+1)*step + settleDelay.Milliseconds()
	series.Priority = seriesPriority(series, now)
	series.UpdatedAt = now
	if err := h.store.saveSeries(series); err != nil {
		logger.Warnf("Market data: save %s %s: %v", series.Symbol, series.Interval, err)
	}
}

// fail backs off this one series. A rate limit answer is not its fault: the
// limiter already paused everything, so the series simply waits its turn.
func (h *Hydrator) fail(ctx context.Context, series *Series, err error, now int64) {
	if ctx.Err() != nil {
		return
	}
	if binance.RateLimited(err) {
		series.NextDueAt = now
		_ = h.store.saveSeries(series)
		return
	}
	series.FailCount++
	series.LastError = truncateError(err)
	wait := time.Duration(math.Pow(2, float64(min(series.FailCount, 10)))) * 15 * time.Second
	if wait > maxBackoff {
		wait = maxBackoff
	}
	series.NextDueAt = now + wait.Milliseconds()
	series.UpdatedAt = now
	if saveErr := h.store.saveSeries(series); saveErr != nil {
		logger.Warnf("Market data: save %s %s: %v", series.Symbol, series.Interval, saveErr)
	}
	logger.Warnf("Market data: sync %s %s: %v", series.Symbol, series.Interval, err)
}

// truncateError shortens an error for storage without cutting a character in two,
// which PostgreSQL would reject.
func truncateError(err error) string {
	text := err.Error()
	if len(text) > 200 {
		text = text[:200]
	}
	return strings.ToValidUTF8(text, "")
}

// dropUnclosed removes candles not yet final at now. Only final ones are stored,
// and a candle read in the moments after its close can still be missing its last
// trades, so it counts as final only once settleDelay has passed.
func dropUnclosed(candles []binance.Candle, step, now int64) []binance.Candle {
	end := len(candles)
	for end > 0 && candles[end-1].OpenTime+step+settleDelay.Milliseconds() > now {
		end--
	}
	return candles[:end]
}

// contiguous takes the candles that follow on from start without a hole. A hole
// returns where the data resumes; bad data is an error.
func contiguous(candles []binance.Candle, start, step int64) (keep []binance.Candle, resume int64, err error) {
	expected := start
	for _, candle := range candles {
		switch {
		case candle.OpenTime < expected:
			continue
		case candle.OpenTime > expected:
			return keep, candle.OpenTime, nil
		}
		if err := validate(candle, step); err != nil {
			return keep, 0, err
		}
		keep = append(keep, candle)
		expected += step
	}
	return keep, 0, nil
}

func validate(candle binance.Candle, step int64) error {
	if candle.OpenTime%step != 0 {
		return fmt.Errorf("candle at %d is not on an interval boundary", candle.OpenTime)
	}
	if candle.Low <= 0 || candle.High < candle.Low || candle.Open < candle.Low || candle.Open > candle.High ||
		candle.Close < candle.Low || candle.Close > candle.High || candle.Volume < 0 {
		return errors.New("candle has impossible prices")
	}
	return nil
}
