package marketdata

import (
	"context"
	"errors"
	"sync"
	"time"

	"vergex/internal/logger"
	"vergex/internal/provider/binance"
	"vergex/internal/safe"
)

const priceCacheTTL = 10 * time.Second

// Status is what the settings page and Dashboard show about collection.
type Status struct {
	Enabled      bool                  `json:"enabled"`
	Series       int                   `json:"series"`
	Ready        int                   `json:"ready"`
	Behind       int                   `json:"behind"`
	Failing      int                   `json:"failing"`
	LastError    string                `json:"last_error,omitempty"`
	WatchSymbols int                   `json:"watch_symbols"`
	TopSymbols   int                   `json:"top_symbols"`
	Limiter      binance.LimiterStatus `json:"limiter"`
	Backfill     BackfillProgress      `json:"backfill"`
}

// Service ties collection together for the rest of the application.
type Service struct {
	Store      *Store
	client     *binance.DataClient
	hydrator   *Hydrator
	collector  *DerivativeCollector
	backfiller *Backfiller
	now        func() time.Time

	mu       sync.Mutex
	backfill BackfillProgress
	prices   map[string]float64
	pricedAt time.Time
}

// NewService creates the service over a market store.
func NewService(store *Store) *Service {
	client := binance.NewDataClient(binance.NewLimiter())
	return &Service{
		Store:      store,
		client:     client,
		hydrator:   NewHydrator(store, client, binance.Default()),
		collector:  NewDerivativeCollector(store, client),
		backfiller: NewBackfiller(store, client),
		now:        time.Now,
	}
}

// Start begins collecting in the background until ctx ends.
func (s *Service) Start(ctx context.Context) {
	safe.GoNamed("market-data-hydrator", func() { s.hydrator.Run(ctx) })
	safe.GoNamed("market-data-derivatives", func() { s.collector.Run(ctx) })
}

// Client returns the rate-limited exchange client.
func (s *Service) Client() *binance.DataClient { return s.client }

// Status summarizes collection.
func (s *Service) Status() (Status, error) {
	settings, err := s.Store.LoadSettings()
	if err != nil {
		return Status{}, err
	}
	series, err := s.Store.ListSeries()
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Enabled:      settings.Enabled,
		WatchSymbols: len(settings.WatchSymbols()),
		TopSymbols:   len(settings.RankedSymbols()),
		Limiter:      s.client.Limiter().Status(),
	}
	now := s.now().UnixMilli()
	for _, row := range series {
		if row.Status != StatusActive {
			continue
		}
		status.Series++
		if row.LastClosedTime > 0 {
			status.Ready++
		}
		if closedBehind(row, now-settleDelay.Milliseconds()) > 0 {
			status.Behind++
		}
		if row.FailCount > 0 {
			status.Failing++
			status.LastError = row.LastError
		}
	}
	s.mu.Lock()
	status.Backfill = s.backfill
	s.mu.Unlock()
	return status, nil
}

// SaveSettings stores the settings and replans at once.
func (s *Service) SaveSettings(enabled bool, watchlist []string, topN int) error {
	if err := s.Store.SaveSettings(enabled, watchlist, topN); err != nil {
		return err
	}
	s.hydrator.Replan()
	return nil
}

// Open is called when the user looks at a series: it starts or refreshes it at
// the front of the queue and returns it, with whatever data exists so far.
func (s *Service) Open(symbol, interval string) (*Series, error) {
	if IntervalMillis(interval) == 0 {
		return nil, errors.New("unsupported interval")
	}
	now := s.now().UnixMilli()
	series, err := s.Store.EnsureSeries(symbol, interval, TierOnDemand, now)
	if err != nil {
		return nil, err
	}
	if err := s.Store.markOpened(series, now); err != nil {
		return nil, err
	}
	s.hydrator.Wake()
	return series, nil
}

// Prices returns the latest prices, read from the exchange at most every 10
// seconds and only while something is asking.
func (s *Service) Prices(ctx context.Context) (map[string]float64, error) {
	s.mu.Lock()
	if s.prices != nil && s.now().Sub(s.pricedAt) < priceCacheTTL {
		defer s.mu.Unlock()
		return s.prices, nil
	}
	s.mu.Unlock()
	prices, err := s.client.TickerPrices(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.prices, s.pricedAt = prices, s.now()
	s.mu.Unlock()
	return prices, nil
}

// StartBackfill runs one backfill in the background. A second one is refused
// while the first is running.
func (s *Service) StartBackfill(ctx context.Context, options BackfillOptions) error {
	s.mu.Lock()
	if s.backfill.Running {
		s.mu.Unlock()
		return errors.New("a backfill is already running")
	}
	s.backfill = BackfillProgress{Running: true}
	s.mu.Unlock()
	safe.GoNamed("market-data-backfill", func() {
		err := s.backfiller.Run(ctx, options, func(progress BackfillProgress) {
			s.mu.Lock()
			s.backfill = progress
			s.mu.Unlock()
		})
		s.mu.Lock()
		s.backfill.Running = false
		s.backfill.Current = ""
		s.backfill.FinishedAt = s.now().UnixMilli()
		if err != nil {
			s.backfill.Error = err.Error()
			logger.Warnf("Market data: backfill: %v", err)
		}
		s.mu.Unlock()
		s.hydrator.Wake()
	})
	return nil
}

// RunBackfill runs a backfill and waits for it, for the command line.
func (s *Service) RunBackfill(ctx context.Context, options BackfillOptions, report func(BackfillProgress)) error {
	return s.backfiller.Run(ctx, options, report)
}
