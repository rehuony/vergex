package store

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
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TraderID         string    `gorm:"column:trader_id;not null;index:idx_equity_trader_time" json:"trader_id"`
	Timestamp        time.Time `gorm:"not null;index:idx_equity_trader_time,sort:desc;index:idx_equity_timestamp,sort:desc" json:"timestamp"`
	TotalEquity      float64   `gorm:"column:total_equity;not null;default:0" json:"total_equity"`
	Balance          float64   `gorm:"not null;default:0" json:"balance"`
	AvailableBalance float64   `gorm:"column:available_balance;not null;default:0" json:"available_balance"`
	UnrealizedPnL    float64   `gorm:"column:unrealized_pnl;not null;default:0" json:"unrealized_pnl"`
	PositionCount    int       `gorm:"column:position_count;default:0" json:"position_count"`
	MarginUsedPct    float64   `gorm:"column:margin_used_pct;default:0" json:"margin_used_pct"`
	CreatedAt        time.Time `json:"created_at"`
}

func (EquitySnapshot) TableName() string { return "trader_equity_snapshots" }

// NewEquityStore creates a new EquityStore
func NewEquityStore(db *gorm.DB) *EquityStore {
	return &EquityStore{db: db}
}

// initTables initializes equity tables
func (s *EquityStore) initTables() error {
	return s.db.AutoMigrate(&EquitySnapshot{})
}

// Save saves equity snapshot
func (s *EquityStore) Save(snapshot *EquitySnapshot) error {
	if snapshot.Timestamp.IsZero() {
		snapshot.Timestamp = time.Now().UTC()
	} else {
		snapshot.Timestamp = snapshot.Timestamp.UTC()
	}

	// Omit ID to let PostgreSQL sequence auto-generate it
	// Without this, GORM inserts ID=0 which causes duplicate key errors
	if err := s.db.Omit("ID").Create(snapshot).Error; err != nil {
		return fmt.Errorf("failed to save equity snapshot: %w", err)
	}
	return nil
}

// GetLatest gets the latest N equity records for specified trader (sorted in ascending chronological order: old to new)
func (s *EquityStore) GetLatest(traderID string, limit int) ([]*EquitySnapshot, error) {
	var snapshots []*EquitySnapshot
	err := s.db.Where("trader_id = ?", traderID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&snapshots).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query equity records: %w", err)
	}

	// Reverse the array to sort time from old to new (suitable for plotting curves)
	for i, j := 0, len(snapshots)-1; i < j; i, j = i+1, j-1 {
		snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
	}

	return snapshots, nil
}

// GetByTimeRange gets equity records within specified time range
func (s *EquityStore) GetByTimeRange(traderID string, start, end time.Time) ([]*EquitySnapshot, error) {
	var snapshots []*EquitySnapshot
	err := s.db.Where("trader_id = ? AND timestamp >= ? AND timestamp <= ?", traderID, start, end).
		Order("timestamp ASC").
		Find(&snapshots).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query equity records: %w", err)
	}
	return snapshots, nil
}

// GetSeries returns at most maxPoints snapshots since the given time (all of
// them when since is zero), oldest first, thinned evenly and keeping the latest.
func (s *EquityStore) GetSeries(traderID string, since time.Time, maxPoints int) ([]*EquitySnapshot, error) {
	query := s.db.Select("timestamp", "total_equity", "unrealized_pnl").Where("trader_id = ?", traderID)
	if !since.IsZero() {
		query = query.Where("timestamp >= ?", since)
	}
	var snapshots []*EquitySnapshot
	if err := query.Order("timestamp ASC").Find(&snapshots).Error; err != nil {
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

// GetAllTradersLatest gets latest equity for all traders
func (s *EquityStore) GetAllTradersLatest() (map[string]*EquitySnapshot, error) {
	// Use raw SQL for this complex query with subquery
	var snapshots []*EquitySnapshot
	err := s.db.Raw(`
		SELECT e.id, e.trader_id, e.timestamp, e.total_equity, e.balance,
		       e.available_balance,
		       e.unrealized_pnl, e.position_count, e.margin_used_pct, e.created_at
		FROM trader_equity_snapshots e
		INNER JOIN (
			SELECT trader_id, MAX(timestamp) as max_ts
			FROM trader_equity_snapshots
			GROUP BY trader_id
		) latest ON e.trader_id = latest.trader_id AND e.timestamp = latest.max_ts
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

// CleanOldRecords cleans old records from N days ago
func (s *EquityStore) CleanOldRecords(traderID string, days int) (int64, error) {
	cutoffTime := time.Now().AddDate(0, 0, -days)

	result := s.db.Where("trader_id = ? AND timestamp < ?", traderID, cutoffTime).
		Delete(&EquitySnapshot{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to clean old records: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// GetCount gets record count for specified trader
func (s *EquityStore) GetCount(traderID string) (int, error) {
	var count int64
	err := s.db.Model(&EquitySnapshot{}).Where("trader_id = ?", traderID).Count(&count).Error
	return int(count), err
}
