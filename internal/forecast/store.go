package forecast

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Record states.
const (
	StatusOpen     = "open"     // Waiting for its target candle.
	StatusVerified = "verified" // Judged.
	StatusVoid     = "void"     // The target candle does not exist.
)

// Outcomes of a verified record.
const (
	OutcomeHit     = "hit"
	OutcomeMiss    = "miss"
	OutcomeNeutral = "neutral" // No direction was called, or price did not move.
)

// Version is an immutable strategy version. Editing saves a new one.
type Version struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Note          string `json:"note"`
	EngineVersion int    `gorm:"not null" json:"engine_version"`
	ConfigJSON    string `gorm:"column:config;type:text;not null" json:"-"`
	Config        Config `gorm:"-" json:"config"`
	CreatedAt     int64  `gorm:"not null" json:"created_at"`
}

func (Version) TableName() string { return "forecast_strategy_versions" }

// Settings is the single row of forecast settings.
type Settings struct {
	ID              uint  `gorm:"primaryKey" json:"-"`
	ActiveVersionID int64 `json:"active_version_id"`
	// AlertThreshold is the score at or beyond which a signal alerts: 70 means 70
	// and above upwards, 30 and below downwards.
	AlertThreshold int `json:"alert_threshold"`
}

func (Settings) TableName() string { return "forecast_settings" }

// DefaultAlertThreshold is used until the user sets one.
const DefaultAlertThreshold = 70

// Record is one forecast and, later, how it turned out.
type Record struct {
	ID        int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	VersionID int64   `gorm:"not null;uniqueIndex:idx_forecast_record_key,priority:1;index:idx_forecast_record_latest,priority:1" json:"version_id"`
	SeriesID  int64   `gorm:"not null;uniqueIndex:idx_forecast_record_key,priority:2;index:idx_forecast_record_latest,priority:3" json:"series_id"`
	OpenTime  int64   `gorm:"not null;uniqueIndex:idx_forecast_record_key,priority:3;index:idx_forecast_record_latest,priority:4" json:"open_time"`
	Symbol    string  `gorm:"not null" json:"symbol"`
	Interval  string  `gorm:"column:interval_name;not null;index:idx_forecast_record_latest,priority:2" json:"interval"`
	Close     float64 `gorm:"not null" json:"close"`
	// ProbCandles uses candles only; ProbFull adds derivatives.
	ProbCandles float64 `gorm:"not null" json:"prob_candles"`
	ProbFull    float64 `gorm:"not null" json:"prob_full"`
	// DisplayProb is what to show: ProbFull until calibrated, then the calibrated value.
	DisplayProb     float64 `gorm:"not null" json:"display_prob"`
	Calibrated      bool    `gorm:"not null" json:"calibrated"`
	Direction       string  `gorm:"not null" json:"direction"`
	Confidence      int     `gorm:"not null" json:"confidence"`
	ExpectedMovePct float64 `gorm:"not null" json:"expected_move_pct"`
	Support         float64 `gorm:"not null" json:"support"`
	Resistance      float64 `gorm:"not null" json:"resistance"`
	Regime          string  `gorm:"not null" json:"regime"`
	Horizon         int     `gorm:"not null" json:"horizon"`
	// TargetTime is the open time of the candle whose close judges this forecast.
	TargetTime    int64   `gorm:"not null;index:idx_forecast_record_due,priority:2" json:"target_time"`
	Status        string  `gorm:"not null;index:idx_forecast_record_due,priority:1" json:"status"`
	Outcome       string  `json:"outcome,omitempty"`
	TargetClose   float64 `json:"target_close,omitempty"`
	ActualMovePct float64 `json:"actual_move_pct,omitempty"`
	VerifiedAt    int64   `json:"verified_at,omitempty"`
}

func (Record) TableName() string { return "forecast_records" }

// Calibration is a version's curve for one interval.
type Calibration struct {
	VersionID int64  `gorm:"primaryKey;autoIncrement:false"`
	Interval  string `gorm:"column:interval_name;primaryKey"`
	Samples   int    `gorm:"not null"`
	Knots     string `gorm:"type:text;not null"`
	BuiltAt   int64  `gorm:"not null"`
}

func (Calibration) TableName() string { return "forecast_calibrations" }

