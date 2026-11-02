package account

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"vergex/internal/database"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The sources an order or position can belong to.
const (
	SourceTrader   = "trader"
	SourceFutures  = "futures"
	SourceManual   = "manual"
	SourceExternal = "external"
)

// The roles of an order.
const (
	RoleEntry = "entry"
	RoleClose = "close"
	RoleStop  = "sl"
	RoleTake  = "tp"
	RoleGrid  = "grid"
)

// The sides of an order, and of a position.
const (
	SideBuy   = "buy"
	SideSell  = "sell"
	SideLong  = "long"
	SideShort = "short"
)

// The statuses of an order that trading sets.
const (
	OrderNew             = "NEW"
	OrderPartiallyFilled = "PARTIALLY_FILLED"
	OrderFilled          = "FILLED"
	OrderCanceled        = "CANCELED"
	OrderExpired         = "EXPIRED"
	OrderRejected        = "REJECTED"
)

// The statuses of a position.
const (
	PositionOpen   = "open"
	PositionClosed = "closed"
)

// LedgerOrder is one real order.
type LedgerOrder struct {
	ID                  int64               `gorm:"primaryKey;autoIncrement"`
	AccountID           string              `gorm:"column:account_id"`
	InstrumentID        int64               `gorm:"column:instrument_id"`
	Source              string              `gorm:"column:source"`
	SourceID            string              `gorm:"column:source_id;serializer:nulltext"`
	Seq                 *int                `gorm:"column:seq"`
	Role                string              `gorm:"column:role"`
	ClientOrderID       string              `gorm:"column:client_order_id;serializer:nulltext"`
	VenueClientID       string              `gorm:"column:venue_client_id;serializer:nulltext"`
	ExchangeOrderID     string              `gorm:"column:exchange_order_id;serializer:nulltext"`
	TriggeredOrderID    string              `gorm:"column:triggered_order_id;serializer:nulltext"`
	ParentClientOrderID string              `gorm:"column:parent_client_order_id;serializer:nulltext"`
	Side                string              `gorm:"column:side"`
	PositionSide        string              `gorm:"column:position_side;serializer:nulltext"`
	Type                string              `gorm:"column:type"`
	Status              string              `gorm:"column:status"`
	Quantity            decimal.Decimal     `gorm:"column:quantity"`
	Price               decimal.NullDecimal `gorm:"column:price"`
	StopPrice           decimal.NullDecimal `gorm:"column:stop_price"`
	FilledQuantity      decimal.Decimal     `gorm:"column:filled_quantity"`
	AvgFillPrice        decimal.NullDecimal `gorm:"column:avg_fill_price"`
	ReduceOnly          bool                `gorm:"column:reduce_only"`
	PostOnly            bool                `gorm:"column:post_only"`
	Leverage            *int                `gorm:"column:leverage"`
	MarginMode          string              `gorm:"column:margin_mode;serializer:nulltext"`
	Error               string              `gorm:"column:error;serializer:nulltext"`
	CreatedAt           time.Time           `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt           time.Time           `gorm:"column:updated_at;autoUpdateTime:false"`
	ClosedAt            *time.Time          `gorm:"column:closed_at"`
}

func (LedgerOrder) TableName() string { return "account.orders" }

// LedgerFill is what actually traded.
type LedgerFill struct {
	ID                int64               `gorm:"primaryKey;autoIncrement"`
	OrderID           int64               `gorm:"column:order_id"`
	PositionID        *int64              `gorm:"column:position_id"`
	AccountID         string              `gorm:"column:account_id"`
	InstrumentID      int64               `gorm:"column:instrument_id"`
	ExchangeTradeID   string              `gorm:"column:exchange_trade_id"`
	Side              string              `gorm:"column:side"`
	Price             decimal.Decimal     `gorm:"column:price"`
	Quantity          decimal.Decimal     `gorm:"column:quantity"`
	QuoteQuantity     decimal.Decimal     `gorm:"column:quote_quantity"`
	Commission        decimal.Decimal     `gorm:"column:commission"`
	CommissionAsset   string              `gorm:"column:commission_asset"`
	RealizedPnL       decimal.NullDecimal `gorm:"column:realized_pnl"`
	UnmatchedQuantity decimal.Decimal     `gorm:"column:unmatched_quantity"`
	IsMaker           bool                `gorm:"column:is_maker"`
	ExecutedAt        time.Time           `gorm:"column:executed_at"`
	RecordedAt        time.Time           `gorm:"column:recorded_at;autoCreateTime"`
}

func (LedgerFill) TableName() string { return "account.fills" }

// LedgerPosition is a position as worked out from fills.
type LedgerPosition struct {
	ID            int64           `gorm:"primaryKey;autoIncrement"`
	AccountID     string          `gorm:"column:account_id"`
	InstrumentID  int64           `gorm:"column:instrument_id"`
	Source        string          `gorm:"column:source"`
	SourceID      string          `gorm:"column:source_id;serializer:nulltext"`
	Side          string          `gorm:"column:side"`
	Status        string          `gorm:"column:status"`
	EntryQuantity decimal.Decimal `gorm:"column:entry_quantity"`
	Quantity      decimal.Decimal `gorm:"column:quantity"`
	EntryPrice    decimal.Decimal `gorm:"column:entry_price"`
	ExitPrice     decimal.Decimal `gorm:"column:exit_price"`
	RealizedPnL   decimal.Decimal `gorm:"column:realized_pnl"`
	Fee           decimal.Decimal `gorm:"column:fee"`
	Leverage      *int            `gorm:"column:leverage"`
	MarginMode    string          `gorm:"column:margin_mode;serializer:nulltext"`
	OpenedAt      time.Time       `gorm:"column:opened_at"`
	ClosedAt      *time.Time      `gorm:"column:closed_at"`
	CloseReason   string          `gorm:"column:close_reason;serializer:nulltext"`
	UpdatedAt     time.Time       `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (LedgerPosition) TableName() string { return "account.positions" }

// SkippedFill is a trade an exchange reported that cannot be booked.
type SkippedFill struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	AccountID       string    `gorm:"column:account_id"`
	NativeSymbol    string    `gorm:"column:native_symbol"`
	ExchangeTradeID string    `gorm:"column:exchange_trade_id"`
	Detail          string    `gorm:"column:detail"`
	ExecutedAt      time.Time `gorm:"column:executed_at"`
	RecordedAt      time.Time `gorm:"column:recorded_at;autoCreateTime"`
}

