// Package feed keeps candles and derivatives data in the market schema and
// fills them within Binance's rate limits. Calculations read only this local
// store; only the hydrator talks to the exchange.
package feed

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"vergex/internal/database"
	"vergex/internal/market/instrument"
	"vergex/internal/venue/binance"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const VenueBinance = "binance"

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

// Series is one instrument at one interval, and how far it has been filled. Exchange
// and Symbol come from the instrument.
type Series struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	InstrumentID int64  `json:"-"`
	Exchange     string `gorm:"column:venue;->" json:"exchange"`
	Symbol       string `gorm:"column:symbol;->" json:"symbol"`
	Interval     string `gorm:"column:timeframe" json:"interval"`
	Tier         int    `json:"tier"`
	Status       string `json:"status"`
	Priority     int    `json:"priority"`
	// NextDueAt is when this series should next be synced.
	NextDueAt int64 `gorm:"serializer:unixms" json:"next_due_at"`
	// FirstOpenTime is the oldest stored candle's open time.
	FirstOpenTime int64 `gorm:"column:first_open_at;serializer:unixms" json:"first_open_time"`
	// LastClosedTime is the water mark: the open time of the newest candle that
	// has closed and joins the data before it without an unexplained hole.
	LastClosedTime int64  `gorm:"column:last_closed_at;serializer:unixms" json:"last_closed_time"`
	FailCount      int    `json:"fail_count"`
	LastError      string `json:"last_error,omitempty"`
	// LastOpenedAt is when the user last asked for this series.
	LastOpenedAt int64 `gorm:"serializer:unixms" json:"last_opened_at"`
	UpdatedAt    int64 `gorm:"serializer:unixms;autoUpdateTime:false" json:"updated_at"`
}

func (Series) TableName() string { return "market.series" }

// Candle is a closed candle.
type Candle struct {
	SeriesID       int64 `gorm:"primaryKey;autoIncrement:false"`
	OpenTime       int64 `gorm:"column:open_at;primaryKey;autoIncrement:false;serializer:unixms"`
	Open           float64
	High           float64
	Low            float64
	Close          float64
	Volume         float64
	QuoteVolume    float64
	Trades         int64
	TakerBuyVolume float64
	TakerBuyQuote  float64
}

func (Candle) TableName() string { return "market.candles" }

// Gap is a range the exchange confirmed it has no candles for, so it is not
// requested again and does not count as missing data.
type Gap struct {
	SeriesID int64 `gorm:"primaryKey;autoIncrement:false"`
	FromTime int64 `gorm:"column:from_at;primaryKey;autoIncrement:false;serializer:unixms"`
	ToTime   int64 `gorm:"column:to_at;serializer:unixms"`
}

func (Gap) TableName() string { return "market.candle_gaps" }

// Derivative kinds.
const (
	KindFunding   = "funding"
	KindOpenValue = "open_interest"
	KindLongShort = "long_short"
)

// Derivative is one timed reading of a derivatives series.
type Derivative struct {
	InstrumentID int64  `gorm:"primaryKey;autoIncrement:false"`
	Kind         string `gorm:"primaryKey;autoIncrement:false"`
	Time         int64  `gorm:"column:observed_at;primaryKey;autoIncrement:false;serializer:unixms"`
	Value        float64
}

func (Derivative) TableName() string { return "market.derivative_metrics" }

// settingsModule is the row in core.settings holding Settings.
const settingsModule = "market"

