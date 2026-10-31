package manager

import (
	"fmt"
	"sync"
	"time"
	"vergex/internal/logger"
	"vergex/internal/store"
	"vergex/internal/trader"
)

func traderLogTag(traderID, traderName string) string {
	if traderName != "" {
		return fmt.Sprintf("[trader_id=%s trader_name=%s]", traderID, traderName)
	}
	return fmt.Sprintf("[trader_id=%s]", traderID)
}

// loadRetryInterval is how long EnsureUserTradersLoaded leaves a trader whose
// last load failed before trying it again. Loading can call the exchange (for
// the initial balance) under the manager lock, and the read endpoints that
// ensure loading are polled by the Dashboard.
const loadRetryInterval = time.Minute

// TraderManager manages multiple trader instances
type TraderManager struct {
	traders      map[string]*trader.AutoTrader // key: trader ID
	loadErrors   map[string]error              // key: trader ID, stores last load error
	loadFailedAt map[string]time.Time          // key: trader ID, time of last failed load
	now          func() time.Time
	mu           sync.RWMutex
}

// NewTraderManager creates a trader manager
func NewTraderManager() *TraderManager {
	return &TraderManager{
		traders:      make(map[string]*trader.AutoTrader),
		loadErrors:   make(map[string]error),
		loadFailedAt: make(map[string]time.Time),
		now:          time.Now,
	}
}

// recordLoad remembers the outcome of a load attempt. Caller holds tm.mu.
func (tm *TraderManager) recordLoad(traderID string, err error) {
	if err != nil {
		tm.loadErrors[traderID] = err
		tm.loadFailedAt[traderID] = tm.now()
		return
	}
	delete(tm.loadErrors, traderID)
	delete(tm.loadFailedAt, traderID)
}

// GetLoadError returns the last load error for a trader
func (tm *TraderManager) GetLoadError(traderID string) error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.loadErrors[traderID]
}

// GetTrader retrieves a trader by ID
func (tm *TraderManager) GetTrader(id string) (*trader.AutoTrader, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, exists := tm.traders[id]
	if !exists {
		return nil, fmt.Errorf("trader ID '%s' does not exist", id)
	}
	return t, nil
}

// GetAllTraders retrieves all traders
func (tm *TraderManager) GetAllTraders() map[string]*trader.AutoTrader {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[string]*trader.AutoTrader)
	for id, t := range tm.traders {
		result[id] = t
	}
	return result
}

// GetTraderIDs retrieves all trader IDs
func (tm *TraderManager) GetTraderIDs() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	ids := make([]string, 0, len(tm.traders))
	for id := range tm.traders {
		ids = append(ids, id)
	}
	return ids
}

// StartAll starts all traders
func (tm *TraderManager) StartAll() {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	logger.Info("🚀 Starting all traders...")
	for id, t := range tm.traders {
		go func(traderID string, at *trader.AutoTrader) {
			logger.Infof("%s ▶️ Starting trader runtime", traderLogTag(traderID, at.GetName()))
			if err := at.Run(); err != nil {
				logger.Warnf("%s runtime error: %v", traderLogTag(traderID, at.GetName()), err)
			}
		}(id, t)
	}
}

// StopAll stops all traders
func (tm *TraderManager) StopAll() {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	logger.Info("⏹  Stopping all traders...")
	for _, t := range tm.traders {
		t.Stop()
	}
}

// RemoveTrader removes a trader from memory (does not affect database)
// Used to force reload when updating trader configuration
// If the trader is running, it will be stopped first
func (tm *TraderManager) RemoveTrader(traderID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if t, exists := tm.traders[traderID]; exists {
		// Stop the trader if it's running (this ensures the goroutine exits)
		status := t.GetStatus()
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			logger.Infof("⏹ Stopping trader %s before removing from memory...", traderID)
			t.Stop()
		}
		delete(tm.traders, traderID)
		logger.Infof("✓ Trader %s removed from memory", traderID)
	}
}

// LoadUserTradersFromStore loads traders from store for a specific user to
// memory, retrying every trader that is not loaded. Use it after the user
// changes a trader, model or exchange, or asks to start one.
func (tm *TraderManager) LoadUserTradersFromStore(st *store.Store, userID string) error {
	return tm.loadUserTraders(st, userID, false)
}

