// Package accounttest books trades for tests of what reads the ledger.
package accounttest

import (
	"fmt"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Ledger books trades for tests of what reads the ledger. It uses the real bookkeeping,
// so what it leaves is what trading would.
type Ledger struct {
	t       testing.TB
	ledger  *account.Ledger
	db      *gorm.DB
	Account string
	// Leverage, when set, is recorded on the order of each open.
	Leverage int
	next     int
}

// NewLedger creates an exchange account for the user with UserID, creating the user if
// a test has not.
func NewLedger(t testing.TB, db *gorm.DB) *Ledger {
	t.Helper()
	testdb.AddUser(t, db)
	id, err := account.NewStore(db).Create(testdb.UserID, "binance", "Ledger", uuid.NewString(), "ledger-secret", "", false)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return &Ledger{t: t, ledger: account.NewLedger(db), db: db, Account: id}
}

// Open books a trader opening a position.
func (l *Ledger) Open(traderID, symbol, side string, quantity, price float64, at time.Time) {
	l.t.Helper()
	tradeSide := account.SideBuy
	if side == account.SideShort {
		tradeSide = account.SideSell
	}
	trade := l.trade(symbol, tradeSide, side, quantity, price, at)
	if l.Leverage > 0 {
		leverage := l.Leverage
		err := l.ledger.RecordOrder(&account.LedgerOrder{
			AccountID: l.Account, InstrumentID: trade.InstrumentID, Source: account.SourceTrader, SourceID: traderID, Role: account.RoleEntry,
			ExchangeOrderID: trade.ExchangeOrderID, Side: tradeSide, PositionSide: side, Type: "MARKET", Status: account.OrderNew,
			Quantity: trade.Quantity, Leverage: &leverage, CreatedAt: at,
		})
		if err != nil {
			l.t.Fatalf("record order: %v", err)
		}
	}
	l.book(trade, traderID)
}

// Close books a trader closing a position, realizing what the exchange reports.
func (l *Ledger) Close(traderID, symbol, side string, quantity, price, realized float64, at time.Time) {
	l.t.Helper()
	tradeSide := account.SideSell
	if side == account.SideShort {
		tradeSide = account.SideBuy
	}
	trade := l.trade(symbol, tradeSide, side, quantity, price, at)
	trade.RealizedPnL = decimal.NewNullDecimal(decimal.NewFromFloat(realized))
	l.book(trade, traderID)
}

func (l *Ledger) trade(symbol, side, positionSide string, quantity, price float64, at time.Time) account.Trade {
	l.t.Helper()
	instrumentID, err := instrument.Ensure(l.db, "binance", instrument.KindPerpetual, symbol, symbol)
	if err != nil {
		l.t.Fatalf("instrument: %v", err)
	}
	l.next++
	q, p := decimal.NewFromFloat(quantity), decimal.NewFromFloat(price)
	return account.Trade{
		AccountID: l.Account, InstrumentID: instrumentID,
		ExchangeTradeID: fmt.Sprintf("%s-trade-%d", l.Account[:8], l.next), ExchangeOrderID: fmt.Sprintf("%s-order-%d", l.Account[:8], l.next),
		Side: side, PositionSide: positionSide, Price: p, Quantity: q, QuoteQuantity: p.Mul(q), CommissionAsset: "USDT", ExecutedAt: at,
	}
}

func (l *Ledger) book(trade account.Trade, traderID string) {
	l.t.Helper()
	if _, err := l.ledger.RecordTrade(trade, account.Owner{Source: account.SourceTrader, SourceID: traderID}); err != nil {
		l.t.Fatalf("record trade: %v", err)
	}
}
