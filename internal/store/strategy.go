package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StrategyStore strategy storage
type StrategyStore struct {
	db *gorm.DB
}

// What a trading strategy is in strategy.strategies.
const strategyDomain = "trading"

// Strategy strategy configuration
type Strategy struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	UserID      string    `json:"user_id"`
	Domain      string    `json:"-"`
	Type        string    `json:"-"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	Config      string    `json:"config"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Strategy) TableName() string { return "strategy.strategies" }

// NewStrategyStore creates a new StrategyStore
func NewStrategyStore(db *gorm.DB) *StrategyStore {
	return &StrategyStore{db: db}
}

// typeColumn is the type column for a stored config.
func typeColumn(config string) string {
	var fields struct {
		StrategyType string `json:"strategy_type"`
	}
	if json.Unmarshal([]byte(config), &fields) == nil && normalizeStrategyType(fields.StrategyType) == "grid_trading" {
		return "grid"
	}
	return "agent"
}

// Create create a strategy
func (s *StrategyStore) Create(strategy *Strategy) error {
	return createStrategy(s.db, strategy)
}

func createStrategy(db *gorm.DB, strategy *Strategy) error {
	strategy.Domain = strategyDomain
	strategy.Type = typeColumn(strategy.Config)
	return db.Create(strategy).Error
}

// SeedIfNone creates the given strategies for an account that has none, and returns
// those it created. They are a starting point, not something to keep in sync: matching
// by name would add a second copy whenever the language differs or the user renamed one.
func (s *StrategyStore) SeedIfNone(userID string, strategies []*Strategy) ([]*Strategy, error) {
	var created []*Strategy
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Strategy{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to count strategies: %w", err)
		}
		if count > 0 {
			return nil
		}
		for _, strategy := range strategies {
			if err := createStrategy(tx, strategy); err != nil {
				return fmt.Errorf("failed to create strategy %q: %w", strategy.Name, err)
			}
		}
		created = strategies
		return nil
	})
	return created, err
}

// Update update a strategy
func (s *StrategyStore) Update(strategy *Strategy) error {
	return s.db.Model(&Strategy{}).
		Where("id = ? AND user_id = ?", strategy.ID, strategy.UserID).
		Updates(map[string]interface{}{
			"name":        strategy.Name,
			"description": strategy.Description,
			"config":      strategy.Config,
			"type":        typeColumn(strategy.Config),
			"updated_at":  time.Now().UTC(),
		}).Error
}

// Delete delete a strategy
func (s *StrategyStore) Delete(userID, id string) error {
	st, err := s.Get(userID, id)
	if err != nil {
		return err
	}
	if st.IsActive {
		return fmt.Errorf("cannot delete active strategy")
	}

	// Check if any trader references this strategy
	var count int64
	if err := s.db.Model(&Trader{}).
		Where("user_id = ? AND strategy_id = ?", userID, id).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete strategy in use by %d trader(s) - reassign those traders first", count)
	}

	return s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Strategy{}).Error
}

// List get user's strategy list
func (s *StrategyStore) List(userID string) ([]*Strategy, error) {
	var strategies []*Strategy
	err := s.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&strategies).Error
	if err != nil {
		return nil, err
	}
	return strategies, nil
}

// Get get a single strategy
func (s *StrategyStore) Get(userID, id string) (*Strategy, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var st Strategy
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetActive get user's currently active strategy
func (s *StrategyStore) GetActive(userID string) (*Strategy, error) {
	var st Strategy
	err := s.db.Where("user_id = ? AND is_active", userID).First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// SetActive set active strategy (will first deactivate other strategies)
func (s *StrategyStore) SetActive(userID, strategyID string) error {
	if _, err := s.Get(userID, strategyID); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Strategy{}).Where("user_id = ? AND is_active", userID).
			Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Model(&Strategy{}).Where("id = ? AND user_id = ?", strategyID, userID).
			Update("is_active", true).Error
	})
}

// Duplicate duplicate a strategy (used to create custom strategy based on default strategy)
func (s *StrategyStore) Duplicate(userID, sourceID, newID, newName string) error {
	// get source strategy
	source, err := s.Get(userID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source strategy: %w", err)
	}

	// create new strategy
	newStrategy := &Strategy{
		ID:          newID,
		UserID:      userID,
		Name:        newName,
		Description: "Created based on [" + source.Name + "]",
		IsActive:    false,
		Config:      source.Config,
	}

	return s.Create(newStrategy)
}

// ParseConfig parse strategy configuration JSON
func (s *Strategy) ParseConfig() (*StrategyConfig, error) {
	var config StrategyConfig
	if err := json.Unmarshal([]byte(s.Config), &config); err != nil {
		return nil, fmt.Errorf("failed to parse strategy configuration: %w", err)
	}
	return &config, nil
}

// SetConfig set strategy configuration
func (s *Strategy) SetConfig(config *StrategyConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize strategy configuration: %w", err)
	}
	s.Config = string(data)
	return nil
}