// EnsureUserTradersLoaded loads the user's traders like LoadUserTradersFromStore
// but skips a trader whose load failed less than loadRetryInterval ago, so
// polled reads do not hit an unreachable exchange on every request.
func (tm *TraderManager) EnsureUserTradersLoaded(st *store.Store, userID string) error {
	return tm.loadUserTraders(st, userID, true)
}

func (tm *TraderManager) loadUserTraders(st *store.Store, userID string, backoff bool) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Get all traders for the specified user
	traders, err := st.Trader().List(userID)
	if err != nil {
		return fmt.Errorf("failed to get trader list for user %s: %w", userID, err)
	}

	logger.Infof("📋 Loading trader configurations for user %s: %d traders", userID, len(traders))

	// Get AI model and exchange lists (query only once outside loop)
	aiModels, err := st.AIModel().List(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to get AI model config for user %s: %v", userID, err)
		return fmt.Errorf("failed to get AI model config: %w", err)
	}

	exchanges, err := st.Exchange().List(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to get exchange config for user %s: %v", userID, err)
		return fmt.Errorf("failed to get exchange config: %w", err)
	}

	// Load configuration for each trader
	for _, traderCfg := range traders {
		// Check if this trader is already loaded
		if _, exists := tm.traders[traderCfg.ID]; exists {
			// Trader already loaded - this is normal, no need to log
			continue
		}
		if failedAt, failed := tm.loadFailedAt[traderCfg.ID]; backoff && failed && tm.now().Sub(failedAt) < loadRetryInterval {
			continue
		}

		// Find AI model config from already queried list
		var aiModelCfg *store.AIModel
		for _, model := range aiModels {
			if model.ID == traderCfg.AIModelID {
				aiModelCfg = model
				break
			}
		}
		if aiModelCfg == nil {
			for _, model := range aiModels {
				if model.Provider == traderCfg.AIModelID {
					aiModelCfg = model
					break
				}
			}
		}

		if aiModelCfg == nil {
			logger.Infof("⚠️ AI model %s for trader %s does not exist, skipping", traderCfg.AIModelID, traderCfg.Name)
			continue
		}

		if !aiModelCfg.Enabled {
			logger.Infof("⚠️ AI model %s for trader %s is not enabled, skipping", traderCfg.AIModelID, traderCfg.Name)
			continue
		}

		// Find exchange config from already queried list
		var exchangeCfg *store.Exchange
		for _, exchange := range exchanges {
			if exchange.ID == traderCfg.ExchangeID {
				exchangeCfg = exchange
				break
			}
		}

		if exchangeCfg == nil {
			logger.Infof("⚠️ Exchange %s for trader %s does not exist, skipping", traderCfg.ExchangeID, traderCfg.Name)
			continue
		}

		if !exchangeCfg.Enabled {
			logger.Infof("⚠️ Exchange %s for trader %s is not enabled, skipping", traderCfg.ExchangeID, traderCfg.Name)
			continue
		}

		// Use existing method to load trader
		logger.Infof("📦 Loading trader %s (AI Model: %s, Exchange: %s/%s, Strategy ID: %s)", traderCfg.Name, aiModelCfg.Provider, exchangeCfg.ExchangeType, exchangeCfg.AccountName, traderCfg.StrategyID)
		err = tm.addTraderFromStore(traderCfg, aiModelCfg, exchangeCfg, st)
		if err != nil {
			logger.Warnf("%s failed to load trader: %v", traderLogTag(traderCfg.ID, traderCfg.Name), err)
		}
		tm.recordLoad(traderCfg.ID, err)
	}

	return nil
}

