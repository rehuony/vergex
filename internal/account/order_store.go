package account

import (
	"fmt"
	"time"
	"vergex/internal/database"

	"gorm.io/gorm"
)

// TraderOrder is an order as the trading loop and the interface read it: a row of
// account.orders with its instrument's names and what its fills cost. Times are Unix
// milliseconds.
type TraderOrder struct {
	ID              int64   `gorm:"column:id" json:"id"`
	Source          string  `gorm:"column:source" json:"source"`
	TraderID        string  `gorm:"column:trader_id" json:"trader_id"`
	AccountID       string  `gorm:"column:account_id" json:"account_id"`
	Venue           string  `gorm:"column:venue" json:"venue"`
	VenueOrderID    string  `gorm:"column:venue_order_id" json:"venue_order_id"`
	ClientID        string  `gorm:"column:client_id" json:"client_id"`
	Symbol          string  `gorm:"column:symbol" json:"symbol"`
	Side            string  `gorm:"column:side" json:"side"`
	PositionSide    string  `gorm:"column:position_side" json:"position_side"`
	Role            string  `gorm:"column:role" json:"role"`
	Type            string  `gorm:"column:type" json:"type"`
	Quantity        float64 `gorm:"column:quantity" json:"quantity"`
	Price           float64 `gorm:"column:price" json:"price"`
	StopPrice       float64 `gorm:"column:stop_price" json:"stop_price"`
	Status          string  `gorm:"column:status" json:"status"`
	FilledQuantity  float64 `gorm:"column:filled_quantity" json:"filled_quantity"`
	AvgFillPrice    float64 `gorm:"column:avg_fill_price" json:"avg_fill_price"`
	Commission      float64 `gorm:"column:commission" json:"commission"`
	CommissionAsset string  `gorm:"column:commission_asset" json:"commission_asset"`
	Leverage        int     `gorm:"column:leverage" json:"leverage"`
	ReduceOnly      bool    `gorm:"column:reduce_only" json:"reduce_only"`
	OrderAction     string  `gorm:"column:order_action" json:"order_action"`
	CreatedAt       int64   `gorm:"column:created_at;serializer:unixms;autoCreateTime:false" json:"created_at"`
	UpdatedAt       int64   `gorm:"column:updated_at;serializer:unixms;autoUpdateTime:false" json:"updated_at"`
	ClosedAt        int64   `gorm:"column:closed_at;serializer:unixms" json:"closed_at"`
	// Error is what the exchange or the system said of a failed order, where it was kept.
	Error string `gorm:"column:error" json:"error,omitempty"`
}

// TraderFill is a fill as the interface reads it: a row of account.fills with its
// instrument's names and its order's owner.
type TraderFill struct {
	ID              int64   `gorm:"column:id" json:"id"`
	Source          string  `gorm:"column:source" json:"source"`
	TraderID        string  `gorm:"column:trader_id" json:"trader_id"`
	AccountID       string  `gorm:"column:account_id" json:"account_id"`
	Venue           string  `gorm:"column:venue" json:"venue"`
	OrderID         int64   `gorm:"column:order_id" json:"order_id"`
	VenueOrderID    string  `gorm:"column:venue_order_id" json:"venue_order_id"`
	VenueTradeID    string  `gorm:"column:venue_trade_id" json:"venue_trade_id"`
	Symbol          string  `gorm:"column:symbol" json:"symbol"`
	Side            string  `gorm:"column:side" json:"side"`
	Price           float64 `gorm:"column:price" json:"price"`
	Quantity        float64 `gorm:"column:quantity" json:"quantity"`
	QuoteQuantity   float64 `gorm:"column:quote_quantity" json:"quote_quantity"`
	Commission      float64 `gorm:"column:commission" json:"commission"`
	CommissionAsset string  `gorm:"column:commission_asset" json:"commission_asset"`
	RealizedPnL     float64 `gorm:"column:realized_pnl" json:"realized_pnl"`
	Maker           bool    `gorm:"column:maker" json:"maker"`
	CreatedAt       int64   `gorm:"column:created_at;serializer:unixms;autoCreateTime:false" json:"created_at"`
}

const orderColumns = `o.id, o.source, COALESCE(o.source_id, '') AS trader_id, o.account_id::text AS account_id, i.venue AS venue,
	COALESCE(o.venue_order_id, '') AS venue_order_id, COALESCE(o.client_id, '') AS client_id,
	i.symbol, upper(o.side) AS side, upper(COALESCE(o.position_side, '')) AS position_side, o.role, o.type,
	o.quantity, COALESCE(o.price, 0) AS price, COALESCE(o.stop_price, 0) AS stop_price, o.status,
	o.filled_quantity, COALESCE(o.avg_fill_price, 0) AS avg_fill_price,
	COALESCE(f.commission, 0) AS commission, COALESCE(f.commission_asset, '') AS commission_asset,
	COALESCE(o.leverage, 0) AS leverage, o.reduce_only,
	CASE WHEN o.role IN ('entry', 'grid') THEN 'open_' ELSE 'close_' END ||
		COALESCE(o.position_side, CASE WHEN (o.role IN ('entry', 'grid')) = (o.side = 'buy') THEN 'long' ELSE 'short' END) AS order_action,
	o.created_at, o.updated_at, o.closed_at, COALESCE(o.error, '') AS error`

const fillColumns = `f.id, o.source, COALESCE(o.source_id, '') AS trader_id, f.account_id::text AS account_id, i.venue AS venue,
	f.order_id, COALESCE(o.venue_order_id, '') AS venue_order_id, f.venue_trade_id, i.symbol, upper(f.side) AS side,
	f.price, f.quantity, f.quote_quantity, f.commission, f.commission_asset, COALESCE(f.realized_pnl, 0) AS realized_pnl,
	f.maker, f.executed_at AS created_at`

