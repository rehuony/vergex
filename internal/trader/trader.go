package trader

import (
	"encoding/json"
	"fmt"
	"time"
	"vergex/internal/account"
	"vergex/internal/agent"
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
type Store struct {
	db *gorm.DB
}

// NewTraderStore creates a new trader store
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Trader trader configuration
type Trader struct {
	ID           string `gorm:"primaryKey" json:"id"`
	UserID       string `gorm:"column:user_id" json:"user_id"`
	Name         string `gorm:"column:name" json:"name"`
	AgentModelID string `gorm:"column:agent_model_id" json:"agent_model_id"`
	AccountID    string `gorm:"column:account_id" json:"account_id"`
	// Kind says what runs it, and Config is what that kind decides: see trader/kind.
	Kind                string          `gorm:"column:kind" json:"kind"`
	Config              json.RawMessage `gorm:"column:config" json:"config"`
	InitialBalance      float64         `gorm:"column:initial_balance" json:"initial_balance"`
	ScanIntervalMinutes int             `gorm:"column:scan_interval_minutes;default:15" json:"scan_interval_minutes"`
	IsRunning           bool            `gorm:"column:is_running" json:"is_running"`
	MarginMode          string          `gorm:"column:margin_mode;default:cross" json:"margin_mode"`
	CreatedAt           time.Time       `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time       `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for Trader
func (Trader) TableName() string {
	return "trader.traders"
}

// IsCrossMargin reports whether the trader uses cross margin.
func (t *Trader) IsCrossMargin() bool { return t.MarginMode != MarginIsolated }

// FullConfig trader full configuration (includes model and account)
type FullConfig struct {
	Trader     *Trader
	AgentModel *agent.Model
	Account    *account.Account
}

// inUse is the traders that have not been deleted. Nothing below reads the others:
// deleting one archives it and lets go of its model and account.
func (s *Store) inUse() *gorm.DB {
	return s.db.Where("archived_at IS NULL")
}

// Create creates trader
func (s *Store) Create(trader *Trader) error {
	return s.db.Create(trader).Error
}

// List gets user's trader list
func (s *Store) List(userID string) ([]*Trader, error) {
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
func (s *Store) UpdateStatus(userID, id string, isRunning bool) error {
	if uuid.Validate(id) != nil {
		return gorm.ErrRecordNotFound
	}
	return s.inUse().Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_running", isRunning).Error
}

// Update updates trader configuration
func (s *Store) Update(trader *Trader) error {
	if uuid.Validate(trader.ID) != nil {
		return gorm.ErrRecordNotFound
	}
	logger.Debugf("TraderStore.Update: ID=%s, Name=%s, AgentModelID=%s",
		trader.ID, trader.Name, trader.AgentModelID)

	updates := map[string]interface{}{
		"name":           trader.Name,
		"agent_model_id": trader.AgentModelID,
		"account_id":     trader.AccountID,
		"margin_mode":    trader.MarginMode,
	}
	if len(trader.Config) > 0 {
		updates["config"] = trader.Config
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
func (s *Store) UpdateInitialBalance(userID, id string, newBalance float64) error {
	if uuid.Validate(id) != nil {
		return gorm.ErrRecordNotFound
	}
	return s.inUse().Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("initial_balance", newBalance).Error
}

// Archive deletes a trader as far as anything can see, and keeps its cycles, equity
// and ledger entries, and what it was configured with. It lets go of its model and account,
// so they can go too.
func (s *Store) Archive(userID, id string) error {
	if uuid.Validate(id) != nil {
		return gorm.ErrRecordNotFound
	}
	result := s.db.Exec(`UPDATE trader.traders
		SET archived_at = now(), updated_at = now(), is_running = false, agent_model_id = NULL, account_id = NULL
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
func (s *Store) Get(userID, id string) (*Trader, error) {
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
func (s *Store) GetFullConfig(userID, traderID string) (*FullConfig, error) {
	trader, err := s.Get(userID, traderID)
	if err != nil {
		return nil, err
	}

	// Get model
	var agentModel agent.Model
	err = s.db.Where("id = ? AND user_id = ?", trader.AgentModelID, userID).First(&agentModel).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	// Get account
	account, err := account.NewStore(s.db).GetByID(userID, trader.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &FullConfig{
		Trader:     trader,
		AgentModel: &agentModel,
		Account:    account,
	}, nil
}

// GetByID gets a trader by ID without requiring userID (for public APIs)
func (s *Store) GetByID(traderID string) (*Trader, error) {
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

// RunningOn returns the longest-standing trader running on an account, if any.
func (s *Store) RunningOn(accountID string) (string, error) {
	var ids []string
	err := s.db.Raw(`SELECT id::text FROM trader.traders WHERE account_id = ? AND is_running ORDER BY created_at, id LIMIT 1`, accountID).Scan(&ids).Error
	if err != nil || len(ids) == 0 {
		return "", err
	}
	return ids[0], nil
}

// RunningAccounts lists the accounts a trader is running on.
func (s *Store) RunningAccounts() ([]string, error) {
	var ids []string
	err := s.db.Raw(`SELECT DISTINCT account_id::text FROM trader.traders WHERE is_running AND account_id IS NOT NULL`).Scan(&ids).Error
	return ids, err
}

// ListByAccountID gets traders that use a specific account
func (s *Store) ListByAccountID(userID, accountID string) ([]*Trader, error) {
	if uuid.Validate(accountID) != nil {
		return nil, nil
	}
	var traders []*Trader
	err := s.inUse().Where("user_id = ? AND account_id = ?", userID, accountID).Order("created_at DESC").Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// ListByAgentModelID gets traders that use a specific model
func (s *Store) ListByAgentModelID(userID, agentModelID string) ([]*Trader, error) {
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