// Settings is what the user chose to collect.
type Settings struct {
	Enabled   bool     `json:"enabled"`
	Watchlist []string `json:"watchlist"`
	TopN      int      `json:"top_n"`
	// Ranked holds the current high-volume coins and RankedAt when they were last
	// worked out; they change slowly so the sample stays steady.
	Ranked   []string `json:"ranked"`
	RankedAt int64    `json:"ranked_at"`
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

// intervalMillis holds what can be kept locally. Trading and charts can ask for any
// of these; the lists above are what collection keeps on its own.
var intervalMillis = map[string]int64{
	"1m":  60 * 1000,
	"3m":  3 * 60 * 1000,
	"5m":  5 * 60 * 1000,
	"15m": 15 * 60 * 1000,
	"30m": 30 * 60 * 1000,
	"1h":  60 * 60 * 1000,
	"2h":  2 * 60 * 60 * 1000,
	"4h":  4 * 60 * 60 * 1000,
	"6h":  6 * 60 * 60 * 1000,
	"8h":  8 * 60 * 60 * 1000,
	"12h": 12 * 60 * 60 * 1000,
	"1d":  24 * 60 * 60 * 1000,
}

// IntervalMillis returns an interval's length, or 0 if it is not supported.
func IntervalMillis(interval string) int64 { return intervalMillis[interval] }

// IsHistoryInterval reports an interval kept for the long term, which backtests
// use and deep history can be loaded for.
func IsHistoryInterval(interval string) bool {
	return slices.Contains(WatchIntervals, interval)
}

// shortCandleKeep is how long candles under 15 minutes are kept. Trading only reads
// the recent ones, and over a year they would dwarf everything else.
const shortCandleKeep = 90 * 24 * time.Hour

var shortIntervals = []string{"1m", "3m", "5m"}

// Store is the market database.
type Store struct {
	db *gorm.DB
	// instruments remembers IDs by symbol; instruments are never deleted.
	instruments sync.Map
}

// NewStore keeps market data on a connection, normally a pool kept apart from the
// main one so bulk candle writes never queue up orders and positions.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// instrumentID returns a Binance perpetual's instrument, creating it if needed.
func (s *Store) instrumentID(symbol string) (int64, error) {
	if id, ok := s.instruments.Load(symbol); ok {
		return id.(int64), nil
	}
	id, err := instrument.Ensure(s.db, VenueBinance, instrument.KindPerpetual, symbol, symbol)
	if err != nil {
		return 0, err
	}
	s.instruments.Store(symbol, id)
	return id, nil
}

// at is a time in Unix milliseconds as a query argument.
func at(millis int64) time.Time { return time.UnixMilli(millis).UTC() }

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
	found, err := database.LoadSetting(s.db, settingsModule, &settings)
	if err != nil || found {
		return settings, err
	}
	settings = Settings{Enabled: true, Watchlist: DefaultWatchlist, TopN: DefaultTopN}
	return settings, database.PatchSetting(s.db, settingsModule, map[string]any{
		"enabled": settings.Enabled, "watchlist": settings.Watchlist, "top_n": settings.TopN,
	})
}

// SaveSettings stores what the user chose. Changing the high-volume count makes
// the ranking run again on the next plan.
func (s *Store) SaveSettings(enabled bool, watchlist []string, topN int) error {
	current, err := s.LoadSettings()
	if err != nil {
		return err
	}
	fields := map[string]any{"enabled": enabled, "watchlist": watchlist, "top_n": topN}
	if topN != current.TopN {
		fields["ranked_at"] = 0
	}
	return database.PatchSetting(s.db, settingsModule, fields)
}

func (s *Store) saveRanking(symbols []string, at int64) error {
	return database.PatchSetting(s.db, settingsModule, map[string]any{"ranked": symbols, "ranked_at": at})
}

// seriesQuery selects series with their exchange and symbol.
func (s *Store) seriesQuery() *gorm.DB {
	return s.db.Table("market.series AS s").Select("s.*, i.venue, i.symbol").
		Joins("JOIN market.instruments AS i ON i.id = s.instrument_id")
}

// FindSeries returns one series, or nil if it does not exist yet.
func (s *Store) FindSeries(symbol, interval string) (*Series, error) {
	var series []*Series
	err := s.seriesQuery().Where("i.venue = ? AND i.kind = ? AND i.symbol = ? AND s.timeframe = ?", VenueBinance, instrument.KindPerpetual, symbol, interval).
		Limit(1).Find(&series).Error
	if err != nil || len(series) == 0 {
		return nil, err
	}
	return series[0], nil
}

// EnsureSeries returns the series, creating it due now if it is new.
func (s *Store) EnsureSeries(symbol, interval string, tier int, now int64) (*Series, error) {
	series, err := s.FindSeries(symbol, interval)
	if err != nil || series != nil {
		return series, err
	}
	instrumentID, err := s.instrumentID(symbol)
	if err != nil {
		return nil, err
	}
	series = &Series{InstrumentID: instrumentID, Interval: interval, Tier: tier, Status: StatusActive, NextDueAt: now, UpdatedAt: now}
	series.Priority = seriesPriority(series, now)
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(series).Error; err != nil {
		return nil, err
	}
	return s.FindSeries(symbol, interval)
}

// ListSeries returns every series.
func (s *Store) ListSeries() ([]*Series, error) {
	var series []*Series
	err := s.seriesQuery().Order("i.symbol, s.timeframe").Find(&series).Error
	return series, err
}

