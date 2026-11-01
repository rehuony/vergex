package forecast

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"vergex/internal/store/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Signal states.
const (
	StatusOpen     = "open"     // Waiting for its target candle.
	StatusVerified = "verified" // Judged.
	StatusVoid     = "void"     // The target candle does not exist.
)

// Outcomes of a verified signal.
const (
	OutcomeHit     = "hit"
	OutcomeMiss    = "miss"
	OutcomeNeutral = "neutral" // No direction was called, or price did not move.
)

// What a forecast strategy is in strategy.strategies.
const (
	strategyDomain = "forecast"
	strategyType   = "signal"
)

// Strategy is a saved forecast strategy. It cannot change: another idea is saved as
// a new strategy, so its signals always describe one set of rules.
type Strategy struct {
	ID            string `gorm:"primaryKey" json:"id"`
	Domain        string `json:"-"`
	Type          string `json:"-"`
	Name          string `json:"name"`
	EngineVersion int    `json:"engine_version"`
	ConfigJSON    string `gorm:"column:config" json:"-"`
	Config        Config `gorm:"-" json:"config"`
	IsActive      bool   `json:"-"`
	CreatedAt     int64  `gorm:"serializer:unixms;autoCreateTime:false" json:"created_at"`
}

func (Strategy) TableName() string { return "strategy.strategies" }

// settingsModule is the row in core.settings holding the alert threshold.
const settingsModule = "forecast"

// Settings holds the forecast settings.
type Settings struct {
	// ActiveStrategyID is the strategy marked active, not a stored setting.
	ActiveStrategyID string `json:"-"`
	// AlertThreshold is the score at or beyond which a signal alerts: 70 means 70
	// and above upwards, 30 and below downwards.
	AlertThreshold int `json:"alert_threshold"`
}

// DefaultAlertThreshold is used until the user sets one.
const DefaultAlertThreshold = 70

// Signal is one forecast and, later, how it turned out. Symbol and Interval come from
// its series.
type Signal struct {
	ID         int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID string  `json:"strategy_id"`
	SeriesID   int64   `json:"series_id"`
	OpenTime   int64   `gorm:"column:open_at;serializer:unixms" json:"open_time"`
	Symbol     string  `gorm:"column:symbol;->" json:"symbol"`
	Interval   string  `gorm:"column:timeframe;->" json:"interval"`
	Close      float64 `json:"close"`
	// ProbCandles uses candles only; ProbFull adds derivatives.
	ProbCandles float64 `json:"prob_candles"`
	ProbFull    float64 `json:"prob_full"`
	// DisplayProb is what to show: ProbFull until calibrated, then the calibrated value.
	DisplayProb     float64 `json:"display_prob"`
	Calibrated      bool    `json:"calibrated"`
	Direction       string  `json:"direction"`
	Confidence      int     `json:"confidence"`
	ExpectedMovePct float64 `json:"expected_move_pct"`
	Support         float64 `json:"support"`
	Resistance      float64 `json:"resistance"`
	Regime          string  `json:"regime"`
	Horizon         int     `json:"horizon"`
	// TargetTime is the open time of the candle whose close judges this forecast.
	TargetTime    int64   `gorm:"column:target_at;serializer:unixms" json:"target_time"`
	Status        string  `json:"status"`
	Outcome       string  `gorm:"default:null" json:"outcome,omitempty"`
	TargetClose   float64 `gorm:"default:null" json:"target_close,omitempty"`
	ActualMovePct float64 `gorm:"default:null" json:"actual_move_pct,omitempty"`
	VerifiedAt    int64   `gorm:"serializer:unixms" json:"verified_at,omitempty"`
	// Independent means its window does not overlap an earlier independent signal of
	// the series, and price moved.
	Independent bool `json:"-"`
}

func (Signal) TableName() string { return "forecast.signals" }

// Calibration is a strategy's curve for one interval.
type Calibration struct {
	StrategyID string `gorm:"primaryKey"`
	Interval   string `gorm:"column:timeframe;primaryKey"`
	Samples    int
	Knots      string
	BuiltAt    int64 `gorm:"serializer:unixms"`
}

func (Calibration) TableName() string { return "forecast.calibrations" }

// AlertArm remembers that a series has alerted; it re-arms once the score comes
// back inside the threshold.
type AlertArm struct {
	SeriesID int64 `gorm:"primaryKey;autoIncrement:false"`
	Alerted  bool
}