// LoadTradersFromStore loads all traders from store to memory (new API)
func (tm *TraderManager) LoadTradersFromStore(st *store.Store) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Get all users
	userIDs, err := st.User().GetAllIDs()
	if err != nil {
		return fmt.Errorf("failed to get user list: %w", err)
	}

	logger.Infof("📋 Found %d users, loading all trader configurations...", len(userIDs))

	var allTraders []*store.Trader
	for _, userID := range userIDs {
		// Get traders for each user
		traders, err := st.Trader().List(userID)
		if err != nil {
			logger.Infof("⚠️ Failed to get traders for user %s: %v", userID, err)
			continue
		}
		logger.Infof("📋 User %s: %d traders", userID, len(traders))
		allTraders = append(allTraders, traders...)
	}

	logger.Infof("📋 Total loaded trader configurations: %d", len(allTraders))

	// Get AI model and exchange configs for each trader
	for _, traderCfg := range allTraders {
		// Get AI model config
		aiModels, err := st.AIModel().List(traderCfg.UserID)
		if err != nil {
			logger.Infof("⚠️  Failed to get AI model config: %v", err)
			continue
		}

		var aiModelCfg *store.AIModel
		// Prioritize exact match on model.ID
		for _, model := range aiModels {
			if model.ID == traderCfg.AIModelID {
				aiModelCfg = model
				break
			}
		}
		// If no exact match, try matching provider (for backward compatibility)
		if aiModelCfg == nil {
			for _, model := range aiModels {
				if model.Provider == traderCfg.AIModelID {
					aiModelCfg = model
					logger.Infof("⚠️  Trader %s using legacy provider match: %s -> %s", traderCfg.Name, traderCfg.AIModelID, model.ID)
					break
				}
			}
		}

		if aiModelCfg == nil {
			logger.Infof("⚠️  AI model %s for trader %s does not exist, skipping", traderCfg.AIModelID, traderCfg.Name)
			continue
		}

		if !aiModelCfg.Enabled {
			logger.Infof("⚠️  AI model %s for trader %s is not enabled, skipping", traderCfg.AIModelID, traderCfg.Name)
			continue
		}

		// Get exchange config
		exchanges, err := st.Exchange().List(traderCfg.UserID)
		if err != nil {
			logger.Infof("⚠️  Failed to get exchange config: %v", err)
			continue
		}

		var exchangeCfg *store.Exchange
		for _, exchange := range exchanges {
			if exchange.ID == traderCfg.ExchangeID {
				exchangeCfg = exchange
				break
			}
		}

		if exchangeCfg == nil {
			logger.Infof("⚠️  Exchange %s for trader %s does not exist, skipping", traderCfg.ExchangeID, traderCfg.Name)
			continue
		}

		if !exchangeCfg.Enabled {
			logger.Infof("⚠️  Exchange %s for trader %s is not enabled, skipping", traderCfg.ExchangeID, traderCfg.Name)
			continue
		}

		// Add to TraderManager
		err = tm.addTraderFromStore(traderCfg, aiModelCfg, exchangeCfg, st)
		tm.recordLoad(traderCfg.ID, err)
		if err != nil {
			logger.Warnf("%s failed to add trader: %v", traderLogTag(traderCfg.ID, traderCfg.Name), err)
			continue
		}
	}

	logger.Infof("✓ Successfully loaded %d traders to memory", len(tm.traders))
	return nil
}

