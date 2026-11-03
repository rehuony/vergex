package forecast

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"vergex/internal/database"

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

// Strategy is a saved forecast strategy. Its rules cannot change: another idea is saved
// as a new strategy, so its signals always describe one set of rules. Its name can.
type Strategy struct {
	ID            string `gorm:"primaryKey" json:"id"`
	Name          string `json:"name"`
	EngineVersion int    `json:"-"`
	ConfigJSON    string `gorm:"column:config" json:"-"`
	Config        Config `gorm:"-" json:"config"`
	Active        bool   `json:"-"`
	CreatedAt     int64  `gorm:"serializer:unixms;autoCreateTime:false" json:"created_at"`
	// Retired means the forecast code changed after this was saved. It keeps its
	// records, but its rules would no longer give the same forecasts, so it cannot be
	// backtested, and it can only go back to use by giving those records up: see Revive.
	Retired bool `gorm:"-" json:"retired"`
}

func (Strategy) TableName() string { return "forecast.strategies" }

// settingsModule is the row in core.settings holding the alert threshold.
const settingsModule = "forecast"

// Settings holds the forecast settings.
type Settings struct {
	// ActiveStrategyID is the strategy marked active, not a stored setting.
	ActiveStrategyID string `json:"-"`
	// AlertThreshold is the score at or beyond which a signal alerts: 70 means 70
	// and above upwards, 30 and below downwards.
	AlertThreshold int `json:"alert_threshold"`
	// AlertIntervals are the intervals whose calls can alert.
	AlertIntervals []string `json:"alert_intervals"`
}

// DefaultAlertThreshold is used until the user sets one. Under the default rules it
// picks about the strongest fifth of calls, which hit five to six points over chance out
// of sample.
const DefaultAlertThreshold = 56

// DefaultAlertIntervals leaves out 15m, whose calls would fill the chat, and 1d, whose
// calls have too short a record to stand on.
var DefaultAlertIntervals = []string{"1h", "4h"}

// Signal is one forecast and, later, how it turned out. Symbol and Interval come from
// its series.
type Signal struct {
	ID         int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID string  `json:"strategy_id"`
	SeriesID   int64   `json:"series_id"`
	OpenTime   int64   `gorm:"column:open_at;serializer:unixms" json:"open_time"`
	Symbol     string  `gorm:"column:symbol;->" json:"symbol"`
	Interval   string  `gorm:"column:interval;->" json:"interval"`
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
	// the series, and price moved. The statistics count only these, and so does the
	// chart's record of past calls.
	Independent bool `json:"independent"`
}

func (Signal) TableName() string { return "forecast.signals" }

// Calibration is a strategy's curve for one interval.
type Calibration struct {
	StrategyID string `gorm:"primaryKey"`
	Interval   string `gorm:"column:interval;primaryKey"`
	Knots      string
	BuiltAt    int64 `gorm:"serializer:unixms"`
}

func (Calibration) TableName() string { return "forecast.calibrations" }

// AlertState remembers that a series has alerted; it re-arms once the score comes back
// inside the threshold.
type AlertState struct {
	SeriesID int64 `gorm:"primaryKey;autoIncrement:false"`
	Alerted  bool
}

func (AlertState) TableName() string { return "forecast.alert_states" }

// Backtest states.
const (
	BacktestRunning  = "running"
	BacktestFinished = "finished"
	BacktestFailed   = "failed"
)

// BacktestCanceled is the error of a failed run that was stopped on request.
const BacktestCanceled = "canceled"

// BacktestInterrupted is the error of a run that was going when the process stopped.
const BacktestInterrupted = "interrupted by a restart"

// Refusals when naming or deleting a backtest.
var (
	ErrBacktestName   = errors.New("a backtest's name can be at most 80 characters")
	ErrBacktestAbsent = errors.New("backtest not found")
)

