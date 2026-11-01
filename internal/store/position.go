package store

import (
	"fmt"

	"gorm.io/gorm"
)

// formatDurationMs formats a duration in milliseconds
func formatDurationMs(ms int64) string {
	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24

	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	if hours < 24 {
		remainingMins := minutes % 60
		if remainingMins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, remainingMins)
	}
	remainingHours := hours % 24
	if remainingHours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, remainingHours)
}

// TraderPosition is a position as the trading loop and the interface read it: a row of
// trading.positions with its instrument's names. Times are Unix milliseconds. Once a
// position is closed, Quantity is how much it held.
type TraderPosition struct {
	ID            int64   `gorm:"column:id" json:"id"`
	Source        string  `gorm:"column:source" json:"source"`
	TraderID      string  `gorm:"column:trader_id" json:"trader_id"`
	ExchangeID    string  `gorm:"column:exchange_id" json:"exchange_id"`
	ExchangeType  string  `gorm:"column:exchange_type" json:"exchange_type"`
	Symbol        string  `gorm:"column:symbol" json:"symbol"`
	Side          string  `gorm:"column:side" json:"side"`
	EntryQuantity float64 `gorm:"column:entry_quantity" json:"entry_quantity"`
	Quantity      float64 `gorm:"column:quantity" json:"quantity"`
	EntryPrice    float64 `gorm:"column:entry_price" json:"entry_price"`
	EntryTime     int64   `gorm:"column:entry_time;serializer:unixms" json:"entry_time"`
	ExitPrice     float64 `gorm:"column:exit_price" json:"exit_price"`
	ExitTime      int64   `gorm:"column:exit_time;serializer:unixms" json:"exit_time"`
	RealizedPnL   float64 `gorm:"column:realized_pnl" json:"realized_pnl"`
	Fee           float64 `gorm:"column:fee" json:"fee"`
	Leverage      int     `gorm:"column:leverage" json:"leverage"`
	Status        string  `gorm:"column:status" json:"status"`
	CloseReason   string  `gorm:"column:close_reason" json:"close_reason"`
	UpdatedAt     int64   `gorm:"column:updated_at;serializer:unixms;autoUpdateTime:false" json:"updated_at"`
}

const positionColumns = `p.id, p.source, COALESCE(p.source_id, '') AS trader_id, p.account_id::text AS exchange_id, i.venue AS exchange_type,
	i.symbol, upper(p.side) AS side, p.entry_quantity,
	CASE WHEN p.status = 'closed' THEN p.entry_quantity ELSE p.quantity END AS quantity,
	p.entry_price, p.opened_at AS entry_time, p.exit_price, p.closed_at AS exit_time, p.realized_pnl, p.fee,
	COALESCE(p.leverage, 0) AS leverage, upper(p.status) AS status, COALESCE(p.close_reason, '') AS close_reason, p.updated_at`

// PositionStore reads positions.
type PositionStore struct {
	db *gorm.DB
}

// NewPositionStore creates position storage instance
func NewPositionStore(db *gorm.DB) *PositionStore {
	return &PositionStore{db: db}
}

func (s *PositionStore) positions() *gorm.DB {
	return s.db.Table("trading.positions p").Select(positionColumns).
		Joins("JOIN market.instruments i ON i.id = p.instrument_id")
}

// GetOpenPositionBySymbol gets open position for specified symbol and direction
func (s *PositionStore) GetOpenPositionBySymbol(traderID, symbol, side string) (*TraderPosition, error) {
	var positions []TraderPosition
	err := s.positions().Where("p.source = ? AND p.source_id = ? AND i.symbol = ? AND p.side = lower(?) AND p.status = ?", SourceTrader, traderID, symbol, side, PositionOpen).
		Order("p.opened_at DESC").Limit(1).Find(&positions).Error
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, nil
	}
	return &positions[0], nil
}

// closedPositions selects a trader's closed positions.
func (s *PositionStore) closedPositions(traderID string) *gorm.DB {
	return s.positions().Where("p.status = ? AND p.source = ? AND p.source_id = ?", PositionClosed, SourceTrader, traderID)
}

// GetClosedPositions gets closed positions, newest first
func (s *PositionStore) GetClosedPositions(traderID string, limit int) ([]*TraderPosition, error) {
	return s.closedPage(s.closedPositions(traderID), limit)
}

// GetClosedPositionsBefore gets the page of closed positions after a given one in
// that order. The position must be one of the trader's closed positions.
func (s *PositionStore) GetClosedPositionsBefore(traderID string, beforeID int64, limit int) ([]*TraderPosition, error) {
	var cursor []TraderPosition
	if err := s.closedPositions(traderID).Where("p.id = ?", beforeID).Limit(1).Find(&cursor).Error; err != nil {
		return nil, fmt.Errorf("failed to find closed position %d: %w", beforeID, err)
	}
	if len(cursor) == 0 {
		return nil, fmt.Errorf("failed to find closed position %d: %w", beforeID, gorm.ErrRecordNotFound)
	}
	return s.closedPage(
		s.closedPositions(traderID).Where("(p.closed_at, p.id) < (SELECT closed_at, id FROM trading.positions WHERE id = ?)", cursor[0].ID),
		limit,
	)
}

func (s *PositionStore) closedPage(query *gorm.DB, limit int) ([]*TraderPosition, error) {
	// The ID keeps pages apart when positions close at the same moment.
	query = query.Order("p.closed_at DESC, p.id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var positions []*TraderPosition
	if err := query.Find(&positions).Error; err != nil {
		return nil, fmt.Errorf("failed to query closed positions: %w", err)
	}
	return positions, nil
}