// addTraderFromStore internal method: adds trader from store configuration
func (tm *TraderManager) addTraderFromStore(traderCfg *store.Trader, aiModelCfg *store.AIModel, exchangeCfg *store.Exchange, st *store.Store) error {
	if _, exists := tm.traders[traderCfg.ID]; exists {
		return fmt.Errorf("trader ID '%s' already exists", traderCfg.ID)
	}

	// Load strategy config (must have strategy)
	var strategyConfig *store.StrategyConfig
	strategyConfigRaw := ""
	if traderCfg.StrategyID != "" {
		strategy, err := st.Strategy().Get(traderCfg.UserID, traderCfg.StrategyID)
		if err != nil {
			return fmt.Errorf("failed to load strategy %s for trader %s: %w", traderCfg.StrategyID, traderCfg.Name, err)
		}
		// Parse JSON config
		strategyConfig, err = strategy.ParseConfig()
		if err != nil {
			return fmt.Errorf("failed to parse strategy config for trader %s: %w", traderCfg.Name, err)
		}
		strategyConfigRaw = strategy.Config
		strategyConfig.ClampLimits()
		// Sizing comes from the strategy's RiskControl. Autopilot separately
		// distributes its margin budget across the configured slots while the
		// per-position ratio remains a hard safety cap.
		logger.Infof("✓ Trader %s loaded strategy config: %s (maxPos=%d, posRatio=%.1f)", traderCfg.Name, strategy.Name, strategyConfig.RiskControl.MaxPositions, strategyConfig.RiskControl.AltcoinMaxPositionValueRatio)
	} else {
		return fmt.Errorf("trader %s has no strategy configured", traderCfg.Name)
	}

	// Build AutoTraderConfig
	traderConfig := trader.AutoTraderConfig{
		ID:                traderCfg.ID,
		Name:              traderCfg.Name,
		StrategyID:        traderCfg.StrategyID,
		AIModel:           aiModelCfg.Provider,
		Exchange:          exchangeCfg.ExchangeType, // Exchange type: binance/okx
		ExchangeID:        exchangeCfg.ID,           // Exchange account UUID (for multi-account)
		BinanceAPIKey:     "",
		BinanceSecretKey:  "",
		UseQwen:           aiModelCfg.Provider == "qwen",
		DeepSeekKey:       "",
		QwenKey:           "",
		CustomAPIURL:      aiModelCfg.CustomAPIURL,
		CustomModelName:   aiModelCfg.CustomModelName,
		ScanInterval:      time.Duration(traderCfg.ScanIntervalMinutes) * time.Minute,
		InitialBalance:    traderCfg.InitialBalance,
		IsCrossMargin:     traderCfg.IsCrossMargin,
		StrategyConfig:    strategyConfig,
		StrategyConfigRaw: strategyConfigRaw,
	}

	logger.Infof("📊 Loading trader %s: ScanIntervalMinutes=%d (from DB), ScanInterval=%v",
		traderCfg.Name, traderCfg.ScanIntervalMinutes, traderConfig.ScanInterval)

	// Set API keys based on exchange type (convert EncryptedString to string)
	switch exchangeCfg.ExchangeType {
	case "binance":
		traderConfig.BinanceAPIKey = string(exchangeCfg.APIKey)
		traderConfig.BinanceSecretKey = string(exchangeCfg.SecretKey)
	case "okx":
		traderConfig.OKXAPIKey = string(exchangeCfg.APIKey)
		traderConfig.OKXSecretKey = string(exchangeCfg.SecretKey)
		traderConfig.OKXPassphrase = string(exchangeCfg.Passphrase)
	}

	// Set API keys based on AI model (convert EncryptedString to string)
	switch aiModelCfg.Provider {
	case "qwen":
		traderConfig.QwenKey = string(aiModelCfg.APIKey)
	case "deepseek":
		traderConfig.DeepSeekKey = string(aiModelCfg.APIKey)
	default:
		// For other providers (grok, openai, claude, gemini, kimi, etc.), use CustomAPIKey
		traderConfig.CustomAPIKey = string(aiModelCfg.APIKey)
	}

	// Create trader instance
	at, err := trader.NewAutoTrader(traderConfig, st, traderCfg.UserID)
	if err != nil {
		return fmt.Errorf("failed to create trader: %w", err)
	}

	// Set custom prompt (if exists)
	if traderCfg.CustomPrompt != "" {
		at.SetCustomPrompt(traderCfg.CustomPrompt)
		at.SetOverrideBasePrompt(traderCfg.OverrideBasePrompt)
		if traderCfg.OverrideBasePrompt {
			logger.Infof("✓ Set custom trading strategy prompt (overriding base prompt)")
		} else {
			logger.Infof("✓ Set custom trading strategy prompt (supplementing base prompt)")
		}
	}

	tm.traders[traderCfg.ID] = at
	logger.Infof("✓ Trader '%s' (%s + %s/%s) loaded to memory", traderCfg.Name, aiModelCfg.Provider, exchangeCfg.ExchangeType, exchangeCfg.AccountName)

	// Auto-start if trader was running before shutdown
	if traderCfg.IsRunning {
		logger.Infof("%s 🔄 Auto-starting trader (was running before shutdown)...", traderLogTag(traderCfg.ID, traderCfg.Name))
		go func(trader *trader.AutoTrader, traderName, traderID, userID string) {
			if err := trader.Run(); err != nil {
				logger.Warnf("%s trader stopped with error: %v", traderLogTag(traderID, traderName), err)
				// Update database to reflect stopped state
				if st != nil {
					_ = st.Trader().UpdateStatus(userID, traderID, false)
				}
			}
		}(at, traderCfg.Name, traderCfg.ID, traderCfg.UserID)
		logger.Infof("%s ✅ Trader auto-started successfully", traderLogTag(traderCfg.ID, traderCfg.Name))
	}

	return nil
}