// AlertArm remembers that a series has alerted; it re-arms once the score comes
// back inside the threshold.
type AlertArm struct {
	SeriesID int64 `gorm:"primaryKey;autoIncrement:false"`
	Alerted  bool  `gorm:"not null"`
}

func (AlertArm) TableName() string { return "forecast_alert_arms" }

// Backtest states.
const (
	BacktestRunning  = "running"
	BacktestFinished = "finished"
	BacktestFailed   = "failed"
)

// Backtest is one run.
type Backtest struct {
	ID          int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	VersionID   int64           `gorm:"not null" json:"version_id"`
	OptionsJSON string          `gorm:"column:options;type:text;not null" json:"-"`
	Options     BacktestOptions `gorm:"-" json:"options"`
	Status      string          `gorm:"not null" json:"status"`
	Done        int             `gorm:"not null" json:"done"`
	Total       int             `gorm:"not null" json:"total"`
	ReportJSON  string          `gorm:"column:report;type:text" json:"-"`
	Report      *Report         `gorm:"-" json:"report,omitempty"`
	Error       string          `json:"error,omitempty"`
	CreatedAt   int64           `gorm:"not null" json:"created_at"`
	FinishedAt  int64           `json:"finished_at,omitempty"`
}

func (Backtest) TableName() string { return "forecast_backtests" }

// Store keeps forecasts beside the candles in the market database.
type Store struct {
	db *gorm.DB
}

// NewStore prepares the forecast tables.
func NewStore(db *gorm.DB) (*Store, error) {
	if err := db.AutoMigrate(&Version{}, &Settings{}, &Record{}, &Calibration{}, &AlertArm{}, &Backtest{}); err != nil {
		return nil, fmt.Errorf("migrate forecast tables: %w", err)
	}
	return &Store{db: db}, nil
}

func (v *Version) decode() error {
	return json.Unmarshal([]byte(v.ConfigJSON), &v.Config)
}

// CreateVersion saves a new immutable version.
func (s *Store) CreateVersion(config Config, note string, now time.Time) (*Version, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	version := &Version{Note: note, EngineVersion: EngineVersion, ConfigJSON: string(body), Config: config, CreatedAt: now.UnixMilli()}
	if err := s.db.Create(version).Error; err != nil {
		return nil, err
	}
	return version, nil
}

// Versions lists every version, newest first.
func (s *Store) Versions() ([]*Version, error) {
	var versions []*Version
	if err := s.db.Order("id DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	for _, version := range versions {
		if err := version.decode(); err != nil {
			return nil, err
		}
	}
	return versions, nil
}

// VersionByID returns one version.
func (s *Store) VersionByID(id int64) (*Version, error) {
	var version Version
	if err := s.db.First(&version, id).Error; err != nil {
		return nil, err
	}
	return &version, version.decode()
}

// LoadSettings returns the settings, creating the first version when there is none.
func (s *Store) LoadSettings(now time.Time) (Settings, error) {
	var settings Settings
	err := s.db.First(&settings, 1).Error
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return settings, err
	}
	version, err := s.CreateVersion(DefaultConfig(), "Default", now)
	if err != nil {
		return settings, err
	}
	settings = Settings{ID: 1, ActiveVersionID: version.ID, AlertThreshold: DefaultAlertThreshold}
	return settings, s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&settings).Error
}

// ActiveVersion returns the version in use.
func (s *Store) ActiveVersion(now time.Time) (*Version, error) {
	settings, err := s.LoadSettings(now)
	if err != nil {
		return nil, err
	}
	return s.VersionByID(settings.ActiveVersionID)
}

// Activate makes a version the one in use. Records stay with their own versions.
func (s *Store) Activate(id int64) error {
	if _, err := s.VersionByID(id); err != nil {
		return err
	}
	return s.db.Model(&Settings{}).Where("id = 1").Update("active_version_id", id).Error
}

// SetAlertThreshold stores the alert threshold.
func (s *Store) SetAlertThreshold(threshold int) error {
	return s.db.Model(&Settings{}).Where("id = 1").Update("alert_threshold", threshold).Error
}

// SaveRecord stores a forecast, doing nothing if that candle already has one.
// It reports whether the record is new.
func (s *Store) SaveRecord(record *Record) (bool, error) {
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	return result.RowsAffected > 0, result.Error
}