func (SkippedFill) TableName() string { return "account.skipped_fills" }

// Owner is what an order or position belongs to.
type Owner struct {
	Source   string
	SourceID string
}

// Trade is one trade as the exchange reported it.
type Trade struct {
	AccountID       string
	InstrumentID    int64
	ExchangeTradeID string
	ExchangeOrderID string
	Side            string // SideBuy or SideSell
	PositionSide    string // SideLong or SideShort; empty in one-way mode
	Price           decimal.Decimal
	Quantity        decimal.Decimal
	QuoteQuantity   decimal.Decimal
	Commission      decimal.Decimal // what it cost; negative for a rebate
	CommissionAsset string
	RealizedPnL     decimal.NullDecimal
	IsMaker         bool
	ExecutedAt      time.Time
}

// LedgerStore keeps the trading ledger: orders, fills and positions.
type Ledger struct {
	db *gorm.DB
}

// NewLedgerStore creates a new LedgerStore.
func NewLedger(db *gorm.DB) *Ledger {
	return &Ledger{db: db}
}

// RecordSkippedFill notes a trade that cannot be booked, and reports whether it is news.
func (s *Ledger) RecordSkippedFill(fill *SkippedFill) (bool, error) {
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(fill)
	return result.RowsAffected != 0, result.Error
}

// ClearSkippedFill forgets a skipped trade that has since been booked.
func (s *Ledger) ClearSkippedFill(accountID, nativeSymbol, tradeID string) error {
	return s.db.Where("account_id = ? AND native_symbol = ? AND exchange_trade_id = ?", accountID, nativeSymbol, tradeID).
		Delete(&SkippedFill{}).Error
}

// SkippedFillsSince returns a user's skipped trades recorded since a time, newest first.
func (s *Ledger) SkippedFillsSince(userID string, since time.Time, limit int) ([]SkippedFill, error) {
	var fills []SkippedFill
	if uuid.Validate(userID) != nil {
		return fills, nil
	}
	err := s.db.Where("recorded_at >= ? AND account_id IN (SELECT id FROM account.accounts WHERE user_id = ?)", since, userID).
		Order("recorded_at DESC, id DESC").Limit(limit).Find(&fills).Error
	return fills, err
}