// OrderStore reads orders and fills.
type OrderStore struct {
	db *gorm.DB
}

// NewOrderStore creates order storage instance
func NewOrderStore(db *gorm.DB) *OrderStore {
	return &OrderStore{db: db}
}

func (s *OrderStore) orders() *gorm.DB {
	return s.db.Table("account.orders o").Select(orderColumns).
		Joins("JOIN market.instruments i ON i.id = o.instrument_id").
		Joins(`LEFT JOIN LATERAL (SELECT SUM(commission) AS commission, MIN(commission_asset) AS commission_asset
			FROM account.fills WHERE order_id = o.id) f ON true`)
}

func (s *OrderStore) fills() *gorm.DB {
	return s.db.Table("account.fills f").Select(fillColumns).
		Joins("JOIN account.orders o ON o.id = f.order_id").
		Joins("JOIN market.instruments i ON i.id = f.instrument_id")
}

// GetTraderOrders gets trader's order list
func (s *OrderStore) GetTraderOrders(traderID string, limit int) ([]*TraderOrder, error) {
	return s.GetTraderOrdersFiltered(traderID, "", "", limit)
}

// UpdatedSince returns the orders updated at or after a time, oldest change first.
func (s *OrderStore) UpdatedSince(sinceMs int64, limit int) ([]*TraderOrder, error) {
	var orders []*TraderOrder
	err := s.orders().Where("o.updated_at >= ?", time.UnixMilli(sinceMs).UTC()).Order("o.updated_at ASC").Limit(limit).Find(&orders).Error
	return orders, err
}

// FillCursorAt returns where to read on from with FillsAfter to see the fills from a time
// on.
func (s *OrderStore) FillCursorAt(at time.Time) (int64, error) {
	return database.CursorAt(s.db, "account.fills", "executed_at", at)
}

// FillsAfter returns the fills past an ID and no older than a time, oldest first.
func (s *OrderStore) FillsAfter(id, sinceMs int64, limit int) ([]*TraderFill, error) {
	var fills []*TraderFill
	err := s.fills().Where("f.id > ? AND f.executed_at >= ?", id, time.UnixMilli(sinceMs).UTC()).Order("f.id ASC").Limit(limit).Find(&fills).Error
	return fills, err
}

// RejectedSince returns orders the exchange rejected after sinceMs, oldest first.
func (s *OrderStore) RejectedSince(traderIDs []string, sinceMs int64, limit int) ([]*TraderOrder, error) {
	if len(traderIDs) == 0 {
		return nil, nil
	}
	var orders []*TraderOrder
	err := s.orders().Where("o.status = ? AND o.source = ? AND o.source_id IN ? AND o.updated_at >= ?", "REJECTED", SourceTrader, traderIDs, afterMilli(sinceMs)).
		Order("o.updated_at ASC").
		Limit(limit).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query rejected orders: %w", err)
	}
	return orders, nil
}

// GetTraderOrdersFiltered gets trader's order list with optional symbol and status filters
func (s *OrderStore) GetTraderOrdersFiltered(traderID string, symbol string, status string, limit int) ([]*TraderOrder, error) {
	var orders []*TraderOrder
	query := s.orders().Where("o.source = ? AND o.source_id = ?", SourceTrader, traderID)
	if symbol != "" {
		query = query.Where("i.symbol = ?", symbol)
	}
	if status != "" {
		query = query.Where("o.status = ?", status)
	}
	err := query.Order("o.created_at DESC").Limit(limit).Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query orders: %w", err)
	}
	return orders, nil
}

// GetAccountOrders gets an account's orders, whoever placed them, with optional symbol and
// status filters.
func (s *OrderStore) GetAccountOrders(accountID, symbol, status string, limit int) ([]*TraderOrder, error) {
	var orders []*TraderOrder
	query := s.orders().Where("o.account_id = ?", accountID)
	if symbol != "" {
		query = query.Where("i.symbol = ?", symbol)
	}
	if status != "" {
		query = query.Where("o.status = ?", status)
	}
	err := query.Order("o.created_at DESC").Limit(limit).Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query orders: %w", err)
	}
	return orders, nil
}

// GetAccountFills gets an account's fills, newest first, with an optional symbol filter.
func (s *OrderStore) GetAccountFills(accountID, symbol string, limit int) ([]*TraderFill, error) {
	var fills []*TraderFill
	query := s.fills().Where("f.account_id = ?", accountID)
	if symbol != "" {
		query = query.Where("i.symbol = ?", symbol)
	}
	if err := query.Order("f.executed_at DESC").Limit(limit).Find(&fills).Error; err != nil {
		return nil, fmt.Errorf("failed to query fills: %w", err)
	}
	return fills, nil
}

// GetOrderFills gets fill records for a specific order. The traderID arg
// scopes the join so a caller cannot read fills for an order that does not
// belong to their trader (IDOR boundary). Pass an empty traderID only from
// trusted internal callers that have already verified ownership.
func (s *OrderStore) GetOrderFills(traderID string, orderID int64) ([]*TraderFill, error) {
	query := s.fills().Where("f.order_id = ?", orderID)
	if traderID != "" {
		query = query.Where("o.source = ? AND o.source_id = ?", SourceTrader, traderID)
	}
	var fills []*TraderFill
	if err := query.Order("f.executed_at ASC").Find(&fills).Error; err != nil {
		return nil, fmt.Errorf("failed to query fills: %w", err)
	}
	return fills, nil
}
