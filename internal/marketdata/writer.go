package marketdata

import (
	"context"
	"sort"
	"time"

	"vergex/internal/logger"
	"vergex/internal/provider/binance"
	"vergex/internal/provider/wsfeed"
)

const (
	writerFlushEvery = 500 * time.Millisecond
	writerBatch      = 256
)

// Writer stores finished candles from the stream, in batches. It only ever extends
// a series from its water mark: a candle that does not join up is left alone and
// the series falls due, so the hydrator fills the hole over REST and validates what
// is there. A lost connection or a full queue needs no other handling.
type Writer struct {
	store   *Store
	in      <-chan wsfeed.Candle
	now     func() time.Time
	onHole  func() // Told when a series needs the hydrator now.
	written func() // Told after candles were stored.
}

// NewWriter creates a writer. Either callback may be nil.
func NewWriter(store *Store, in <-chan wsfeed.Candle, onHole, written func()) *Writer {
	return &Writer{store: store, in: in, now: time.Now, onHole: onHole, written: written}
}

// Run writes until ctx ends.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(writerFlushEvery)
	defer ticker.Stop()
	var pending []wsfeed.Candle
	flush := func() {
		if len(pending) > 0 {
			w.flush(pending)
			pending = nil
		}
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case candle := <-w.in:
			pending = append(pending, candle)
			if len(pending) >= writerBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (w *Writer) flush(batch []wsfeed.Candle) {
	type seriesName struct{ symbol, interval string }
	groups := map[seriesName][]wsfeed.Candle{}
	for _, candle := range batch {
		name := seriesName{candle.Symbol, candle.Interval}
		groups[name] = append(groups[name], candle)
	}
	hole, wrote := false, false
	for name, candles := range groups {
		sort.Slice(candles, func(i, j int) bool { return candles[i].OpenTime < candles[j].OpenTime })
		seriesWrote, seriesHole, err := w.extend(name.symbol, name.interval, candles)
		if err != nil {
			logger.Warnf("Market data: write %s %s: %v", name.symbol, name.interval, err)
		}
		wrote = wrote || seriesWrote
		hole = hole || seriesHole
	}
	if hole && w.onHole != nil {
		w.onHole()
	}
	if wrote && w.written != nil {
		w.written()
	}
}

// extend adds to a series what follows on from its water mark. Candles for a series
// nobody tracks, such as one only open on a chart, are not stored.
func (w *Writer) extend(symbol, interval string, candles []wsfeed.Candle) (wrote, hole bool, err error) {
	step := IntervalMillis(interval)
	if step == 0 {
		return false, false, nil
	}
	series, err := w.store.FindSeries(symbol, interval)
	if err != nil || series == nil || series.LastClosedTime == 0 {
		return false, false, err // A new series starts from the hydrator's history.
	}

	var keep []binance.Candle
	expected := series.LastClosedTime + step
	for _, candle := range candles {
		if candle.OpenTime < expected {
			continue // Already stored.
		}
		row := toBinanceCandle(candle)
		if candle.OpenTime > expected || validate(row, step) != nil {
			hole = true
			break
		}
		keep = append(keep, row)
		expected += step
	}
	if len(keep) == 0 && !hole {
		return false, false, nil
	}

	now := w.now().UnixMilli()
	updated := *series
	if len(keep) > 0 {
		updated.LastClosedTime = keep[len(keep)-1].OpenTime
	}
	scheduleNext(&updated, now)
	if hole {
		updated.NextDueAt = now
	}
	if err := w.store.appendCandles(&updated, keep, nil); err != nil {
		return false, hole, err
	}
	return len(keep) > 0, hole, nil
}

func toBinanceCandle(c wsfeed.Candle) binance.Candle {
	return binance.Candle{
		OpenTime: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
		QuoteVolume: c.QuoteVolume, Trades: c.Trades, TakerBuyVolume: c.TakerBuyVolume, TakerBuyQuote: c.TakerBuyQuote,
	}
}
