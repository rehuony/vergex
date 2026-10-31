// Package marketdata keeps candles and derivatives data in a database of its
// own and fills it within Binance's rate limits. Calculations read only this
// local store; only the hydrator talks to the exchange.
package marketdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vergex/internal/provider/binance"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const ExchangeBinance = "binance"

// Series tiers: which list keeps a series up to date.
const (
	TierWatch    = 1 // On the user's watchlist.
	TierTop      = 2 // Among the highest-volume coins.
	TierOnDemand = 3 // Opened by the user; kept up to date for a while.
)

// Series states.
const (
	StatusActive   = "active"
	StatusInactive = "inactive" // No longer tracked; its data stays.
)

// Series is one exchange, symbol and interval, and how far it has been filled.
type Series struct {
	ID       int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Exchange string `gorm:"not null;uniqueIndex:idx_market_series_key,priority:1" json:"exchange"`
	Symbol   string `gorm:"not null;uniqueIndex:idx_market_series_key,priority:2" json:"symbol"`
	Interval string `gorm:"column:interval_name;not null;uniqueIndex:idx_market_series_key,priority:3" json:"interval"`
	Tier     int    `gorm:"not null" json:"tier"`
	Status   string `gorm:"not null;index:idx_market_series_due,priority:1" json:"status"`
	Priority int    `gorm:"not null;index:idx_market_series_due,priority:2" json:"priority"`
	// NextDueAt is when this series should next be synced, in Unix milliseconds.
	NextDueAt int64 `gorm:"not null;index:idx_market_series_due,priority:3" json:"next_due_at"`
	// FirstOpenTime is the oldest stored candle's open time.
	FirstOpenTime int64 `gorm:"not null" json:"first_open_time"`
	// LastClosedTime is the water mark: the open time of the newest candle that
	// has closed and joins the data before it without an unexplained hole.
	LastClosedTime int64  `gorm:"not null" json:"last_closed_time"`
	FailCount      int    `gorm:"not null" json:"fail_count"`
	LastError      string `json:"last_error,omitempty"`
	// LastOpenedAt is when the user last asked for this series.
	LastOpenedAt int64 `gorm:"not null" json:"last_opened_at"`
	UpdatedAt    int64 `gorm:"not null" json:"updated_at"`
}

func (Series) TableName() string { return "market_series" }

// Candle is a closed candle.
type Candle struct {
	SeriesID       int64   `gorm:"primaryKey;autoIncrement:false"`
	OpenTime       int64   `gorm:"primaryKey;autoIncrement:false"`
	Open           float64 `gorm:"not null"`
	High           float64 `gorm:"not null"`
	Low            float64 `gorm:"not null"`
	Close          float64 `gorm:"not null"`
	Volume         float64 `gorm:"not null"`
	QuoteVolume    float64 `gorm:"not null"`
	Trades         int64   `gorm:"not null"`
	TakerBuyVolume float64 `gorm:"not null"`
	TakerBuyQuote  float64 `gorm:"not null"`
}

func (Candle) TableName() string { return "market_candles" }

// Gap is a range the exchange confirmed it has no candles for, so it is not
// requested again and does not count as missing data.
type Gap struct {
	SeriesID int64 `gorm:"primaryKey;autoIncrement:false"`
	FromTime int64 `gorm:"primaryKey;autoIncrement:false"`
	ToTime   int64 `gorm:"not null"`
}

func (Gap) TableName() string { return "market_gaps" }

// Derivative kinds.
const (
	KindFunding   = "funding"
	KindOpenValue = "open_interest"
	KindLongShort = "long_short"
)

// Derivative is one timed reading of a derivatives series.
type Derivative struct {
	Symbol string  `gorm:"primaryKey;autoIncrement:false"`
	Kind   string  `gorm:"primaryKey;autoIncrement:false"`
	Time   int64   `gorm:"primaryKey;autoIncrement:false"`
	Value  float64 `gorm:"not null"`
}

func (Derivative) TableName() string { return "market_derivatives" }

// Settings is what the user chose to collect; a single row.
type Settings struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	Enabled   bool   `json:"enabled"`
	Watchlist string `json:"-"`
	TopN      int    `json:"top_n"`
	// Ranked holds the current high-volume coins and RankedAt when they were last
	// worked out; they change slowly so the sample stays steady.
	Ranked   string `json:"-"`
	RankedAt int64  `json:"-"`
}

func (Settings) TableName() string { return "market_settings" }

// WatchSymbols returns the watchlist as symbols.
func (s Settings) WatchSymbols() []string { return splitList(s.Watchlist) }

// RankedSymbols returns the current high-volume coins.
func (s Settings) RankedSymbols() []string { return splitList(s.Ranked) }

func splitList(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, ",")
}

// DefaultWatchlist is what a new installation watches until the user edits it.
var DefaultWatchlist = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "LINKUSDT", "AVAXUSDT", "SUIUSDT"}

// DefaultTopN is how many high-volume coins are tracked by default.
const DefaultTopN = 200

// WatchIntervals and TopIntervals are the intervals kept for each list.
var (
	WatchIntervals = []string{"15m", "1h", "4h", "1d"}
	TopIntervals   = []string{"1h", "4h", "1d"}
)