func (AlertArm) TableName() string { return "forecast.alert_states" }

// Backtest states.
const (
	BacktestRunning  = "running"
	BacktestFinished = "finished"
	BacktestFailed   = "failed"
)

// Backtest is one run.
type Backtest struct {
	ID          int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID  string          `json:"strategy_id"`
	OptionsJSON string          `gorm:"column:options" json:"-"`
	Options     BacktestOptions `gorm:"-" json:"options"`
	Status      string          `json:"status"`
	Done        int             `json:"done"`
	Total       int             `json:"total"`
	ReportJSON  string          `gorm:"column:report;default:null" json:"-"`
	Report      *Report         `gorm:"-" json:"report,omitempty"`
	Error       string          `json:"error,omitempty"`
	CreatedAt   int64           `gorm:"serializer:unixms;autoCreateTime:false" json:"created_at"`
	FinishedAt  int64           `gorm:"serializer:unixms" json:"finished_at,omitempty"`
}

func (Backtest) TableName() string { return "strategy.backtests" }

// Store keeps forecasts beside the candles.
type Store struct {
	db *gorm.DB
}

// NewStore keeps forecasts on a connection.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// at is a time in Unix milliseconds as a query argument.
func at(millis int64) time.Time { return time.UnixMilli(millis).UTC() }

func (v *Strategy) decode() error {
	return json.Unmarshal([]byte(v.ConfigJSON), &v.Config)
}

// CreateStrategy saves a new strategy under a name of its own.
func (s *Store) CreateStrategy(config Config, name string, now time.Time) (*Strategy, error) {
	return s.createStrategy(s.db, config, name, false, now)
}

func (s *Store) createStrategy(db *gorm.DB, config Config, name string, active bool, now time.Time) (*Strategy, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("a strategy needs a name")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	strategy := &Strategy{
		ID: uuid.NewString(), Domain: strategyDomain, Type: strategyType, Name: name, EngineVersion: EngineVersion,
		ConfigJSON: string(body), Config: config, IsActive: active, CreatedAt: now.UnixMilli(),
	}
	if err := db.Create(strategy).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errors.New("a strategy with this name already exists")
		}
		return nil, err
	}
	return strategy, nil
}

// Refusals when deleting a strategy.
var (
	ErrStrategyInUse  = errors.New("the strategy in use cannot be deleted")
	ErrStrategyBusy   = errors.New("a backtest of this strategy is running")
	ErrStrategyAbsent = errors.New("strategy not found")
)

// DeleteStrategy removes a strategy not in use. Its signals, statistics, curves and
// backtests go with it.
func (s *Store) DeleteStrategy(id string, now time.Time) error {
	active, err := s.ActiveStrategy(now)
	if err != nil {
		return err
	}
	if active.ID == id {
		return ErrStrategyInUse
	}
	if uuid.Validate(id) != nil {
		return ErrStrategyAbsent
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var running int64
		if err := tx.Model(&Backtest{}).Where("strategy_id = ? AND status = ?", id, BacktestRunning).Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrStrategyBusy
		}
		result := tx.Where("id = ? AND domain = ?", id, strategyDomain).Delete(&Strategy{})
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrStrategyAbsent
		}
		return result.Error
	})
}

// Strategies lists every strategy, newest first.
func (s *Store) Strategies() ([]*Strategy, error) {
	var strategies []*Strategy
	if err := s.db.Where("domain = ?", strategyDomain).Order("created_at DESC, id").Find(&strategies).Error; err != nil {
		return nil, err
	}
	for _, strategy := range strategies {
		if err := strategy.decode(); err != nil {
			return nil, err
		}
	}
	return strategies, nil
}

// StrategyByID returns one strategy.
func (s *Store) StrategyByID(id string) (*Strategy, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var strategy Strategy
	if err := s.db.Where("id = ? AND domain = ?", id, strategyDomain).First(&strategy).Error; err != nil {
		return nil, err
	}
	return &strategy, strategy.decode()
}

// LoadSettings returns the settings, creating the first strategy when there is none.
func (s *Store) LoadSettings(now time.Time) (Settings, error) {
	active, err := s.ActiveStrategy(now)
	if err != nil {
		return Settings{}, err
	}
	settings := Settings{ActiveStrategyID: active.ID, AlertThreshold: DefaultAlertThreshold}
	found, err := database.LoadSetting(s.db, settingsModule, &settings)
	if err != nil || found {
		return settings, err
	}
	return settings, s.SetAlertThreshold(DefaultAlertThreshold)
}

