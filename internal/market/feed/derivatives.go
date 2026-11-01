package feed

import (
	"context"
	"sync"
	"time"

	"vergex/internal/exchange/binance"
	"vergex/internal/logger"
)

const (
	derivativeTick   = 20 * time.Second
	derivativeSettle = 45 * time.Second // Binance publishes a period's figures a little after it ends.
	fundingCheck     = time.Hour
	derivativeRetry  = 5 * time.Minute
	derivativePage   = 500
	fundingPage      = 200
)

// DerivativeCollector saves funding, open interest and long/short history for
// the tracked coins: every 15 minutes for the watchlist, hourly for the rest.
// Binance keeps open interest and long/short for about 30 days, so what is not
// saved here is only available later from the bulk files.
type DerivativeCollector struct {
	store  *Store
	client *binance.DataClient
	now    func() time.Time

	mu   sync.Mutex
	next map[string]int64
}

// NewDerivativeCollector creates a collector.
func NewDerivativeCollector(store *Store, client *binance.DataClient) *DerivativeCollector {
	return &DerivativeCollector{store: store, client: client, now: time.Now, next: map[string]int64{}}
}

// Run collects until ctx ends.
func (d *DerivativeCollector) Run(ctx context.Context) {
	ticker := time.NewTicker(derivativeTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.Collect(ctx)
		}
	}
}

// Collect makes one pass over the tracked coins.
func (d *DerivativeCollector) Collect(ctx context.Context) {
	settings, err := d.store.LoadSettings()
	if err != nil || !settings.Enabled {
		return
	}
	watch := map[string]bool{}
	symbols := []string{}
	for _, symbol := range settings.Watchlist {
		if !watch[symbol] {
			watch[symbol] = true
			symbols = append(symbols, symbol)
		}
	}
	for _, symbol := range settings.Ranked {
		if !watch[symbol] {
			symbols = append(symbols, symbol)
		}
	}
	for _, symbol := range symbols {
		if ctx.Err() != nil {
			return
		}
		period, length := "1h", time.Hour
		if watch[symbol] {
			period, length = "15m", 15*time.Minute
		}
		d.collectOne(ctx, symbol, KindOpenValue, length, func(start int64) ([]binance.Point, error) {
			return d.client.OpenInterestHistory(ctx, symbol, period, start, derivativePage)
		})
		d.collectOne(ctx, symbol, KindLongShort, length, func(start int64) ([]binance.Point, error) {
			return d.client.LongShortRatio(ctx, symbol, period, start, derivativePage)
		})
		d.collectOne(ctx, symbol, KindFunding, fundingCheck, func(start int64) ([]binance.Point, error) {
			return d.client.FundingHistory(ctx, symbol, start, fundingPage)
		})
	}
}

func (d *DerivativeCollector) collectOne(ctx context.Context, symbol, kind string, period time.Duration, fetch func(start int64) ([]binance.Point, error)) {
	key := symbol + "/" + kind
	now := d.now().UnixMilli()
	d.mu.Lock()
	due, known := d.next[key]
	d.mu.Unlock()
	if known && now < due {
		return
	}
	last, err := d.store.LastDerivativeTime(symbol, kind)
	if err != nil {
		logger.Warnf("Market data: %s %s: %v", symbol, kind, err)
		return
	}
	start := int64(0)
	if last > 0 {
		start = last + 1
		if !known && now < last+period.Milliseconds()+derivativeSettle.Milliseconds() {
			d.setNext(key, last+period.Milliseconds()+derivativeSettle.Milliseconds())
			return
		}
	}
	points, err := fetch(start)
	if err != nil {
		if ctx.Err() == nil {
			logger.Debugf("Market data: %s %s: %v", symbol, kind, err)
			d.setNext(key, now+derivativeRetry.Milliseconds())
		}
		return
	}
	if err := d.store.SaveDerivatives(symbol, kind, points); err != nil {
		logger.Warnf("Market data: save %s %s: %v", symbol, kind, err)
		return
	}
	newest := last
	if len(points) > 0 {
		newest = points[len(points)-1].Time
	}
	next := now + period.Milliseconds()
	if newest > 0 {
		next = newest + period.Milliseconds() + derivativeSettle.Milliseconds()
		if next <= now {
			next = now + period.Milliseconds()
		}
	}
	d.setNext(key, next)
}

func (d *DerivativeCollector) setNext(key string, at int64) {
	d.mu.Lock()
	d.next[key] = at
	d.mu.Unlock()
}