// LeaseOwner is whoever has reserved the account, or no one.
func (s *Ledger) LeaseOwner(accountID string) (Owner, error) {
	var lease Lease
	err := s.db.Where("account_id = ?", accountID).First(&lease).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Owner{}, nil
	}
	if err != nil {
		return Owner{}, err
	}
	return Owner{Source: lease.Holder, SourceID: lease.HolderID}, nil
}

// OpenSymbols lists the symbols with a position still open on an account.
func (s *Ledger) OpenSymbols(accountID string) ([]string, error) {
	var symbols []string
	err := s.db.Raw(`SELECT DISTINCT i.symbol FROM account.positions p
		JOIN market.instruments i ON i.id = p.instrument_id
		WHERE p.account_id = ? AND p.status = ?`, accountID, PositionOpen).Scan(&symbols).Error
	return symbols, err
}

// LastFillTime returns when the account's latest known trade happened; zero if none.
func (s *Ledger) LastFillTime(accountID string) (time.Time, error) {
	var at sql.NullTime
	err := s.db.Raw(`SELECT MAX(executed_at) FROM account.fills WHERE account_id = ?`, accountID).Scan(&at).Error
	if err != nil || !at.Valid {
		return time.Time{}, err
	}
	return at.Time, nil
}

// lockInstrument makes one writer at a time work on an account's instrument, until
// the transaction ends. A position's numbers depend on the order fills are applied.
func lockInstrument(tx *gorm.DB, accountID string, instrumentID int64) error {
	key := accountID + "/" + strconv.FormatInt(instrumentID, 10)
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error
}

// RecordOrder records an order that was placed. A fill can get here first and leave
// an order of its own, which this then completes, so what the fill worked out stays.
func (s *Ledger) RecordOrder(order *LedgerOrder) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockInstrument(tx, order.AccountID, order.InstrumentID); err != nil {
			return err
		}
		now := time.Now().UTC()
		order.UpdatedAt = now
		existing, err := findExchangeOrder(tx, order.AccountID, order.InstrumentID, order.ExchangeOrderID)
		if err != nil {
			return err
		}
		if existing == nil {
			if order.CreatedAt.IsZero() {
				order.CreatedAt = now
			}
			return tx.Create(order).Error
		}
		order.ID = existing.ID
		updates := map[string]any{
			"source":          order.Source,
			"source_id":       database.NullText(order.SourceID),
			"role":            order.Role,
			"client_order_id": database.NullText(order.ClientOrderID),
			"type":            order.Type,
			"quantity":        order.Quantity,
			"price":           order.Price,
			"stop_price":      order.StopPrice,
			"reduce_only":     order.ReduceOnly,
			"post_only":       order.PostOnly,
			"leverage":        order.Leverage,
			"margin_mode":     database.NullText(order.MarginMode),
			"updated_at":      now,
		}
		if !existing.FilledQuantity.LessThan(order.Quantity) {
			updates["status"] = OrderFilled
		} else {
			updates["status"] = OrderPartiallyFilled
			updates["closed_at"] = nil
		}
		if err := tx.Model(&LedgerOrder{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return err
		}
		// A position opened by this order's fills learns what the order knew.
		if order.Leverage != nil {
			if err := tx.Exec(`UPDATE account.positions SET leverage = ?, margin_mode = ?
				WHERE leverage IS NULL AND id IN (SELECT position_id FROM account.fills WHERE order_id = ?)`,
				*order.Leverage, database.NullText(order.MarginMode), existing.ID).Error; err != nil {
				return err
			}
		}
		order.ID = existing.ID
		return nil
	})
}