var intervalMillis = map[string]int64{
	"15m": 15 * 60 * 1000,
	"1h":  60 * 60 * 1000,
	"4h":  4 * 60 * 60 * 1000,
	"1d":  24 * 60 * 60 * 1000,
}

// IntervalMillis returns an interval's length, or 0 if it is not supported.
func IntervalMillis(interval string) int64 { return intervalMillis[interval] }

// Store is the market database.
type Store struct {
	db *gorm.DB
}

// Open opens the SQLite market database at path, separate from the main one so
// bulk candle writes never queue up orders and positions. WAL and
// synchronous=NORMAL suit data that can be fetched again.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create market data directory: %w", err)
	}
	dsn := "file:" + path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open market database: %w", err)
	}
	return NewStore(db)
}

// NewStore prepares the market tables on a connection, including a PostgreSQL
// one kept apart from the main connection pool.
func NewStore(db *gorm.DB) (*Store, error) {
	if err := db.AutoMigrate(&Series{}, &Candle{}, &Gap{}, &Derivative{}, &Settings{}); err != nil {
		return nil, fmt.Errorf("migrate market tables: %w", err)
	}
	return &Store{db: db}, nil
}

// DB exposes the connection for packages keeping their tables beside the candles.
func (s *Store) DB() *gorm.DB { return s.db }

// Close closes the connection.
func (s *Store) Close() error {
	sql, err := s.db.DB()
	if err != nil {
		return err
	}
	return sql.Close()
}

// LoadSettings returns the settings, creating the defaults the first time.
func (s *Store) LoadSettings() (Settings, error) {
	var settings Settings
	err := s.db.First(&settings, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		settings = Settings{ID: 1, Enabled: true, Watchlist: strings.Join(DefaultWatchlist, ","), TopN: DefaultTopN}
		return settings, s.db.Create(&settings).Error
	}
	return settings, err
}

// SaveSettings stores what the user chose. Changing the high-volume count makes
// the ranking run again on the next plan.
func (s *Store) SaveSettings(enabled bool, watchlist []string, topN int) error {
	current, err := s.LoadSettings()
	if err != nil {
		return err
	}
	updates := map[string]any{"enabled": enabled, "watchlist": strings.Join(watchlist, ","), "top_n": topN}
	if topN != current.TopN {
		updates["ranked_at"] = int64(0)
	}
	return s.db.Model(&Settings{}).Where("id = 1").Updates(updates).Error
}

func (s *Store) saveRanking(symbols []string, at int64) error {
	return s.db.Model(&Settings{}).Where("id = 1").Updates(map[string]any{"ranked": strings.Join(symbols, ","), "ranked_at": at}).Error
}

// FindSeries returns one series, or nil if it does not exist yet.
func (s *Store) FindSeries(symbol, interval string) (*Series, error) {
	var series Series
	err := s.db.Where("exchange = ? AND symbol = ? AND interval_name = ?", ExchangeBinance, symbol, interval).First(&series).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &series, nil
}

// EnsureSeries returns the series, creating it due now if it is new.
func (s *Store) EnsureSeries(symbol, interval string, tier int, now int64) (*Series, error) {
	series, err := s.FindSeries(symbol, interval)
	if err != nil || series != nil {
		return series, err
	}
	series = &Series{Exchange: ExchangeBinance, Symbol: symbol, Interval: interval, Tier: tier, Status: StatusActive, NextDueAt: now, UpdatedAt: now}
	series.Priority = seriesPriority(series, now)
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(series).Error; err != nil {
		return nil, err
	}
	return s.FindSeries(symbol, interval)
}

// ListSeries returns every series.
func (s *Store) ListSeries() ([]*Series, error) {
	var series []*Series
	err := s.db.Order("symbol, interval_name").Find(&series).Error
	return series, err
}

// NextDue returns the most urgent series due at now, or nil.
func (s *Store) NextDue(now int64) (*Series, error) {
	var series Series
	err := s.db.Where("status = ? AND next_due_at <= ?", StatusActive, now).
		Order("priority ASC, next_due_at ASC").First(&series).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &series, nil
}

// EarliestDue returns when the next active series falls due, or 0 if none does.
func (s *Store) EarliestDue() (int64, error) {
	var earliest *int64
	err := s.db.Model(&Series{}).Where("status = ?", StatusActive).Select("MIN(next_due_at)").Scan(&earliest).Error
	if err != nil || earliest == nil {
		return 0, err
	}
	return *earliest, nil
}

func (s *Store) saveSeries(series *Series) error {
	return s.db.Save(series).Error
}

func toRow(seriesID int64, c binance.Candle) Candle {
	return Candle{
		SeriesID: seriesID, OpenTime: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close,
		Volume: c.Volume, QuoteVolume: c.QuoteVolume, Trades: c.Trades, TakerBuyVolume: c.TakerBuyVolume, TakerBuyQuote: c.TakerBuyQuote,
	}
}

