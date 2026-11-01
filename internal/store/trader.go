package store

import (
	"fmt"
	"time"
	"vergex/internal/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The margin modes a trader can use, in the words the ledger uses.
const (
	MarginCross    = "cross"
	MarginIsolated = "isolated"
)

// MarginModeOf names the margin mode of the cross-margin flag the API and traders use.
func MarginModeOf(isCross bool) string {
	if isCross {
		return MarginCross
	}
	return MarginIsolated
}

// TraderStore trader storage
type TraderStore struct {
	db *gorm.DB
}

// NewTraderStore creates a new trader store
func NewTraderStore(db *gorm.DB) *TraderStore {
	return &TraderStore{db: db}
}

// Trader trader configuration
type Trader struct {
	ID                  string    `gorm:"primaryKey" json:"id"`
	UserID              string    `gorm:"column:user_id" json:"user_id"`
	Name                string    `gorm:"column:name" json:"name"`
	AgentModelID        string    `gorm:"column:agent_model_id" json:"agent_model_id"`
	ExchangeID          string    `gorm:"column:account_id" json:"exchange_id"`
	StrategyID          string    `gorm:"column:strategy_id" json:"strategy_id"`
	InitialBalance      float64   `gorm:"column:initial_balance" json:"initial_balance"`
	ScanIntervalMinutes int       `gorm:"column:scan_interval_minutes;default:15" json:"scan_interval_minutes"`
	IsRunning           bool      `gorm:"column:is_running" json:"is_running"`
	MarginMode          string    `gorm:"column:margin_mode;default:cross" json:"margin_mode"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for Trader
func (Trader) TableName() string {
	return "trader.traders"
}

// IsCrossMargin reports whether the trader uses cross margin.
func (t *Trader) IsCrossMargin() bool { return t.MarginMode != MarginIsolated }

// TraderFullConfig trader full configuration (includes model, exchange and strategy)
type TraderFullConfig struct {
	Trader     *Trader
	AgentModel *AgentModel
	Exchange   *Exchange
	Strategy   *Strategy
}

// inUse is the traders that have not been deleted. Nothing below reads the others:
// deleting one archives it and lets go of its model, account and strategy.
func (s *TraderStore) inUse() *gorm.DB {
	return s.db.Where("archived_at IS NULL")
}

// Create creates trader
func (s *TraderStore) Create(trader *Trader) error {
	return s.db.Create(trader).Error
}

// List gets user's trader list
func (s *TraderStore) List(userID string) ([]*Trader, error) {
	var traders []*Trader
	err := s.inUse().Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// UpdateStatus updates trader running status
func (s *TraderStore) UpdateStatus(userID, id string, isRunning bool) error {
	if uuid.Validate(id) != nil {
		return gorm.ErrRecordNotFound
	}
	return s.inUse().Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_running", isRunning).Error
}

// Update updates trader configuration
func (s *TraderStore) Update(trader *Trader) error {
	if uuid.Validate(trader.ID) != nil {
		return gorm.ErrRecordNotFound
	}
	logger.Debugf("TraderStore.Update: ID=%s, Name=%s, AgentModelID=%s, StrategyID=%s",
		trader.ID, trader.Name, trader.AgentModelID, trader.StrategyID)

	updates := map[string]interface{}{
		"name":           trader.Name,
		"agent_model_id": trader.AgentModelID,
		"account_id":     trader.ExchangeID,
		"strategy_id":    trader.StrategyID,
		"margin_mode":    trader.MarginMode,
	}

	// Only update these if > 0
	if trader.InitialBalance > 0 {
		updates["initial_balance"] = trader.InitialBalance
	}
	if trader.ScanIntervalMinutes > 0 {
		updates["scan_interval_minutes"] = trader.ScanIntervalMinutes
	}

	return s.inUse().Model(&Trader{}).
		Where("id = ? AND user_id = ?", trader.ID, trader.UserID).
		Updates(updates).Error
}

// UpdateInitialBalance updates initial balance
func (s *TraderStore) UpdateInitialBalance(userID, id string, newBalance float64) error {
	if uuid.Validate(id) != nil {
		return gorm.ErrRecordNotFound
	}
	return s.inUse().Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("initial_balance", newBalance).Error
}

// Archive deletes a trader as far as anything can see, and keeps its cycles, equity
// and ledger entries. It lets go of its model, account and strategy, so they can go too.
func (s *TraderStore) Archive(userID, id string) error {
	if uuid.Validate(id) != nil {
		return gorm.ErrRecordNotFound
	}
	result := s.db.Exec(`UPDATE trader.traders
		SET archived_at = now(), updated_at = now(), is_running = false, agent_model_id = NULL, account_id = NULL, strategy_id = NULL
		WHERE id = ? AND user_id = ? AND archived_at IS NULL`, id, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Get returns one of a user's traders.
func (s *TraderStore) Get(userID, id string) (*Trader, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var trader Trader
	if err := s.inUse().Where("id = ? AND user_id = ?", id, userID).First(&trader).Error; err != nil {
		return nil, err
	}
	return &trader, nil
}

// GetFullConfig gets trader full configuration
func (s *TraderStore) GetFullConfig(userID, traderID string) (*TraderFullConfig, error) {
	trader, err := s.Get(userID, traderID)
	if err != nil {
		return nil, err
	}

	// Get model
	var agentModel AgentModel
	err = s.db.Where("id = ? AND user_id = ?", trader.AgentModelID, userID).First(&agentModel).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	// Get exchange
	exchange, err := NewExchangeStore(s.db).GetByID(userID, trader.ExchangeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange: %w", err)
	}

	strategy, err := NewStrategyStore(s.db).Get(userID, trader.StrategyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}

	return &TraderFullConfig{
		Trader:     trader,
		AgentModel: &agentModel,
		Exchange:   exchange,
		Strategy:   strategy,
	}, nil
}

// GetByID gets a trader by ID without requiring userID (for public APIs)
func (s *TraderStore) GetByID(traderID string) (*Trader, error) {
	if uuid.Validate(traderID) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var trader Trader
	err := s.inUse().Where("id = ?", traderID).First(&trader).Error
	if err != nil {
		return nil, err
	}
	return &trader, nil
}

// ListByExchangeID gets traders that use a specific exchange
func (s *TraderStore) ListByExchangeID(userID, exchangeID string) ([]*Trader, error) {
	if uuid.Validate(exchangeID) != nil {
		return nil, nil
	}
	var traders []*Trader
	err := s.inUse().Where("user_id = ? AND account_id = ?", userID, exchangeID).Order("created_at DESC").Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// ListByAgentModelID gets traders that use a specific model
func (s *TraderStore) ListByAgentModelID(userID, agentModelID string) ([]*Trader, error) {
	if uuid.Validate(agentModelID) != nil {
		return nil, nil
	}
	var traders []*Trader
	err := s.inUse().Where("user_id = ? AND agent_model_id = ?", userID, agentModelID).Order("created_at DESC").Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}