func findExchangeOrder(tx *gorm.DB, accountID string, instrumentID int64, exchangeOrderID string) (*LedgerOrder, error) {
	if exchangeOrderID == "" {
		return nil, nil
	}
	var order LedgerOrder
	err := tx.Where("account_id = ? AND instrument_id = ? AND (exchange_order_id = ? OR triggered_order_id = ?)",
		accountID, instrumentID, exchangeOrderID, exchangeOrderID).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// RecordTrade books one trade, all in one transaction: its order, its fill and the
// positions it changes. fallback owns an order nothing here knows of when no position
// claims it. A trade already booked changes nothing, so it is safe to report a trade
// any number of times. It reports whether the trade was new.
func (s *Ledger) RecordTrade(trade Trade, fallback Owner) (bool, error) {
	recorded := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockInstrument(tx, trade.AccountID, trade.InstrumentID); err != nil {
			return err
		}
		var seen int64
		err := tx.Model(&LedgerFill{}).
			Where("account_id = ? AND instrument_id = ? AND exchange_trade_id = ?", trade.AccountID, trade.InstrumentID, trade.ExchangeTradeID).
			Count(&seen).Error
		if err != nil || seen != 0 {
			return err
		}
		booking := tradeBooking{tx: tx, trade: trade}
		if err := booking.plan(); err != nil {
			return err
		}
		order, err := booking.findOrder(fallback)
		if err != nil {
			return err
		}
		fill := LedgerFill{
			OrderID:         order.ID,
			AccountID:       trade.AccountID,
			InstrumentID:    trade.InstrumentID,
			ExchangeTradeID: trade.ExchangeTradeID,
			Side:            trade.Side,
			Price:           trade.Price,
			Quantity:        trade.Quantity,
			QuoteQuantity:   trade.QuoteQuantity,
			Commission:      trade.Commission,
			CommissionAsset: trade.CommissionAsset,
			RealizedPnL:     trade.RealizedPnL,
			IsMaker:         trade.IsMaker,
			ExecutedAt:      trade.ExecutedAt,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&fill)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if err := booking.updateOrder(order); err != nil {
			return err
		}
		if err := booking.updatePositions(order, &fill); err != nil {
			return err
		}
		recorded = true
		return nil
	})
	return recorded, err
}

// WorkingOrder is an order the ledger still has as working on the exchange.
type WorkingOrder struct {
	ID              int64
	Symbol          string
	ExchangeOrderID string
}

// WorkingOrders lists the account's orders still working, placed since a time. Futures
// watches its own.
func (s *Ledger) WorkingOrders(accountID string, since time.Time) ([]WorkingOrder, error) {
	var orders []WorkingOrder
	err := s.db.Raw(`SELECT o.id, i.symbol, o.exchange_order_id FROM account.orders o
		JOIN market.instruments i ON i.id = o.instrument_id
		WHERE o.account_id = ? AND o.source <> ? AND o.exchange_order_id IS NOT NULL AND o.closed_at IS NULL
			AND o.status IN (?, ?) AND o.created_at >= ?
		ORDER BY i.symbol, o.id`, accountID, SourceFutures, OrderNew, OrderPartiallyFilled, since).Scan(&orders).Error
	return orders, err
}

// SettleOrder records how the exchange says an order ended. What actually filled comes
// from its fills, which can arrive before or after this.
func (s *Ledger) SettleOrder(id int64, status string) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "closed_at": now, "updated_at": now}
	return s.db.Model(&LedgerOrder{}).Where("id = ? AND closed_at IS NULL", id).Updates(updates).Error
}

// SettleExchangeOrder is SettleOrder for an order known by its exchange ID. An order the
// ledger has not heard of has nothing to settle.
func (s *Ledger) SettleExchangeOrder(accountID string, instrumentID int64, exchangeOrderID, status string) error {
	order, err := findExchangeOrder(s.db, accountID, instrumentID, exchangeOrderID)
	if err != nil || order == nil {
		return err
	}
	return s.SettleOrder(order.ID, status)
}

// RunResult is what the positions of one source added up to.
type RunResult struct {
	SourceID    string     `gorm:"column:source_id"`
	RealizedPnL float64    `gorm:"column:realized_pnl"`
	Closes      int        `gorm:"column:closes"`
	ClosedAt    *time.Time `gorm:"column:closed_at"`
}

// FuturesResults sums the closed positions of an account's Futures runs, by run.
func (s *Ledger) FuturesResults(accountID string) ([]RunResult, error) {
	var results []RunResult
	err := s.db.Raw(`SELECT source_id, SUM(realized_pnl) AS realized_pnl, COUNT(*) AS closes, MAX(closed_at) AS closed_at
		FROM account.positions
		WHERE account_id = ? AND source = ? AND status = ? AND source_id IS NOT NULL
		GROUP BY source_id`, accountID, SourceFutures, PositionClosed).Scan(&results).Error
	return results, err
}

