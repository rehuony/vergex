package telegram

import (
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/trader"
	"vergex/internal/user"

	"gorm.io/gorm"
)

// Stores is what the bot and its alerts read and write.
type Stores struct {
	DB            *gorm.DB
	Telegram      ConfigStore
	Users         *user.Store
	Traders       *trader.Store
	Decisions     *trader.DecisionStore
	Orders        *account.OrderStore
	Positions     *account.PositionStore
	Accounts      *account.Store
	Models        *agent.ModelStore
	Conversations *agent.ConversationStore
	Calls         *agent.CallStore
}

// NewStores opens them on a migrated connection.
func NewStores(db *gorm.DB) *Stores {
	return &Stores{
		DB:            db,
		Telegram:      NewConfigStore(db),
		Users:         user.NewStore(db),
		Traders:       trader.NewStore(db),
		Decisions:     trader.NewDecisionStore(db),
		Orders:        account.NewOrderStore(db),
		Positions:     account.NewPositionStore(db),
		Accounts:      account.NewStore(db),
		Models:        agent.NewModelStore(db),
		Conversations: agent.NewConversationStore(db),
		Calls:         agent.NewCallStore(db),
	}
}
