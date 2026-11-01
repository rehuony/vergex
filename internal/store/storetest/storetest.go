// Package storetest opens a fully migrated Store for tests.
package storetest

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/database/testdb"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// UserID is a valid account ID for tests that need one.
const UserID = "00000000-0000-4000-8000-000000000001"

// OtherUserID is a second valid account ID, for tests of ownership.
const OtherUserID = "00000000-0000-4000-8000-000000000002"

// New returns a Store with all tables and default data, on a database of its own.
func New(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.NewMigrated(testdb.Open(t))
	if err != nil {
		t.Fatalf("prepare test store: %v", err)
	}
	return st
}

// Account creates the account with UserID, for tests of things that belong to one.
func Account(t testing.TB, st *store.Store) string {
	t.Helper()
	if err := st.User().Create(&store.User{ID: UserID, Email: "owner@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return UserID
}

// OtherAccount creates the account with OtherUserID.
func OtherAccount(t testing.TB, st *store.Store) string {
	t.Helper()
	if err := st.User().Create(&store.User{ID: OtherUserID, Email: "other@example.com", PasswordHash: "x"}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return OtherUserID
}

// AddTrader creates a trader, filling in what a test leaves out: its ID, an exchange
// account, a model, a strategy, and its user, which is the one with UserID unless given.
func AddTrader(t testing.TB, st *store.Store, trader *store.Trader) *store.Trader {
	t.Helper()
	if trader.UserID == "" {
		trader.UserID = UserID
	}
	if _, err := st.User().GetByID(trader.UserID); err != nil {
		if err := st.User().Create(&store.User{ID: trader.UserID, Email: trader.UserID + "@example.com", PasswordHash: "x"}); err != nil {
			t.Fatalf("create account: %v", err)
		}
	}
	if trader.ID == "" {
		trader.ID = uuid.NewString()
	}
	if trader.ExchangeID == "" {
		account, err := st.Exchange().Create(trader.UserID, "binance", trader.Name, uuid.NewString(), "secret", "")
		if err != nil {
			t.Fatalf("create exchange account: %v", err)
		}
		trader.ExchangeID = account
	}
	if trader.AgentModelID == "" {
		if err := st.AgentModel().Save(trader.UserID, "deepseek", "DeepSeek", true, "model-key", "", ""); err != nil {
			t.Fatalf("save model: %v", err)
		}
		model, err := st.AgentModel().Find(trader.UserID, "deepseek")
		if err != nil {
			t.Fatalf("find model: %v", err)
		}
		trader.AgentModelID = model.ID
	}
	if trader.StrategyID == "" {
		config, err := json.Marshal(store.GetDefaultStrategyConfig("en"))
		if err != nil {
			t.Fatal(err)
		}
		strategy := &store.Strategy{ID: uuid.NewString(), UserID: trader.UserID, Name: trader.Name, Config: string(config)}
		if err := st.Strategy().Create(strategy); err != nil {
			t.Fatalf("create strategy: %v", err)
		}
		trader.StrategyID = strategy.ID
	}
	if trader.InitialBalance == 0 {
		trader.InitialBalance = 1000
	}
	if err := st.Trader().Create(trader); err != nil {
		t.Fatalf("create trader: %v", err)
	}
	return trader
}

// Ledger books trades for tests of what reads the ledger. It uses the real bookkeeping,
// so what it leaves is what trading would.
type Ledger struct {
	t       testing.TB
	st      *store.Store
	Account string
	// Leverage, when set, is recorded on the order of each open.
	Leverage int
	next     int
}

// NewLedger creates an exchange account for the user with UserID, creating the user if
// a test has not.
func NewLedger(t testing.TB, st *store.Store) *Ledger {
	t.Helper()
	if _, err := st.User().GetByID(UserID); err != nil {
		Account(t, st)
	}
	account, err := st.Exchange().Create(UserID, "binance", "Ledger", uuid.NewString(), "ledger-secret", "")
	if err != nil {
		t.Fatalf("create exchange account: %v", err)
	}
	return &Ledger{t: t, st: st, Account: account}
}

// Open books a trader opening a position.
func (l *Ledger) Open(traderID, symbol, side string, quantity, price float64, at time.Time) {
	l.t.Helper()
	tradeSide := store.SideBuy
	if side == store.SideShort {
		tradeSide = store.SideSell
	}
	trade := l.trade(symbol, tradeSide, side, quantity, price, at)
	if l.Leverage > 0 {
		leverage := l.Leverage
		err := l.st.Ledger().RecordOrder(&store.LedgerOrder{
			AccountID: l.Account, InstrumentID: trade.InstrumentID, Source: store.SourceTrader, SourceID: traderID, Role: store.RoleEntry,
			ExchangeOrderID: trade.ExchangeOrderID, Side: tradeSide, PositionSide: side, Type: "MARKET", Status: store.OrderNew,
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
	tradeSide := store.SideSell
	if side == store.SideShort {
		tradeSide = store.SideBuy
	}
	trade := l.trade(symbol, tradeSide, side, quantity, price, at)
	trade.RealizedPnL = decimal.NewNullDecimal(decimal.NewFromFloat(realized))
	l.book(trade, traderID)
}

func (l *Ledger) trade(symbol, side, positionSide string, quantity, price float64, at time.Time) store.Trade {
	l.t.Helper()
	instrumentID, err := instrument.Ensure(l.st.GormDB(), "binance", instrument.KindPerpetual, symbol, symbol)
	if err != nil {
		l.t.Fatalf("instrument: %v", err)
	}
	l.next++
	q, p := decimal.NewFromFloat(quantity), decimal.NewFromFloat(price)
	return store.Trade{
		AccountID: l.Account, InstrumentID: instrumentID,
		ExchangeTradeID: fmt.Sprintf("%s-trade-%d", l.Account[:8], l.next), ExchangeOrderID: fmt.Sprintf("%s-order-%d", l.Account[:8], l.next),
		Side: side, PositionSide: positionSide, Price: p, Quantity: q, QuoteQuantity: p.Mul(q), CommissionAsset: "USDT", ExecutedAt: at,
	}
}

func (l *Ledger) book(trade store.Trade, traderID string) {
	l.t.Helper()
	if _, err := l.st.Ledger().RecordTrade(trade, store.Owner{Source: store.SourceTrader, SourceID: traderID}); err != nil {
		l.t.Fatalf("record trade: %v", err)
	}
}