// ActiveStrategy returns the strategy in use, creating the first one when there is none.
func (s *Store) ActiveStrategy(now time.Time) (*Strategy, error) {
	var strategies []Strategy
	if err := s.db.Where("domain = ? AND is_active", strategyDomain).Limit(1).Find(&strategies).Error; err != nil {
		return nil, err
	}
	if len(strategies) == 1 {
		return &strategies[0], strategies[0].decode()
	}
	return s.createStrategy(s.db, DefaultConfig(), "Default", true, now)
}

// Activate makes a strategy the one in use. Signals stay with their own strategies.
func (s *Store) Activate(id string) error {
	if _, err := s.StrategyByID(id); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Strategy{}).Where("domain = ? AND is_active", strategyDomain).Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Model(&Strategy{}).Where("id = ?", id).Update("is_active", true).Error
	})
}

// SetAlertThreshold stores the alert threshold.
func (s *Store) SetAlertThreshold(threshold int) error {
	return database.PatchSetting(s.db, settingsModule, map[string]any{"alert_threshold": threshold})
}

// signalQuery selects signals with their symbol and interval.
func (s *Store) signalQuery() *gorm.DB {
	return s.db.Table("forecast.signals AS r").Select("r.*, i.symbol, m.timeframe").
		Joins("JOIN market.series AS m ON m.id = r.series_id").
		Joins("JOIN market.instruments AS i ON i.id = m.instrument_id")
}

// SaveSignal stores a forecast, doing nothing if that candle already has one.
// It reports whether the signal is new.
func (s *Store) SaveSignal(signal *Signal) (bool, error) {
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(signal)
	return result.RowsAffected > 0, result.Error
}

// LatestOpenTimes returns the newest recorded candle of each series for a strategy.
func (s *Store) LatestOpenTimes(strategyID string) (map[int64]int64, error) {
	var rows []struct {
		SeriesID int64
		Latest   time.Time
	}
	err := s.db.Model(&Signal{}).Select("series_id, MAX(open_at) AS latest").
		Where("strategy_id = ?", strategyID).Group("series_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	latest := make(map[int64]int64, len(rows))
	for _, row := range rows {
		latest[row.SeriesID] = row.Latest.UnixMilli()
	}
	return latest, nil
}

// LatestSignals returns each series' newest signal for a strategy and interval.
func (s *Store) LatestSignals(strategyID, interval string) ([]Signal, error) {
	var signals []Signal
	latest := s.db.Table("forecast.signals AS l").Select("l.series_id, MAX(l.open_at) AS latest").
		Joins("JOIN market.series AS lm ON lm.id = l.series_id").
		Where("l.strategy_id = ? AND lm.timeframe = ?", strategyID, interval).Group("l.series_id")
	err := s.signalQuery().
		Joins("JOIN (?) AS newest ON r.series_id = newest.series_id AND r.open_at = newest.latest", latest).
		Where("r.strategy_id = ?", strategyID).Find(&signals).Error
	return signals, err
}

// CoinCount returns how many coins a strategy has forecast.
func (s *Store) CoinCount(strategyID string) (int64, error) {
	var coins int64
	err := s.db.Table("forecast.signals AS r").
		Joins("JOIN market.series AS m ON m.id = r.series_id").
		Where("r.strategy_id = ?", strategyID).Distinct("m.instrument_id").Count(&coins).Error
	return coins, err
}

// SignalsAfter returns the signals past an ID whose candle opened at or after since,
// oldest first, across all strategies.
func (s *Store) SignalsAfter(id, since int64, limit int) ([]Signal, error) {
	var signals []Signal
	err := s.signalQuery().Where("r.id > ? AND r.open_at >= ?", id, at(since)).Order("r.id ASC").Limit(limit).Find(&signals).Error
	return signals, err
}

// RecentSignals returns a series' newest signals for a strategy.
func (s *Store) RecentSignals(strategyID string, seriesID int64, limit int) ([]Signal, error) {
	var signals []Signal
	err := s.signalQuery().Where("r.strategy_id = ? AND r.series_id = ?", strategyID, seriesID).
		Order("r.open_at DESC").Limit(limit).Find(&signals).Error
	return signals, err
}

// DueSignals returns open signals whose target candle should have closed by now.
func (s *Store) DueSignals(before int64, limit int) ([]Signal, error) {
	var signals []Signal
	err := s.signalQuery().Where("r.status = ? AND r.target_at <= ?", StatusOpen, at(before)).
		Order("r.target_at ASC").Limit(limit).Find(&signals).Error
	return signals, err
}

// saveVerification stores how a signal turned out and adds it to the statistics in
// one transaction, so the two always agree. step is the signal's candle length.
func (s *Store) saveVerification(signal *Signal, step int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": signal.Status, "verified_at": database.FromMillis(signal.VerifiedAt)}
		if signal.Status == StatusVerified {
			independent, err := isIndependent(tx, signal, step)
			if err != nil {
				return err
			}
			signal.Independent = independent
			updates["outcome"] = signal.Outcome
			updates["target_close"] = signal.TargetClose
			updates["actual_move_pct"] = signal.ActualMovePct
			updates["independent"] = independent
		}
		result := tx.Model(&Signal{}).Where("id = ? AND status = ?", signal.ID, StatusOpen).Updates(updates)
		if result.Error != nil || result.RowsAffected == 0 || signal.Status != StatusVerified {
			return result.Error
		}
		return addToStats(tx, signal)
	})
}

