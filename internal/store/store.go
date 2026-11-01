// Package store provides unified database storage layer
// All database operations should go through this package
package store

import (
	"database/sql"
	"fmt"
	"sync"
	"vergex/internal/logger"
	"vergex/internal/store/database"

	"gorm.io/gorm"
)

// Store unified data storage interface
type Store struct {
	gdb *gorm.DB // GORM database connection
	db  *sql.DB  // underlying connection of gdb, closed by Close

	// Sub-stores (lazy initialization)
	user           *UserStore
	agentModel     *AgentModelStore
	exchange       *ExchangeStore
	trader         *TraderStore
	decision       *DecisionStore
	position       *PositionStore
	strategy       *StrategyStore
	equity         *EquityStore
	order          *OrderStore
	agentCall      *AgentCallStore
	telegramConfig TelegramConfigStore

	mu sync.RWMutex
}

// New connects to PostgreSQL and returns a Store with its tables in place
func New(cfg database.Config) (*Store, error) {
	gdb, err := database.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := database.Migrate(gdb); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	s, err := NewMigrated(gdb)
	if err != nil {
		return nil, err
	}

	logger.Infof("✅ Database initialized (GORM, PostgreSQL)")
	return s, nil
}

// NewMigrated creates a Store on a connection whose migrations have run.
func NewMigrated(gdb *gorm.DB) (*Store, error) {
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	return &Store{gdb: gdb, db: sqlDB}, nil
}

// User gets user storage
func (s *Store) User() *UserStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user == nil {
		s.user = NewUserStore(s.gdb)
	}
	return s.user
}

// AgentModel gets model storage
func (s *Store) AgentModel() *AgentModelStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentModel == nil {
		s.agentModel = NewAgentModelStore(s.gdb)
	}
	return s.agentModel
}

// Exchange gets exchange storage
func (s *Store) Exchange() *ExchangeStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exchange == nil {
		s.exchange = NewExchangeStore(s.gdb)
	}
	return s.exchange
}

// Trader gets trader storage
func (s *Store) Trader() *TraderStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trader == nil {
		s.trader = NewTraderStore(s.gdb)
	}
	return s.trader
}

// Decision gets decision log storage
func (s *Store) Decision() *DecisionStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decision == nil {
		s.decision = NewDecisionStore(s.gdb)
	}
	return s.decision
}

// Position gets position storage
func (s *Store) Position() *PositionStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.position == nil {
		s.position = NewPositionStore(s.gdb)
	}
	return s.position
}

// Strategy gets strategy storage
func (s *Store) Strategy() *StrategyStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.strategy == nil {
		s.strategy = NewStrategyStore(s.gdb)
	}
	return s.strategy
}

// Equity gets equity storage
func (s *Store) Equity() *EquityStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.equity == nil {
		s.equity = NewEquityStore(s.gdb)
	}
	return s.equity
}

// Order gets order storage
func (s *Store) Order() *OrderStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.order == nil {
		s.order = NewOrderStore(s.gdb)
	}
	return s.order
}

// AgentCall gets agent charge storage
func (s *Store) AgentCall() *AgentCallStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentCall == nil {
		s.agentCall = NewAgentCallStore(s.gdb)
	}
	return s.agentCall
}

// AgentConversation gets conversation storage
func (s *Store) AgentConversation() *AgentConversationStore {
	return NewAgentConversationStore(s.gdb)
}

// AccountClose gets manual close storage
func (s *Store) AccountClose() *AccountCloseStore {
	return &AccountCloseStore{db: s.gdb}
}

// Ledger gets the trading ledger
func (s *Store) Ledger() *LedgerStore {
	return NewLedgerStore(s.gdb)
}

// TelegramConfig gets Telegram bot configuration storage
func (s *Store) TelegramConfig() TelegramConfigStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.telegramConfig == nil {
		s.telegramConfig = NewTelegramConfigStore(s.gdb)
	}
	return s.telegramConfig
}

// LoadSetting reads a module's settings into dst, and reports whether it has any yet.
func (s *Store) LoadSetting(module string, dst any) (bool, error) {
	return database.LoadSetting(s.gdb, module, dst)
}

// PatchSetting merges fields into a module's settings.
func (s *Store) PatchSetting(module string, fields map[string]any) error {
	return database.PatchSetting(s.gdb, module, fields)
}

// Close closes database connection
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// GormDB returns the GORM database connection
func (s *Store) GormDB() *gorm.DB {
	return s.gdb
}

// Transaction executes transaction with GORM
func (s *Store) Transaction(fn func(tx *gorm.DB) error) error {
	return s.gdb.Transaction(fn)
}
