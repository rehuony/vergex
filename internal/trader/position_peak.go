package trader

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PositionPeak is the best a held position has done.
type PositionPeak struct {
	TraderID   string    `gorm:"column:trader_id;primaryKey"`
	Symbol     string    `gorm:"column:symbol;primaryKey"`
	Side       string    `gorm:"column:side;primaryKey"`
	PeakPnLPct float64   `gorm:"column:peak_pnl_pct"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (PositionPeak) TableName() string { return "trader.position_peaks" }

// PeakStore keeps peaks across restarts.
type PeakStore struct {
	db *gorm.DB
}

// NewPeakStore creates a new PeakStore.
func NewPeakStore(db *gorm.DB) *PeakStore {
	return &PeakStore{db: db}
}

// Save records a position's peak.
func (s *PeakStore) Save(peak PositionPeak) error {
	peak.UpdatedAt = time.Now().UTC()
	return s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&peak).Error
}

// Delete forgets a position's peak.
func (s *PeakStore) Delete(traderID, symbol, side string) error {
	return s.db.Where("trader_id = ? AND symbol = ? AND side = ?", traderID, symbol, side).Delete(&PositionPeak{}).Error
}

// Load reads a trader's peaks.
func (s *PeakStore) Load(traderID string) ([]PositionPeak, error) {
	var peaks []PositionPeak
	err := s.db.Where("trader_id = ?", traderID).Find(&peaks).Error
	return peaks, err
}