// Backtest is one run.
type Backtest struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	StrategyID string `json:"strategy_id"`
	// Name is what the user calls the run. Empty shows its strategy's name.
	Name        string          `json:"name"`
	OptionsJSON string          `gorm:"column:options" json:"-"`
	Options     BacktestOptions `gorm:"-" json:"options"`
	Status      string          `json:"status"`
	Done        int             `json:"done"`
	Total       int             `json:"total"`
	ReportJSON  string          `gorm:"column:report;default:null" json:"-"`
	Report      *Report         `gorm:"-" json:"report,omitempty"`
	// Summary is the report's overall line, for a list of runs.
	SummaryJSON string   `gorm:"column:summary;->" json:"-"`
	Summary     *Metrics `gorm:"-" json:"summary,omitempty"`
	// TestedFrom and TestedTo are the candles it actually tested, which local history can
	// make much less than what was asked for.
	TestedFrom int64  `gorm:"column:tested_from;->" json:"tested_from,omitempty"`
	TestedTo   int64  `gorm:"column:tested_to;->" json:"tested_to,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedAt  int64  `gorm:"serializer:unixms;autoCreateTime:false" json:"created_at"`
	FinishedAt int64  `gorm:"serializer:unixms" json:"finished_at,omitempty"`
}

func (Backtest) TableName() string { return "forecast.backtests" }

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

func (strategy *Strategy) decode() error {
	strategy.Retired = strategy.EngineVersion != EngineVersion
	return json.Unmarshal([]byte(strategy.ConfigJSON), &strategy.Config)
}

// CreateStrategy saves a new strategy under a name of its own.
func (s *Store) CreateStrategy(config Config, name string, now time.Time) (*Strategy, error) {
	if err := config.validateNew(); err != nil {
		return nil, err
	}
	return s.createStrategy(s.db, config, name, false, now)
}

// maxNameLength is the longest name a strategy or backtest can have.
const maxNameLength = 80

// strategyName tidies a name a strategy is to have, or refuses it.
func strategyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", ErrStrategyName
	}
	return name, nil
}

func (s *Store) createStrategy(db *gorm.DB, config Config, name string, active bool, now time.Time) (*Strategy, error) {
	name, err := strategyName(name)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	strategy := &Strategy{
		ID: uuid.NewString(), Name: name, EngineVersion: EngineVersion,
		ConfigJSON: string(body), Config: config, Active: active, CreatedAt: now.UnixMilli(),
	}
	if err := db.Create(strategy).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrStrategyNameTaken
		}
		return nil, err
	}
	return strategy, nil
}

// RenameStrategy gives a strategy another name. What its signals describe is its rules,
// so a name can change when they cannot.
func (s *Store) RenameStrategy(id, name string) error {
	name, err := strategyName(name)
	if err != nil {
		return err
	}
	if uuid.Validate(id) != nil {
		return ErrStrategyAbsent
	}
	result := s.db.Model(&Strategy{}).Where("id = ?", id).Update("name", name)
	switch {
	case errors.Is(result.Error, gorm.ErrDuplicatedKey):
		return ErrStrategyNameTaken
	case result.Error != nil:
		return result.Error
	case result.RowsAffected == 0:
		return ErrStrategyAbsent
	}
	return nil
}

// Refusals when using or deleting a strategy.
var (
	ErrStrategyInUse     = errors.New("the strategy in use cannot be deleted")
	ErrLastStrategy      = errors.New("the last strategy cannot be deleted")
	ErrStrategyBusy      = errors.New("a backtest of this strategy is running")
	ErrStrategyAbsent    = errors.New("strategy not found")
	ErrStrategyRetired   = errors.New("this strategy was saved under older forecast code: start a new one from it")
	ErrNoStrategyInUse   = errors.New("no forecast strategy is in use")
	ErrStrategyName      = errors.New("a strategy needs a name of at most 80 characters")
	ErrStrategyNameTaken = errors.New("a strategy with this name already exists")
)

// DeleteStrategy removes a strategy not in use. Its signals, statistics, curves and
// backtests go with it.
func (s *Store) DeleteStrategy(id string, now time.Time) error {
	active, err := s.ActiveStrategy(now)
	if err != nil && !errors.Is(err, ErrNoStrategyInUse) {
		return err
	}
	if active != nil && active.ID == id {
		return ErrStrategyInUse
	}
	if uuid.Validate(id) != nil {
		return ErrStrategyAbsent
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// With none left, the next read would make a new Default and put it to use, which
		// is not what deleting a stopped strategy asks for.
		var others int64
		if err := tx.Model(&Strategy{}).Where("id <> ?", id).Count(&others).Error; err != nil {
			return err
		}
		if others == 0 {
			return ErrLastStrategy
		}
		var running int64
		if err := tx.Model(&Backtest{}).Where("strategy_id = ? AND status = ?", id, BacktestRunning).Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrStrategyBusy
		}
		result := tx.Where("id = ?", id).Delete(&Strategy{})
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrStrategyAbsent
		}
		return result.Error
	})
}

// Strategies lists every strategy, newest first.
func (s *Store) Strategies() ([]*Strategy, error) {
	var strategies []*Strategy
	if err := s.db.Order("created_at DESC, id").Find(&strategies).Error; err != nil {
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
	if err := s.db.Where("id = ?", id).First(&strategy).Error; err != nil {
		return nil, err
	}
	return &strategy, strategy.decode()
}

// LoadSettings returns the settings, creating the first strategy when there is none.
func (s *Store) LoadSettings(now time.Time) (Settings, error) {
	active, err := s.ActiveStrategy(now)
	if err != nil && !errors.Is(err, ErrNoStrategyInUse) {
		return Settings{}, err
	}
	settings := Settings{AlertThreshold: DefaultAlertThreshold, AlertIntervals: DefaultAlertIntervals}
	if active != nil {
		settings.ActiveStrategyID = active.ID
	}
	found, err := database.LoadSetting(s.db, settingsModule, &settings)
	if err != nil || found {
		return settings, err
	}
	return settings, s.SetAlerts(DefaultAlertThreshold, DefaultAlertIntervals)
}

// ActiveStrategy returns the strategy in use, creating the first one when there are none
// at all. With strategies saved and every one stopped, it returns ErrNoStrategyInUse.
func (s *Store) ActiveStrategy(now time.Time) (*Strategy, error) {
	var strategies []Strategy
	if err := s.db.Where("active").Limit(1).Find(&strategies).Error; err != nil {
		return nil, err
	}
	if len(strategies) == 1 {
		return &strategies[0], strategies[0].decode()
	}
	var saved int64
	if err := s.db.Model(&Strategy{}).Count(&saved).Error; err != nil {
		return nil, err
	}
	if saved > 0 {
		return nil, ErrNoStrategyInUse
	}
	return s.createStrategy(s.db, DefaultConfig(), "Default", true, now)
}

// Activate makes a strategy the one in use. Signals stay with their own strategies.
func (s *Store) Activate(id string) error {
	strategy, err := s.usableStrategy(id)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Strategy{}).Where("active").Update("active", false).Error; err != nil {
			return err
		}
		return tx.Model(&Strategy{}).Where("id = ?", strategy.ID).Update("active", true).Error
	})
}

// Deactivate stops a strategy being used. Nothing forecasts until one is put to use again.
// Its signals and statistics stay.
func (s *Store) Deactivate(id string) error {
	if uuid.Validate(id) != nil {
		return ErrStrategyAbsent
	}
	result := s.db.Model(&Strategy{}).Where("id = ?", id).Update("active", false)
	if result.Error == nil && result.RowsAffected == 0 {
		return ErrStrategyAbsent
	}
	return result.Error
}

// Revive puts a retired strategy back to use under this version of the code. What it
// recorded came from the old code and would be added up as if it were the same, so its
// signals, statistics, curves and backtests go, and its record starts again.
func (s *Store) Revive(id string) error {
	strategy, err := s.StrategyByID(id)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrStrategyAbsent
	case err != nil:
		return err
	case !strategy.Retired:
		return s.Activate(id)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// With none left, the next read would make a new Default and put it to use, which
		// is not what deleting a stopped strategy asks for.
		var others int64
		if err := tx.Model(&Strategy{}).Where("id <> ?", id).Count(&others).Error; err != nil {
			return err
		}
		if others == 0 {
			return ErrLastStrategy
		}
		var running int64
		if err := tx.Model(&Backtest{}).Where("strategy_id = ? AND status = ?", id, BacktestRunning).Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrStrategyBusy
		}
		for _, table := range []string{"forecast.signals", "forecast.outcome_stats", "forecast.calibrations", "forecast.backtests"} {
			if err := tx.Exec("DELETE FROM "+table+" WHERE strategy_id = ?", id).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Strategy{}).Where("active").Update("active", false).Error; err != nil {
			return err
		}
		return tx.Model(&Strategy{}).Where("id = ?", id).Updates(map[string]any{"engine_version": EngineVersion, "active": true}).Error
	})
}

// usableStrategy returns a strategy whose rules still mean what they meant when saved.
func (s *Store) usableStrategy(id string) (*Strategy, error) {
	strategy, err := s.StrategyByID(id)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrStrategyAbsent
	case err != nil:
		return nil, err
	case strategy.Retired:
		return nil, ErrStrategyRetired
	}
	return strategy, nil
}

// carriedSuffix is the " (2)" a strategy's name gains when it is carried forward.
var carriedSuffix = regexp.MustCompile(` \(\d+\)$`)

// carryForward keeps one strategy's records to one engine. When the forecast code has
// changed since the strategy in use was saved, a copy of its rules under a new name
// takes over, and the old one stays as it was with what it recorded.
func (s *Store) carryForward(now time.Time) error {
	var current []Strategy
	if err := s.db.Where("active").Limit(1).Find(&current).Error; err != nil {
		return err
	}
	if len(current) == 0 || current[0].EngineVersion == EngineVersion {
		return nil
	}
	old := &current[0]
	if err := old.decode(); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		name, err := carriedName(tx, old.Name)
		if err != nil {
			return err
		}
		if err := tx.Model(&Strategy{}).Where("id = ?", old.ID).Update("active", false).Error; err != nil {
			return err
		}
		_, err = s.createStrategy(tx, old.Config, name, true, now)
		return err
	})
}

// carriedName names a copy after the engine it runs under, past any name already taken.
func carriedName(tx *gorm.DB, name string) (string, error) {
	base := carriedSuffix.ReplaceAllString(name, "")
	candidate := fmt.Sprintf("%s (%d)", base, EngineVersion)
	for tries := 2; ; tries++ {
		var taken int64
		if err := tx.Model(&Strategy{}).Where("name = ?", candidate).Count(&taken).Error; err != nil {
			return "", err
		}
		if taken == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s (%d.%d)", base, EngineVersion, tries)
	}
}

// SetAlerts stores what can alert.
func (s *Store) SetAlerts(threshold int, intervals []string) error {
	return database.PatchSetting(s.db, settingsModule, map[string]any{"alert_threshold": threshold, "alert_intervals": intervals})
}

// signalQuery selects signals with their symbol and interval.
func (s *Store) signalQuery() *gorm.DB {
	return s.db.Table("forecast.signals AS r").Select("r.*, i.symbol, m.interval").
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
// These run often, so each looks up one row per series instead of walking every signal.
func (s *Store) LatestOpenTimes(strategyID string) (map[int64]int64, error) {
	var rows []struct {
		SeriesID int64
		Latest   time.Time
	}
	err := s.db.Raw(`
		SELECT m.id AS series_id, newest.open_at AS latest
		FROM market.series AS m
		CROSS JOIN LATERAL (
			SELECT open_at FROM forecast.signals
			WHERE strategy_id = ? AND series_id = m.id
			ORDER BY open_at DESC LIMIT 1
		) AS newest`, strategyID).Scan(&rows).Error
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
	err := s.db.Raw(`
		SELECT r.*, i.symbol, m.interval
		FROM market.series AS m
		JOIN market.instruments AS i ON i.id = m.instrument_id
		CROSS JOIN LATERAL (
			SELECT * FROM forecast.signals
			WHERE strategy_id = ? AND series_id = m.id
			ORDER BY open_at DESC LIMIT 1
		) AS r
		WHERE m.interval = ?`, strategyID, interval).Scan(&signals).Error
	return signals, err
}

// Coins lists the coins a strategy has forecast.
func (s *Store) Coins(strategyID string) ([]string, error) {
	var coins []string
	err := s.db.Raw(`
		SELECT DISTINCT i.symbol FROM market.series AS m
		JOIN market.instruments AS i ON i.id = m.instrument_id
		WHERE EXISTS (SELECT 1 FROM forecast.signals WHERE strategy_id = ? AND series_id = m.id)
		ORDER BY i.symbol`, strategyID).Scan(&coins).Error
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

// DueSignals returns open signals whose target candle has closed, oldest first. One on
// a series that stopped moving forward waits without taking up room in the batch.
func (s *Store) DueSignals(limit int) ([]Signal, error) {
	var signals []Signal
	err := s.signalQuery().Where("r.status = ? AND r.target_at <= m.last_closed_at", StatusOpen).
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
	// Apart from judged, only independent signals count: overlapping windows would count
	// one move many times.
	independent := signal.Independent
	rose := signal.TargetClose > signal.Close
	hit, miss := signal.Outcome == OutcomeHit, signal.Outcome == OutcomeMiss
	move := 0.0
	if independent {
		move = signal.ActualMovePct
	}
	opened := at(signal.OpenTime)
	return tx.Exec(`
		INSERT INTO forecast.outcome_stats (
			strategy_id, series_id, month, direction, regime, confidence_band,
			judged, moved, rose, hits, misses, move_sum
		) VALUES (?, ?, ?::date, ?, ?, ?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT (strategy_id, series_id, month, direction, regime, confidence_band) DO UPDATE SET
			judged = forecast.outcome_stats.judged + 1,
			moved = forecast.outcome_stats.moved + EXCLUDED.moved,
			rose = forecast.outcome_stats.rose + EXCLUDED.rose,
			hits = forecast.outcome_stats.hits + EXCLUDED.hits,
			misses = forecast.outcome_stats.misses + EXCLUDED.misses,
			move_sum = forecast.outcome_stats.move_sum + EXCLUDED.move_sum`,
		signal.StrategyID, signal.SeriesID, fmt.Sprintf("%04d-%02d-01", opened.Year(), opened.Month()),
		signal.Direction, signal.Regime, confidenceBand(signal.Confidence),
		count(independent), count(independent && rose), count(independent && hit), count(independent && miss), move,
	).Error
}

// IndependentPoints returns a strategy's independent signals for an interval, as
// points to calibrate on.
func (s *Store) IndependentPoints(strategyID, interval string) ([]CalibrationPoint, error) {
	var rows []struct {
		ProbFull float64
		Up       bool
		OpenAt   time.Time
		Horizon  int64
	}
	err := s.db.Table("forecast.signals AS r").Select("r.prob_full, r.target_close > r.close AS up, r.open_at, r.horizon").
		Joins("JOIN market.series AS m ON m.id = r.series_id").
		Where("r.strategy_id = ? AND m.interval = ? AND r.independent", strategyID, interval).
		Order("r.series_id, r.open_at").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	step := intervalMillis(interval)
	points := make([]CalibrationPoint, len(rows))
	for i, row := range rows {
		points[i] = CalibrationPoint{Prob: row.ProbFull, Up: row.Up, Moment: row.OpenAt.UnixMilli() / (max(row.Horizon, 1) * step)}
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

func (s *Store) removeCalibration(strategyID, interval string) error {
	return s.db.Where("strategy_id = ? AND interval = ?", strategyID, interval).Delete(&Calibration{}).Error
}

func (s *Store) alertArm(seriesID int64) bool {
	var arm AlertState
	if err := s.db.First(&arm, "series_id = ?", seriesID).Error; err != nil {
		return false
	}
	return arm.Alerted
}

func (s *Store) setAlertArm(seriesID int64, alerted bool) error {
	return s.db.Save(&AlertState{SeriesID: seriesID, Alerted: alerted}).Error
}

// backtestName tidies what a run is to be called, or refuses it. Empty is fine: the run
// then goes by its strategy's name.
func backtestName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxNameLength {
		return "", ErrBacktestName
	}
	return name, nil
}

// CreateBacktest records a run about to start.
func (s *Store) CreateBacktest(strategyID, name string, options BacktestOptions, now time.Time) (*Backtest, error) {
	name, err := backtestName(name)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	run := &Backtest{StrategyID: strategyID, Name: name, OptionsJSON: string(body), Options: options, Status: BacktestRunning, Total: len(options.Symbols) * len(options.Intervals), CreatedAt: now.UnixMilli()}
	return run, s.db.Create(run).Error
}

// RenameBacktest gives a run another name, or with an empty one, its strategy's again.
func (s *Store) RenameBacktest(id int64, name string) error {
	name, err := backtestName(name)
	if err != nil {
		return err
	}
	result := s.db.Model(&Backtest{}).Where("id = ?", id).Update("name", name)
	if result.Error == nil && result.RowsAffected == 0 {
		return ErrBacktestAbsent
	}
	return result.Error
}

// DeleteBacktest removes a run that is not going.
func (s *Store) DeleteBacktest(id int64) error {
	var run Backtest
	err := s.db.Select("id, status").First(&run, id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrBacktestAbsent
	case err != nil:
		return err
	case run.Status == BacktestRunning:
		return ErrBacktestRunning
	}
	return s.db.Delete(&Backtest{}, id).Error
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
		"status": BacktestFailed, "error": BacktestInterrupted, "finished_at": now.UTC(),
	}).Error
}

func (b *Backtest) decode() error {
	if err := json.Unmarshal([]byte(b.OptionsJSON), &b.Options); err != nil {
		return err
	}
	if b.SummaryJSON != "" {
		b.Summary = &Metrics{}
		if err := json.Unmarshal([]byte(b.SummaryJSON), b.Summary); err != nil {
			return err
		}
	}
	if b.ReportJSON != "" {
		b.Report = &Report{}
		if err := json.Unmarshal([]byte(b.ReportJSON), b.Report); err != nil {
			return err
		}
		b.TestedFrom, b.TestedTo = b.Report.FirstTime, b.Report.LastTime
	}
	return nil
}

// Backtests lists runs newest first, with each report's overall line for comparing them.
func (s *Store) Backtests(limit int) ([]*Backtest, error) {
	var runs []*Backtest
	err := s.db.Select("id, strategy_id, name, options, status, done, total, error, created_at, finished_at, report->'overall' AS summary, (report->>'first_time')::bigint AS tested_from, (report->>'last_time')::bigint AS tested_to").
		Order("id DESC").Limit(limit).Find(&runs).Error
	if err != nil {
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

// SignalStats is how calls have turned out. Apart from Judged it counts only independent
// signals, whose windows do not overlap, so it can be set beside a backtest. The rates
// are nil until there is something to work them out from.
type SignalStats struct {
	// Judged counts every judged signal, overlapping ones too.
	Judged int64 `json:"judged"`
	// Samples counts calls that moved: Hits and Misses.
	Samples int64 `json:"samples"`
	// Moments counts the separate stretches of time those calls come from. Coins move
	// together, so a hundred coins over one stretch are far from a hundred answers.
	Moments int64    `json:"moments"`
	Hits    int64    `json:"hits"`
	Misses  int64    `json:"misses"`
	HitRate *float64 `json:"hit_rate"`
	// ChanceRate is what the same mix of up and down calls would hit with no skill. A
	// hit rate has to beat this to mean anything.
	ChanceRate *float64 `json:"chance_rate"`
	// AvgReturnAfterFee is the average return of acting on each call.
	AvgReturnAfterFee *float64 `json:"avg_return_after_fee_pct"`
}

// outcomeRow is one line of outcome_stats, or several added up.
type outcomeRow struct {
	Interval       string
	Regime         string
	Direction      string
	ConfidenceBand int
	Judged         int64
	Moved          int64
	Rose           int64
	Hits           int64
	Misses         int64
	MoveSum        float64
}

const outcomeColumns = `COALESCE(SUM(o.judged), 0) AS judged, COALESCE(SUM(o.moved), 0) AS moved,
	COALESCE(SUM(o.rose), 0) AS rose, COALESCE(SUM(o.hits), 0) AS hits,
	COALESCE(SUM(o.misses), 0) AS misses, COALESCE(SUM(o.move_sum), 0) AS move_sum`

// outcomeTotals adds rows up, keeping up and down calls apart for the chance rate and
// the sign of their returns.
type outcomeTotals struct {
	judged, moved, rose, upCalls, downCalls, hits, misses int64
	signedMove                                            float64
}

func (t *outcomeTotals) add(row outcomeRow) {
	t.judged += row.Judged
	t.moved += row.Moved
	t.rose += row.Rose
	t.hits += row.Hits
	t.misses += row.Misses
	switch row.Direction {
	case DirectionUp:
		t.upCalls += row.Hits + row.Misses
		t.signedMove += row.MoveSum
	case DirectionDown:
		t.downCalls += row.Hits + row.Misses
		t.signedMove -= row.MoveSum
	}
}

func (t *outcomeTotals) stats() SignalStats {
	stats := SignalStats{Judged: t.judged, Samples: t.hits + t.misses, Hits: t.hits, Misses: t.misses}
	stats.ChanceRate = chanceRate(t.upCalls, t.downCalls, t.rose, t.moved)
	if stats.Samples > 0 {
		rate := float64(t.hits) / float64(stats.Samples)
		stats.HitRate = &rate
	}
	if calls := t.upCalls + t.downCalls; calls > 0 {
		average := t.signedMove/float64(calls) - RoundTripFeePct
		stats.AvgReturnAfterFee = &average
	}
	return stats
}

// SignalStats adds up a series' judged forecasts for a strategy.
func (s *Store) SignalStats(strategyID string, seriesID int64) (SignalStats, error) {
	var rows []outcomeRow
	err := s.db.Table("forecast.outcome_stats AS o").Select("o.direction, "+outcomeColumns).
		Where("o.strategy_id = ? AND o.series_id = ?", strategyID, seriesID).Group("o.direction").Scan(&rows).Error
	if err != nil {
		return SignalStats{}, err
	}
	var totals outcomeTotals
	for _, row := range rows {
		totals.add(row)
	}
	stats := totals.stats()
	// One series' independent calls never share a stretch of time.
	stats.Moments = stats.Samples
	return stats, nil
}

// madeCall is one independent call a strategy made and when, which outcome_stats no
// longer knows.
type madeCall struct {
	Interval   string
	Regime     string
	Direction  string
	Confidence int
	OpenAt     time.Time
	Horizon    int64
}

func (s *Store) madeCalls(strategyID string) ([]madeCall, error) {
	var calls []madeCall
	err := s.db.Table("forecast.signals AS r").Select("m.interval, r.regime, r.direction, r.confidence, r.open_at, r.horizon").
		Joins("JOIN market.series AS m ON m.id = r.series_id").
		Where("r.strategy_id = ? AND r.independent AND r.direction <> ?", strategyID, DirectionNeutral).Scan(&calls).Error
	return calls, err
}

// momentsBy counts the separate moments behind the calls of each group. A moment belongs
// to its interval, so an hour at 1h and an hour at 4h are not the same one.
func momentsBy(calls []madeCall, key func(outcomeRow) string) map[string]int64 {
	type moment struct {
		group, interval string
		index           int64
	}
	seen := map[moment]bool{}
	counts := map[string]int64{}
	for _, call := range calls {
		group := key(outcomeRow{Interval: call.Interval, Regime: call.Regime, Direction: call.Direction, ConfidenceBand: confidenceBand(call.Confidence)})
		at := moment{group, call.Interval, call.OpenAt.UnixMilli() / (max(call.Horizon, 1) * intervalMillis(call.Interval))}
		if !seen[at] {
			seen[at] = true
			counts[group]++
		}
	}
	return counts
}

// Performance is how a strategy has done live, overall and by what might explain it.
type Performance struct {
	Overall      SignalStats            `json:"overall"`
	ByInterval   map[string]SignalStats `json:"by_interval"`
	ByRegime     map[string]SignalStats `json:"by_regime"`
	ByDirection  map[string]SignalStats `json:"by_direction"`
	ByConfidence map[string]SignalStats `json:"by_confidence"`
	// Calibration says, per interval, how far its scores are from showing as probabilities.
	Calibration []CalibrationStatus `json:"calibration"`
}

// CalibrationStatus is where one interval stands on earning a curve.
type CalibrationStatus struct {
	Interval string `json:"interval"`
	Moments  int    `json:"moments"`
	Needed   int    `json:"needed"`
	Active   bool   `json:"active"`
	BuiltAt  int64  `json:"built_at,omitempty"`
}

// Performance adds up everything a strategy has been judged on.
func (s *Store) Performance(strategyID string) (*Performance, error) {
	var rows []outcomeRow
	err := s.db.Table("forecast.outcome_stats AS o").
		Select("m.interval, o.regime, o.direction, o.confidence_band, "+outcomeColumns).
		Joins("JOIN market.series AS m ON m.id = o.series_id").
		Where("o.strategy_id = ?", strategyID).
		Group("m.interval, o.regime, o.direction, o.confidence_band").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	calls, err := s.madeCalls(strategyID)
	if err != nil {
		return nil, err
	}
	var overall outcomeTotals
	performance := &Performance{}
	groups := []struct {
		dest *map[string]SignalStats
		key  func(outcomeRow) string
		// Whether a call was up or down is the very thing being compared, so chance goes
		// by how often price rose overall. Within the group it would equal the hit rate.
		overallBase bool
	}{
		{dest: &performance.ByInterval, key: func(row outcomeRow) string { return row.Interval }},
		{dest: &performance.ByRegime, key: func(row outcomeRow) string { return row.Regime }},
		{dest: &performance.ByDirection, key: func(row outcomeRow) string { return row.Direction }, overallBase: true},
		{dest: &performance.ByConfidence, key: func(row outcomeRow) string { return strconv.Itoa(row.ConfidenceBand) }},
	}
	for _, row := range rows {
		overall.add(row)
	}
	performance.Overall = overall.stats()
	performance.Overall.Moments = momentsBy(calls, func(outcomeRow) string { return "" })[""]
	for _, group := range groups {
		moments := momentsBy(calls, group.key)
		totals := map[string]*outcomeTotals{}
		for _, row := range rows {
			key := group.key(row)
			if totals[key] == nil {
				totals[key] = &outcomeTotals{}
			}
			totals[key].add(row)
		}
		*group.dest = make(map[string]SignalStats, len(totals))
		for key, total := range totals {
			if group.overallBase {
				total.rose, total.moved = overall.rose, overall.moved
			}
			stats := total.stats()
			stats.Moments = moments[key]
			(*group.dest)[key] = stats
		}
	}

	var curves []Calibration
	if err := s.db.Where("strategy_id = ?", strategyID).Find(&curves).Error; err != nil {
		return nil, err
	}
	built := make(map[string]int64, len(curves))
	for _, curve := range curves {
		built[curve.Interval] = curve.BuiltAt
	}
	for _, interval := range Intervals {
		points, err := s.IndependentPoints(strategyID, interval)
		if err != nil {
			return nil, err
		}
		_, active := built[interval]
		performance.Calibration = append(performance.Calibration, CalibrationStatus{
			Interval: interval, Moments: countMoments(points), Needed: MinCalibrationMoments, Active: active, BuiltAt: built[interval],
		})
	}
	return performance, nil
}