// isIndependent reports whether a signal's window starts after the last independent
// signal's ended, and price moved. Signals are judged in target time order, so this
// picks the same ones as walking a series from its start.
func isIndependent(tx *gorm.DB, signal *Signal, step int64) (bool, error) {
	if signal.TargetClose == signal.Close {
		return false, nil
	}
	var last sql.NullTime
	err := tx.Model(&Signal{}).Where("strategy_id = ? AND series_id = ? AND independent", signal.StrategyID, signal.SeriesID).
		Select("MAX(open_at)").Scan(&last).Error
	if err != nil {
		return false, err
	}
	return !last.Valid || signal.OpenTime >= last.Time.UnixMilli()+int64(signal.Horizon)*step, nil
}

// confidenceBand groups confidence in steps of 20.
func confidenceBand(confidence int) int { return min(confidence/20, 4) }

func addToStats(tx *gorm.DB, signal *Signal) error {
	count := func(yes bool) int {
		if yes {
			return 1
		}
		return 0
	}
	moved, rose := signal.TargetClose != signal.Close, signal.TargetClose > signal.Close
	hit := signal.Outcome == OutcomeHit
	opened := at(signal.OpenTime)
	return tx.Exec(`
		INSERT INTO forecast.outcome_stats (
			strategy_id, series_id, month, direction, regime, confidence_band,
			judged, hits, misses, moved, rose, move_sum,
			independent_moved, independent_hits, independent_rose
		) VALUES (?, ?, ?::date, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (strategy_id, series_id, month, direction, regime, confidence_band) DO UPDATE SET
			judged = forecast.outcome_stats.judged + 1,
			hits = forecast.outcome_stats.hits + EXCLUDED.hits,
			misses = forecast.outcome_stats.misses + EXCLUDED.misses,
			moved = forecast.outcome_stats.moved + EXCLUDED.moved,
			rose = forecast.outcome_stats.rose + EXCLUDED.rose,
			move_sum = forecast.outcome_stats.move_sum + EXCLUDED.move_sum,
			independent_moved = forecast.outcome_stats.independent_moved + EXCLUDED.independent_moved,
			independent_hits = forecast.outcome_stats.independent_hits + EXCLUDED.independent_hits,
			independent_rose = forecast.outcome_stats.independent_rose + EXCLUDED.independent_rose`,
		signal.StrategyID, signal.SeriesID, fmt.Sprintf("%04d-%02d-01", opened.Year(), opened.Month()),
		signal.Direction, signal.Regime, confidenceBand(signal.Confidence),
		count(hit), count(signal.Outcome == OutcomeMiss), count(moved), count(rose), signal.ActualMovePct,
		count(signal.Independent), count(signal.Independent && hit), count(signal.Independent && rose),
	).Error
}

// IndependentPoints returns a strategy's independent signals for an interval, as
// points to calibrate on.
func (s *Store) IndependentPoints(strategyID, interval string) ([]CalibrationPoint, error) {
	var rows []struct {
		ProbFull float64
		Up       bool
	}
	err := s.db.Table("forecast.signals AS r").Select("r.prob_full, r.target_close > r.close AS up").
		Joins("JOIN market.series AS m ON m.id = r.series_id").
		Where("r.strategy_id = ? AND m.timeframe = ? AND r.independent", strategyID, interval).
		Order("r.series_id, r.open_at").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	points := make([]CalibrationPoint, len(rows))
	for i, row := range rows {
		points[i] = CalibrationPoint{Prob: row.ProbFull, Up: row.Up}
	}
	return points, nil
}

