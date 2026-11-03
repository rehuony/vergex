package trader

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// EquityStore account equity storage (for plotting return curves)
type EquityStore struct {
	db *gorm.DB
}

// EquitySnapshot equity snapshot
type EquitySnapshot struct {
	TraderID      string    `gorm:"column:trader_id;primaryKey" json:"trader_id"`
	Timestamp     time.Time `gorm:"column:taken_at;primaryKey" json:"timestamp"`
	TotalEquity   float64   `gorm:"column:total_equity" json:"total_equity"`
	UnrealizedPnL float64   `gorm:"column:unrealized_pnl" json:"unrealized_pnl"`
}

func (EquitySnapshot) TableName() string { return "trader.equity_snapshots" }

// NewEquityStore creates a new EquityStore
func NewEquityStore(db *gorm.DB) *EquityStore {
	return &EquityStore{db: db}
}

// Save saves equity snapshot
func (s *EquityStore) Save(snapshot *EquitySnapshot) error {
	if snapshot.Timestamp.IsZero() {
		snapshot.Timestamp = time.Now().UTC()
	} else {
		snapshot.Timestamp = snapshot.Timestamp.UTC()
	}

	if err := s.db.Create(snapshot).Error; err != nil {
		return fmt.Errorf("failed to save equity snapshot: %w", err)
	}
	return nil
}

// GetSeries returns at most maxPoints snapshots since the given time (all of
// them when since is zero), oldest first, thinned evenly and keeping the latest.
func (s *EquityStore) GetSeries(traderID string, since time.Time, maxPoints int) ([]*EquitySnapshot, error) {
	query := s.db.Select("taken_at", "total_equity", "unrealized_pnl").Where("trader_id = ?", traderID)
	if !since.IsZero() {
		query = query.Where("taken_at >= ?", since)
	}
	var snapshots []*EquitySnapshot
	if err := query.Order("taken_at ASC").Find(&snapshots).Error; err != nil {
		return nil, fmt.Errorf("failed to query equity series: %w", err)
	}
	if maxPoints < 2 || len(snapshots) <= maxPoints {
		return snapshots, nil
	}
	thinned := make([]*EquitySnapshot, 0, maxPoints)
	last := len(snapshots) - 1
	for i := 0; i < maxPoints; i++ {
		thinned = append(thinned, snapshots[i*last/(maxPoints-1)])
	}
	return thinned, nil
}

// PeakSince is the highest equity recorded since a time, or 0 with none. A zero time
// means all of them.
func (s *EquityStore) PeakSince(traderID string, since time.Time) (float64, error) {
	var peak float64
	query := s.db.Model(&EquitySnapshot{}).Select("COALESCE(MAX(total_equity), 0)").Where("trader_id = ?", traderID)
	if !since.IsZero() {
		query = query.Where("taken_at >= ?", since)
	}
	if err := query.Scan(&peak).Error; err != nil {
		return 0, fmt.Errorf("failed to query peak equity: %w", err)
	}
	return peak, nil
}

// FirstSince is the first equity recorded since a time, or 0 with none.
func (s *EquityStore) FirstSince(traderID string, since time.Time) (float64, error) {
	var first float64
	err := s.db.Model(&EquitySnapshot{}).Select("total_equity").
		Where("trader_id = ? AND taken_at >= ?", traderID, since).
		Order("taken_at ASC").Limit(1).Scan(&first).Error
	if err != nil {
		return 0, fmt.Errorf("failed to query first equity: %w", err)
	}
	return first, nil
}

// GetAllTradersLatest gets latest equity for all traders
func (s *EquityStore) GetAllTradersLatest() (map[string]*EquitySnapshot, error) {
	var snapshots []*EquitySnapshot
	err := s.db.Raw(`
		SELECT e.*
		FROM trader.traders t
		CROSS JOIN LATERAL (
			SELECT * FROM trader.equity_snapshots
			WHERE trader_id = t.id
			ORDER BY taken_at DESC
			LIMIT 1
		) e
	`).Scan(&snapshots).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query latest equity: %w", err)
	}

	result := make(map[string]*EquitySnapshot)
	for _, snap := range snapshots {
		result[snap.TraderID] = snap
	}
	return result, nil
}
