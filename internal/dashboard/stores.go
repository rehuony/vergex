package dashboard

import (
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/trader"
	"vergex/internal/user"

	"gorm.io/gorm"
)

// Stores is what the feed reads.
type Stores struct {
	Users     *user.Store
	Traders   *trader.Store
	Decisions *trader.DecisionStore
	Accounts  *account.Store
	Orders    *account.OrderStore
	Positions *account.PositionStore
	Models    *agent.ModelStore
	Calls     *agent.CallStore
}

// NewStores opens them on a migrated connection.
func NewStores(db *gorm.DB) *Stores {
	return &Stores{
		Users:     user.NewStore(db),
		Traders:   trader.NewStore(db),
		Decisions: trader.NewDecisionStore(db),
		Accounts:  account.NewStore(db),
		Orders:    account.NewOrderStore(db),
		Positions: account.NewPositionStore(db),
		Models:    agent.NewModelStore(db),
		Calls:     agent.NewCallStore(db),
	}
}