// NextDue returns the most urgent series due at now, or nil.
func (s *Store) NextDue(now int64) (*Series, error) {
	var series []*Series
	err := s.seriesQuery().Where("s.status = ? AND s.next_due_at <= ?", StatusActive, at(now)).
		Order("s.priority ASC, s.next_due_at ASC").Limit(1).Find(&series).Error
	if err != nil || len(series) == 0 {
		return nil, err
	}
	return series[0], nil
}

// EarliestDue returns when the next active series falls due, or 0 if none does.
func (s *Store) EarliestDue() (int64, error) {
	var earliest sql.NullTime
	err := s.db.Model(&Series{}).Where("status = ?", StatusActive).Select("MIN(next_due_at)").Scan(&earliest).Error
	if err != nil || !earliest.Valid {
		return 0, err
	}
	return earliest.Time.UnixMilli(), nil
}

// saveSeries stores a series. The water mark only moves forward: the hydrator and
// the stream writer work side by side, and whichever holds the older copy must not
// undo the other.
func (s *Store) saveSeries(series *Series) error { return saveSeriesOn(s.db, series) }

func saveSeriesOn(db *gorm.DB, series *Series) error {
	return db.Model(&Series{}).Where("id = ?", series.ID).Updates(map[string]any{
		"tier":           series.Tier,
		"status":         series.Status,
		"priority":       series.Priority,
		"next_due_at":    database.FromMillis(series.NextDueAt),
		"first_open_at":  database.FromMillis(series.FirstOpenTime),
		"last_closed_at": gorm.Expr("GREATEST(last_closed_at, ?::timestamptz)", database.FromMillis(series.LastClosedTime)),
		"fail_count":     series.FailCount,
		"last_error":     series.LastError,
		"last_opened_at": database.FromMillis(series.LastOpenedAt),
		"updated_at":     database.FromMillis(series.UpdatedAt),
	}).Error
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
		return saveSeriesOn(tx, series)
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

// purgeShortCandles removes candles under 15 minutes from before the cutoff.
func (s *Store) purgeShortCandles(before int64) error {
	return s.db.Where("open_at < ? AND series_id IN (?)", at(before),
		s.db.Model(&Series{}).Select("id").Where("timeframe IN ?", shortIntervals)).Delete(&Candle{}).Error
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
	err := s.db.Where("series_id = ? AND open_at >= ? AND open_at <= ?", seriesID, at(from), at(to)).
		Order("open_at ASC").Find(&candles).Error
	return candles, err
}

// Gaps returns a series' confirmed gaps, oldest first.
func (s *Store) Gaps(seriesID int64) ([]Gap, error) {
	var gaps []Gap
	err := s.db.Where("series_id = ?", seriesID).Order("from_at ASC").Find(&gaps).Error
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
	err := s.db.Where("series_id = ? AND open_at <= ?", series.ID, at(openTime)).
		Order("open_at DESC").Limit(n).Find(&candles).Error
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
	instrumentID, err := s.instrumentID(symbol)
	if err != nil {
		return err
	}
	rows := make([]Derivative, 0, len(points))
	for _, point := range points {
		rows = append(rows, Derivative{InstrumentID: instrumentID, Kind: kind, Time: point.Time, Value: point.Value})
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 500).Error
}

// LastDerivativeTime returns the newest stored reading's time, or 0.
func (s *Store) LastDerivativeTime(symbol, kind string) (int64, error) {
	instrumentID, err := s.instrumentID(symbol)
	if err != nil {
		return 0, err
	}
	var last sql.NullTime
	err = s.db.Model(&Derivative{}).Where("instrument_id = ? AND kind = ?", instrumentID, kind).Select("MAX(observed_at)").Scan(&last).Error
	if err != nil || !last.Valid {
		return 0, err
	}
	return last.Time.UnixMilli(), nil
}

// Derivatives returns readings with from <= time <= to, oldest first.
func (s *Store) Derivatives(symbol, kind string, from, to int64) ([]Derivative, error) {
	instrumentID, err := s.instrumentID(symbol)
	if err != nil {
		return nil, err
	}
	var rows []Derivative
	err = s.db.Where("instrument_id = ? AND kind = ? AND observed_at >= ? AND observed_at <= ?", instrumentID, kind, at(from), at(to)).
		Order("observed_at ASC").Find(&rows).Error
	return rows, err
}