// tradeBooking is the work of booking one trade.
type tradeBooking struct {
	tx    *gorm.DB
	trade Trade
	// What the trade does, in order. A reversal in one-way mode does both.
	reduceSide string
	reduce     decimal.Decimal
	openSide   string
	open       decimal.Decimal
}

func oppositeSide(side string) string {
	if side == SideLong {
		return SideShort
	}
	return SideLong
}

// reducedBy is the side of position that a trade on this side of the book reduces.
func reducedBy(tradeSide string) string {
	if tradeSide == SideSell {
		return SideLong
	}
	return SideShort
}

// plan works out what the trade does to positions.
func (b *tradeBooking) plan() error {
	trade := b.trade
	reduces := reducedBy(trade.Side)
	if trade.PositionSide != "" {
		// Hedge mode says it outright.
		if trade.PositionSide == reduces {
			b.reduceSide, b.reduce = reduces, trade.Quantity
		} else {
			b.openSide, b.open = trade.PositionSide, trade.Quantity
		}
		return nil
	}
	// One-way mode: the trade reduces what is open against it and opens the rest.
	held, err := b.openQuantity(reduces)
	if err != nil {
		return err
	}
	switch {
	case held.GreaterThanOrEqual(trade.Quantity):
		b.reduceSide, b.reduce = reduces, trade.Quantity
	case held.IsPositive():
		b.reduceSide, b.reduce = reduces, held
		b.openSide, b.open = oppositeSide(reduces), trade.Quantity.Sub(held)
	case trade.RealizedPnL.Valid && !trade.RealizedPnL.Decimal.IsZero():
		// Only a close realizes anything, so what it closed predates what we know.
		b.reduceSide, b.reduce = reduces, trade.Quantity
	default:
		b.openSide, b.open = oppositeSide(reduces), trade.Quantity
	}
	return nil
}

func (b *tradeBooking) openQuantity(side string) (decimal.Decimal, error) {
	positions, err := b.openPositions(side, Owner{})
	if err != nil {
		return decimal.Zero, err
	}
	total := decimal.Zero
	for _, position := range positions {
		total = total.Add(position.Quantity)
	}
	return total, nil
}

// openPositions lists the open positions on a side, the owner's first and then the
// oldest first.
func (b *tradeBooking) openPositions(side string, owner Owner) ([]LedgerPosition, error) {
	var positions []LedgerPosition
	err := b.tx.Where("account_id = ? AND instrument_id = ? AND side = ? AND status = ?", b.trade.AccountID, b.trade.InstrumentID, side, PositionOpen).
		Order(clause.OrderBy{Expression: clause.Expr{
			SQL:                "(source = ? AND source_id IS NOT DISTINCT FROM ?::text) DESC, opened_at, id",
			Vars:               []any{owner.Source, database.NullText(owner.SourceID)},
			WithoutParentheses: true,
		}}).
		Find(&positions).Error
	return positions, err
}