// Calibrations returns a strategy's curves by interval.
func (s *Store) Calibrations(strategyID string) (map[string][]Knot, error) {
	var rows []Calibration
	if err := s.db.Where("strategy_id = ?", strategyID).Find(&rows).Error; err != nil {
		return nil, err
	}
	curves := make(map[string][]Knot, len(rows))
	for _, row := range rows {
		curves[row.Interval] = decodeKnots(row.Knots)
	}
	return curves, nil
}

func (s *Store) saveCalibration(calibration Calibration) error {
	return s.db.Save(&calibration).Error
}

func (s *Store) alertArm(seriesID int64) bool {
	var arm AlertArm
	if err := s.db.First(&arm, "series_id = ?", seriesID).Error; err != nil {
		return false
	}
	return arm.Alerted
}

func (s *Store) setAlertArm(seriesID int64, alerted bool) error {
	return s.db.Save(&AlertArm{SeriesID: seriesID, Alerted: alerted}).Error
}

// CreateBacktest records a run about to start.
func (s *Store) CreateBacktest(strategyID string, options BacktestOptions, now time.Time) (*Backtest, error) {
	body, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	run := &Backtest{StrategyID: strategyID, OptionsJSON: string(body), Options: options, Status: BacktestRunning, Total: len(options.Symbols) * len(options.Intervals), CreatedAt: now.UnixMilli()}
	return run, s.db.Create(run).Error
}

func (s *Store) updateBacktestProgress(id int64, done int) error {
	return s.db.Model(&Backtest{}).Where("id = ?", id).Update("done", done).Error
}

func (s *Store) finishBacktest(id int64, report *Report, failure string, now time.Time) error {
	updates := map[string]any{"finished_at": now.UTC(), "status": BacktestFinished}
	if failure != "" {
		updates["status"] = BacktestFailed
		updates["error"] = failure
	}
	if report != nil {
		body, err := json.Marshal(report)
		if err != nil {
			return err
		}
		updates["report"] = string(body)
	}
	return s.db.Model(&Backtest{}).Where("id = ?", id).Updates(updates).Error
}

// failInterruptedBacktests marks runs that were going when the process stopped.
func (s *Store) failInterruptedBacktests(now time.Time) error {
	return s.db.Model(&Backtest{}).Where("status = ?", BacktestRunning).Updates(map[string]any{
		"status": BacktestFailed, "error": "interrupted by a restart", "finished_at": now.UTC(),
	}).Error
}

func (b *Backtest) decode() error {
	if err := json.Unmarshal([]byte(b.OptionsJSON), &b.Options); err != nil {
		return err
	}
	if b.ReportJSON != "" {
		b.Report = &Report{}
		return json.Unmarshal([]byte(b.ReportJSON), b.Report)
	}
	return nil
}

// Backtests lists runs newest first. Reports are included, for comparing runs.
func (s *Store) Backtests(limit int) ([]*Backtest, error) {
	var runs []*Backtest
	if err := s.db.Order("id DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, err
	}
	for _, run := range runs {
		if err := run.decode(); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

// BacktestByID returns one run.
func (s *Store) BacktestByID(id int64) (*Backtest, error) {
	var run Backtest
	if err := s.db.First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, run.decode()
}

// SignalStats is how a strategy's calls on a series have turned out.
type SignalStats struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
	// Moved counts every judged signal where price moved, called or not, and Rose how
	// many of them rose. A hit rate has to beat this share to mean anything.
	Moved int64 `json:"moved"`
	Rose  int64 `json:"rose"`
}

// SignalStats adds up a series' judged forecasts for a strategy.
func (s *Store) SignalStats(strategyID string, seriesID int64) (SignalStats, error) {
	var stats SignalStats
	err := s.db.Table("forecast.outcome_stats").Select(`
		COALESCE(SUM(hits), 0) AS hits, COALESCE(SUM(misses), 0) AS misses,
		COALESCE(SUM(moved), 0) AS moved, COALESCE(SUM(rose), 0) AS rose`).
		Where("strategy_id = ? AND series_id = ?", strategyID, seriesID).Scan(&stats).Error
	return stats, err
}
