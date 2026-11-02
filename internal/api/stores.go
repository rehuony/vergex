package api

import (
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/futures"
	"vergex/internal/telegram"
	"vergex/internal/trader"
	"vergex/internal/user"

	"gorm.io/gorm"
)

// Stores is every module's store the API serves from.
type Stores struct {
	DB            *gorm.DB
	Users         *user.Store
	Models        *agent.ModelStore
	Calls         *agent.CallStore
	Conversations *agent.ConversationStore
	Accounts      *account.Store
	Ledger        *account.Ledger
	Orders        *account.OrderStore
	Positions     *account.PositionStore
	Traders       *trader.Store
	Decisions     *trader.DecisionStore
	Equity        *trader.EquityStore
	AccountCloses *futures.AccountCloseStore
	Telegram      telegram.ConfigStore
	Visitors      *telegram.VisitorStore
}

// NewStores opens them on a migrated connection.
func NewStores(db *gorm.DB) *Stores {
	return &Stores{
		DB:            db,
		Users:         user.NewStore(db),
		Models:        agent.NewModelStore(db),
		Calls:         agent.NewCallStore(db),
		Conversations: agent.NewConversationStore(db),
		Accounts:      account.NewStore(db),
		Ledger:        account.NewLedger(db),
		Orders:        account.NewOrderStore(db),
		Positions:     account.NewPositionStore(db),
		Traders:       trader.NewStore(db),
		Decisions:     trader.NewDecisionStore(db),
		Equity:        trader.NewEquityStore(db),
		AccountCloses: futures.NewAccountCloseStore(db),
		Telegram:      telegram.NewConfigStore(db),
		Visitors:      telegram.NewVisitorStore(db),
	}
}
