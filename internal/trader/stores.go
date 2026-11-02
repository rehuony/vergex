package trader

import (
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/user"

	"gorm.io/gorm"
)

// Stores is what traders read and write, their own tables and those of the modules
// they rest on.
type Stores struct {
	DB            *gorm.DB
	Traders       *Store
	Decisions     *DecisionStore
	Equity        *EquityStore
	Peaks         *PeakStore
	Accounts      *account.Store
	Ledger        *account.Ledger
	Orders        *account.OrderStore
	Positions     *account.PositionStore
	Models        *agent.ModelStore
	Conversations *agent.ConversationStore
	Users         *user.Store
}

// NewStores opens them on a migrated connection.
func NewStores(db *gorm.DB) *Stores {
	return &Stores{
		DB:            db,
		Traders:       NewStore(db),
		Decisions:     NewDecisionStore(db),
		Equity:        NewEquityStore(db),
		Peaks:         NewPeakStore(db),
		Accounts:      account.NewStore(db),
		Ledger:        account.NewLedger(db),
		Orders:        account.NewOrderStore(db),
		Positions:     account.NewPositionStore(db),
		Models:        agent.NewModelStore(db),
		Conversations: agent.NewConversationStore(db),
		Users:         user.NewStore(db),
	}
}