// appendCandles writes candles and moves the series forward in one transaction,
// so the water mark never claims data that was not stored.
func (s *Store) appendCandles(series *Series, candles []binance.Candle, gap *Gap) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := insertCandles(tx, series.ID, candles); err != nil {
			return err
		}
		if gap != nil {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(gap).Error; err != nil {
				return err
			}
		}
		return tx.Save(series).Error
	})
}

func insertCandles(tx *gorm.DB, seriesID int64, candles []binance.Candle) error {
	if len(candles) == 0 {
		return nil
	}
	rows := make([]Candle, 0, len(candles))
	for _, candle := range candles {
		rows = append(rows, toRow(seriesID, candle))
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 500).Error
}

// markOpened records that the user asked for a series.
func (s *Store) markOpened(series *Series, now int64) error {
	series.LastOpenedAt = now
	if series.Status != StatusActive {
		series.Status = StatusActive
		series.Tier = TierOnDemand
	}
	if series.NextDueAt > now && closedBehind(series, now) > 0 {
		series.NextDueAt = now
	}
	series.Priority = seriesPriority(series, now)
	return s.saveSeries(series)
}

// WriteCandlesForTest stores candles and a series directly, for tests of the
// packages that read them.
func (s *Store) WriteCandlesForTest(series *Series, candles []binance.Candle) error {
	return s.appendCandles(series, candles, nil)
}

// CandleCount counts a series' stored candles.
func (s *Store) CandleCount(seriesID int64) (int64, error) {
	var count int64
	err := s.db.Model(&Candle{}).Where("series_id = ?", seriesID).Count(&count).Error
	return count, err
}

// Range returns candles with from <= open time <= to, oldest first.
func (s *Store) Range(seriesID, from, to int64) ([]Candle, error) {
	var candles []Candle
	err := s.db.Where("series_id = ? AND open_time >= ? AND open_time <= ?", seriesID, from, to).
		Order("open_time ASC").Find(&candles).Error
	return candles, err
}

// Gaps returns a series' confirmed gaps, oldest first.
func (s *Store) Gaps(seriesID int64) ([]Gap, error) {
	var gaps []Gap
	err := s.db.Where("series_id = ?", seriesID).Order("from_time ASC").Find(&gaps).Error
	return gaps, err
}

// ErrIncomplete says the data before a time cannot be trusted for a calculation.
var ErrIncomplete = errors.New("market data is not continuous")

// ErrNotEnoughHistory says fewer candles exist than a calculation needs.
var ErrNotEnoughHistory = errors.New("not enough history")

// Window returns the n candles ending at openTime, oldest first. It fails rather
// than return data with an unexplained hole; confirmed gaps are allowed.
func (s *Store) Window(series *Series, openTime int64, n int) ([]Candle, error) {
	var candles []Candle
	err := s.db.Where("series_id = ? AND open_time <= ?", series.ID, openTime).
		Order("open_time DESC").Limit(n).Find(&candles).Error
	if err != nil {
		return nil, err
	}
	if len(candles) < n {
		return nil, ErrNotEnoughHistory
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].OpenTime < candles[j].OpenTime })
	gaps, err := s.Gaps(series.ID)
	if err != nil {
		return nil, err
	}
	if err := CheckContinuous(candles, IntervalMillis(series.Interval), gaps); err != nil {
		return nil, err
	}
	return candles, nil
}

// CheckContinuous verifies that candles follow each other, allowing known gaps.
func CheckContinuous(candles []Candle, step int64, gaps []Gap) error {
	for i := 1; i < len(candles); i++ {
		expected := candles[i-1].OpenTime + step
		if candles[i].OpenTime == expected {
			continue
		}
		explained := false
		for _, gap := range gaps {
			if gap.FromTime == expected && gap.ToTime+step == candles[i].OpenTime {
				explained = true
				break
			}
		}
		if !explained {
			return fmt.Errorf("%w: nothing at %s", ErrIncomplete, time.UnixMilli(expected).UTC().Format(time.RFC3339))
		}
	}
	return nil
}

// SaveDerivatives stores readings, ignoring ones already held.
func (s *Store) SaveDerivatives(symbol, kind string, points []binance.Point) error {
	if len(points) == 0 {
		return nil
	}
	rows := make([]Derivative, 0, len(points))
	for _, point := range points {
		rows = append(rows, Derivative{Symbol: symbol, Kind: kind, Time: point.Time, Value: point.Value})
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 500).Error
}

// LastDerivativeTime returns the newest stored reading's time, or 0.
func (s *Store) LastDerivativeTime(symbol, kind string) (int64, error) {
	var last *int64
	err := s.db.Model(&Derivative{}).Where("symbol = ? AND kind = ?", symbol, kind).Select("MAX(time)").Scan(&last).Error
	if err != nil || last == nil {
		return 0, err
	}
	return *last, nil
}

// Derivatives returns readings with from <= time <= to, oldest first.
func (s *Store) Derivatives(symbol, kind string, from, to int64) ([]Derivative, error) {
	var rows []Derivative
	err := s.db.Where("symbol = ? AND kind = ? AND time >= ? AND time <= ?", symbol, kind, from, to).
		Order("time ASC").Find(&rows).Error
	return rows, err
}