// LatestOpenTimes returns the newest recorded candle of each series for a version.
func (s *Store) LatestOpenTimes(versionID int64) (map[int64]int64, error) {
	var rows []struct {
		SeriesID int64
		Latest   int64
	}
	err := s.db.Model(&Record{}).Select("series_id, MAX(open_time) AS latest").
		Where("version_id = ?", versionID).Group("series_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	latest := make(map[int64]int64, len(rows))
	for _, row := range rows {
		latest[row.SeriesID] = row.Latest
	}
	return latest, nil
}

// LatestRecords returns each series' newest record for a version and interval.
func (s *Store) LatestRecords(versionID int64, interval string) ([]Record, error) {
	var records []Record
	latest := s.db.Model(&Record{}).Select("series_id, MAX(open_time) AS latest").
		Where("version_id = ? AND interval_name = ?", versionID, interval).Group("series_id")
	err := s.db.Table("forecast_records AS r").Select("r.*").
		Joins("JOIN (?) AS l ON r.series_id = l.series_id AND r.open_time = l.latest", latest).
		Where("r.version_id = ?", versionID).Find(&records).Error
	return records, err
}

// RecentRecords returns a series' newest records for a version.
func (s *Store) RecentRecords(versionID, seriesID int64, limit int) ([]Record, error) {
	var records []Record
	err := s.db.Where("version_id = ? AND series_id = ?", versionID, seriesID).
		Order("open_time DESC").Limit(limit).Find(&records).Error
	return records, err
}

// DueRecords returns open records whose target candle should have closed by now.
func (s *Store) DueRecords(before int64, limit int) ([]Record, error) {
	var records []Record
	err := s.db.Where("status = ? AND target_time <= ?", StatusOpen, before).
		Order("target_time ASC").Limit(limit).Find(&records).Error
	return records, err
}

func (s *Store) saveVerification(record *Record) error {
	return s.db.Model(&Record{}).Where("id = ?", record.ID).Updates(map[string]any{
		"status": record.Status, "outcome": record.Outcome, "target_close": record.TargetClose,
		"actual_move_pct": record.ActualMovePct, "verified_at": record.VerifiedAt,
	}).Error
}

// VerifiedRecords returns a version's verified records for an interval, in the
// order needed to pick non-overlapping ones.
func (s *Store) VerifiedRecords(versionID int64, interval string) ([]Record, error) {
	var records []Record
	err := s.db.Where("version_id = ? AND interval_name = ? AND status = ?", versionID, interval, StatusVerified).
		Order("series_id ASC, open_time ASC").Find(&records).Error
	return records, err
}

// Calibrations returns a version's curves by interval.
func (s *Store) Calibrations(versionID int64) (map[string][]Knot, error) {
	var rows []Calibration
	if err := s.db.Where("version_id = ?", versionID).Find(&rows).Error; err != nil {
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
func (s *Store) CreateBacktest(versionID int64, options BacktestOptions, now time.Time) (*Backtest, error) {
	body, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	run := &Backtest{VersionID: versionID, OptionsJSON: string(body), Options: options, Status: BacktestRunning, Total: len(options.Symbols) * len(options.Intervals), CreatedAt: now.UnixMilli()}
	return run, s.db.Create(run).Error
}

func (s *Store) updateBacktestProgress(id int64, done int) error {
	return s.db.Model(&Backtest{}).Where("id = ?", id).Update("done", done).Error
}

func (s *Store) finishBacktest(id int64, report *Report, failure string, now time.Time) error {
	updates := map[string]any{"finished_at": now.UnixMilli(), "status": BacktestFinished}
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
		"status": BacktestFailed, "error": "interrupted by a restart", "finished_at": now.UnixMilli(),
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

// HitStats counts a series' judged forecasts for a version.
func (s *Store) HitStats(versionID, seriesID int64) (hits, misses int64, err error) {
	var rows []struct {
		Outcome string
		Count   int64
	}
	err = s.db.Model(&Record{}).Select("outcome, COUNT(*) AS count").
		Where("version_id = ? AND series_id = ? AND status = ?", versionID, seriesID, StatusVerified).
		Group("outcome").Scan(&rows).Error
	for _, row := range rows {
		switch row.Outcome {
		case OutcomeHit:
			hits = row.Count
		case OutcomeMiss:
			misses = row.Count
		}
	}
	return hits, misses, err
}