// findOrder finds the trade's order, or records one for an order nothing knew of.
func (b *tradeBooking) findOrder(fallback Owner) (*LedgerOrder, error) {
	trade := b.trade
	order, err := findExchangeOrder(b.tx, trade.AccountID, trade.InstrumentID, trade.ExchangeOrderID)
	if err != nil || order != nil {
		return order, err
	}
	owner := fallback
	if owner.Source == "" {
		owner.Source = SourceExternal
	}
	role := RoleEntry
	if b.reduce.IsPositive() {
		role = RoleClose
		// A close belongs to whoever holds the position, which is how a trader's stop
		// order, which it does not record, finds its way home.
		positions, err := b.openPositions(b.reduceSide, Owner{})
		if err != nil {
			return nil, err
		}
		if len(positions) > 0 {
			owner = Owner{Source: positions[0].Source, SourceID: positions[0].SourceID}
		}
	}
	created := LedgerOrder{
		AccountID:       trade.AccountID,
		InstrumentID:    trade.InstrumentID,
		Source:          owner.Source,
		SourceID:        owner.SourceID,
		Role:            role,
		ExchangeOrderID: trade.ExchangeOrderID,
		Side:            trade.Side,
		PositionSide:    trade.PositionSide,
		Status:          OrderFilled,
		Quantity:        decimal.Zero,
		CreatedAt:       trade.ExecutedAt,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := b.tx.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// updateOrder adds the trade to its order. An order known only through its fills is
// as large as they are.
func (b *tradeBooking) updateOrder(order *LedgerOrder) error {
	trade := b.trade
	filled := order.FilledQuantity.Add(trade.Quantity)
	average := trade.Price
	if order.AvgFillPrice.Valid && order.FilledQuantity.IsPositive() {
		average = order.AvgFillPrice.Decimal.Mul(order.FilledQuantity).Add(trade.Price.Mul(trade.Quantity)).Div(filled)
	}
	updates := map[string]any{
		"filled_quantity": filled,
		"avg_fill_price":  average,
		"updated_at":      time.Now().UTC(),
	}
	quantity := order.Quantity
	if order.Quantity.IsZero() || filled.GreaterThan(order.Quantity) && order.Status == OrderFilled {
		quantity = filled
		updates["quantity"] = filled
	}
	switch {
	case order.ClosedAt != nil && order.Status != OrderPartiallyFilled:
		// The exchange has already said how this ended, and a late fill changes nothing.
	case filled.LessThan(quantity):
		updates["status"] = OrderPartiallyFilled
	default:
		updates["status"] = OrderFilled
		updates["closed_at"] = trade.ExecutedAt
	}
	return b.tx.Model(&LedgerOrder{}).Where("id = ?", order.ID).Updates(updates).Error
}

// updatePositions applies the trade to positions, and points the fill at the first
// one it changed.
func (b *tradeBooking) updatePositions(order *LedgerOrder, fill *LedgerFill) error {
	owner := Owner{Source: order.Source, SourceID: order.SourceID}
	var first *int64
	unmatched := decimal.Zero
	if b.reduce.IsPositive() {
		positions, err := b.openPositions(b.reduceSide, owner)
		if err != nil {
			return err
		}
		left := b.reduce
		for i := range positions {
			if !left.IsPositive() {
				break
			}
			take := decimal.Min(left, positions[i].Quantity)
			if err := b.tx.Model(&LedgerPosition{}).Where("id = ?", positions[i].ID).Updates(reduction(&positions[i], take, *fill, closeReason(order.Role))).Error; err != nil {
				return err
			}
			if first == nil {
				id := positions[i].ID
				first = &id
			}
			left = left.Sub(take)
		}
		unmatched = left
	}
	if b.open.IsPositive() {
		id, err := b.addToPosition(order, owner)
		if err != nil {
			return err
		}
		if first == nil {
			first = &id
		}
	}
	return b.tx.Model(&LedgerFill{}).Where("id = ?", fill.ID).Updates(map[string]any{"position_id": first, "unmatched_quantity": unmatched}).Error
}

func closeReason(role string) string {
	switch role {
	case RoleStop:
		return "stop_loss"
	case RoleTake:
		return "take_profit"
	default:
		return "close"
	}
}

// dollarAssets are the fee assets that add up with a position's own numbers.
var dollarAssets = map[string]bool{"USDT": true, "USDC": true, "FDUSD": true, "BUSD": true, "USD": true}

// feeShare is the part of a fill's fee that a position takes for this much of it.
func feeShare(fill LedgerFill, quantity decimal.Decimal) decimal.Decimal {
	if !dollarAssets[strings.ToUpper(fill.CommissionAsset)] || !fill.Quantity.IsPositive() {
		return decimal.Zero
	}
	return fill.Commission.Mul(quantity).Div(fill.Quantity)
}

// reduction is what taking quantity out of a position changes. The exchange's realized
// P/L wins; it only falls back to working it out when the exchange did not report one.
func reduction(position *LedgerPosition, quantity decimal.Decimal, fill LedgerFill, reason string) map[string]any {
	closedBefore := position.EntryQuantity.Sub(position.Quantity)
	closedAfter := closedBefore.Add(quantity)
	exit := position.ExitPrice.Mul(closedBefore).Add(fill.Price.Mul(quantity)).Div(closedAfter)

	var realized decimal.Decimal
	switch {
	case fill.RealizedPnL.Valid && fill.Quantity.IsPositive():
		realized = fill.RealizedPnL.Decimal.Mul(quantity).Div(fill.Quantity)
	case position.Side == SideLong:
		realized = fill.Price.Sub(position.EntryPrice).Mul(quantity)
	default:
		realized = position.EntryPrice.Sub(fill.Price).Mul(quantity)
	}

	left := position.Quantity.Sub(quantity)
	updates := map[string]any{
		"quantity":     left,
		"exit_price":   exit,
		"realized_pnl": position.RealizedPnL.Add(realized),
		"fee":          position.Fee.Add(feeShare(fill, quantity)),
		"updated_at":   time.Now().UTC(),
	}
	if !left.IsPositive() {
		updates["status"] = PositionClosed
		updates["closed_at"] = fill.ExecutedAt
		updates["close_reason"] = reason
	}
	position.Quantity = left
	position.ExitPrice = exit
	position.RealizedPnL = updates["realized_pnl"].(decimal.Decimal)
	position.Fee = updates["fee"].(decimal.Decimal)
	return updates
}

// addToPosition adds the opening part of the trade to the owner's open position,
// starting one if it has none, then lets it take any close that was waiting for it.
func (b *tradeBooking) addToPosition(order *LedgerOrder, owner Owner) (int64, error) {
	trade := b.trade
	fill := LedgerFill{Quantity: trade.Quantity, Commission: trade.Commission, CommissionAsset: trade.CommissionAsset}
	fee := feeShare(fill, b.open)

	var position LedgerPosition
	err := b.tx.Where("account_id = ? AND instrument_id = ? AND source = ? AND source_id IS NOT DISTINCT FROM ?::text AND side = ? AND status = ?",
		trade.AccountID, trade.InstrumentID, owner.Source, database.NullText(owner.SourceID), b.openSide, PositionOpen).First(&position).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		position = LedgerPosition{
			AccountID:     trade.AccountID,
			InstrumentID:  trade.InstrumentID,
			Source:        owner.Source,
			SourceID:      owner.SourceID,
			Side:          b.openSide,
			Status:        PositionOpen,
			EntryQuantity: b.open,
			Quantity:      b.open,
			EntryPrice:    trade.Price,
			Fee:           fee,
			Leverage:      order.Leverage,
			MarginMode:    order.MarginMode,
			OpenedAt:      trade.ExecutedAt,
			UpdatedAt:     time.Now().UTC(),
		}
		if err := b.tx.Create(&position).Error; err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	default:
		total := position.Quantity.Add(b.open)
		position.EntryPrice = position.EntryPrice.Mul(position.Quantity).Add(trade.Price.Mul(b.open)).Div(total)
		position.EntryQuantity = position.EntryQuantity.Add(b.open)
		position.Quantity = total
		position.Fee = position.Fee.Add(fee)
		updates := map[string]any{
			"entry_quantity": position.EntryQuantity,
			"quantity":       total,
			"entry_price":    position.EntryPrice,
			"fee":            position.Fee,
			"updated_at":     time.Now().UTC(),
		}
		if trade.ExecutedAt.Before(position.OpenedAt) {
			updates["opened_at"] = trade.ExecutedAt
		}
		if err := b.tx.Model(&LedgerPosition{}).Where("id = ?", position.ID).Updates(updates).Error; err != nil {
			return 0, err
		}
	}
	return position.ID, b.takeWaitingCloses(&position)
}

// takeWaitingCloses applies closes that arrived before this open did. Only closes at
// or after the open count: an older one closed something from before we were watching.
func (b *tradeBooking) takeWaitingCloses(position *LedgerPosition) error {
	closing := SideSell
	if position.Side == SideShort {
		closing = SideBuy
	}
	var waiting []LedgerFill
	err := b.tx.Where("account_id = ? AND instrument_id = ? AND side = ? AND unmatched_quantity > 0 AND executed_at >= ?",
		position.AccountID, position.InstrumentID, closing, b.trade.ExecutedAt).Order("executed_at, id").Find(&waiting).Error
	if err != nil {
		return err
	}
	for _, fill := range waiting {
		if !position.Quantity.IsPositive() {
			return nil
		}
		take := decimal.Min(fill.UnmatchedQuantity, position.Quantity)
		if err := b.tx.Model(&LedgerPosition{}).Where("id = ?", position.ID).Updates(reduction(position, take, fill, "close")).Error; err != nil {
			return err
		}
		updates := map[string]any{"unmatched_quantity": fill.UnmatchedQuantity.Sub(take)}
		if fill.PositionID == nil {
			updates["position_id"] = position.ID
		}
		if err := b.tx.Model(&LedgerFill{}).Where("id = ?", fill.ID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
