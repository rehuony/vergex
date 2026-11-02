package trader

import (
	"fmt"
	"sync"
	"time"
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/logger"
	"vergex/internal/trader/kind"
)

func traderLogTag(traderID, traderName string) string {
	if traderName != "" {
		return fmt.Sprintf("[trader_id=%s trader_name=%s]", traderID, traderName)
	}
	return fmt.Sprintf("[trader_id=%s]", traderID)
}

// loadRetryInterval is how long EnsureUserTradersLoaded leaves a trader whose
// last load failed before trying it again. Loading can call the exchange (for
// the initial balance), and the read endpoints that ensure loading are polled by
// the Dashboard.
const loadRetryInterval = time.Minute

// Manager manages multiple trader instances
type Manager struct {
	stores       *Stores
	traders      map[string]*AutoTrader // key: trader ID
	loadErrors   map[string]error       // key: trader ID, stores last load error
	loadFailedAt map[string]time.Time   // key: trader ID, time of last failed load
	loading      map[string]int         // key: trader ID, loads building it right now
	removals     uint64                 // RemoveTrader calls so far
	removedAt    map[string]uint64      // key: trader ID, value of removals when last removed
	newTrader    func(AutoTraderConfig, *Stores, string) (*AutoTrader, error)
	now          func() time.Time
	mu           sync.RWMutex // Guards the fields above, and is never held across a store or exchange call
}

// NewManager creates a trader manager on the given stores
func NewManager(stores *Stores) *Manager {
	return &Manager{
		stores:       stores,
		traders:      make(map[string]*AutoTrader),
		loadErrors:   make(map[string]error),
		loadFailedAt: make(map[string]time.Time),
		loading:      make(map[string]int),
		removedAt:    make(map[string]uint64),
		newTrader:    NewAutoTrader,
		now:          time.Now,
	}
}

// recordLoad remembers the outcome of a load attempt. Caller holds tm.mu.
func (tm *Manager) recordLoad(traderID string, err error) {
	if err != nil {
		tm.loadErrors[traderID] = err
		tm.loadFailedAt[traderID] = tm.now()
		return
	}
	delete(tm.loadErrors, traderID)
	delete(tm.loadFailedAt, traderID)
}

// GetLoadError returns the last load error for a trader
func (tm *Manager) GetLoadError(traderID string) error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.loadErrors[traderID]
}

// GetTrader retrieves a trader by ID
func (tm *Manager) GetTrader(id string) (*AutoTrader, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, exists := tm.traders[id]
	if !exists {
		return nil, fmt.Errorf("trader ID '%s' does not exist", id)
	}
	return t, nil
}

// GetAllTraders retrieves all traders
func (tm *Manager) GetAllTraders() map[string]*AutoTrader {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make(map[string]*AutoTrader)
	for id, t := range tm.traders {
		result[id] = t
	}
	return result
}

// GetTraderIDs retrieves all trader IDs
func (tm *Manager) GetTraderIDs() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	ids := make([]string, 0, len(tm.traders))
	for id := range tm.traders {
		ids = append(ids, id)
	}
	return ids
}

// StopAll stops all traders
func (tm *Manager) StopAll() {
	logger.Info("⏹  Stopping all traders...")
	for _, t := range tm.GetAllTraders() {
		t.Stop()
	}
}

// RemoveTrader removes a trader from memory (does not affect database)
// Used to force reload when updating trader configuration
// If the trader is running, it will be stopped first
func (tm *Manager) RemoveTrader(traderID string) {
	tm.mu.Lock()
	t, exists := tm.traders[traderID]
	tm.removals++
	tm.removedAt[traderID] = tm.removals
	tm.mu.Unlock()

	if !exists {
		return
	}

	// Stop the trader if it's running (this ensures the goroutine exits). That waits for
	// its monitor, which can be in an exchange call, so it happens outside the lock. The
	// trader stays in the map meanwhile, so no load can start a second copy beside it.
	status := t.GetStatus()
	if isRunning, ok := status["is_running"].(bool); ok && isRunning {
		logger.Infof("⏹ Stopping trader %s before removing from memory...", traderID)
		t.Stop()
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.traders[traderID] == t {
		delete(tm.traders, traderID)
		logger.Infof("✓ Trader %s removed from memory", traderID)
	}
}

// removalCount is the clock a load compares removals against. A load reads it before it
// reads any rows, so a removal after this point may mean those rows are out of date.
func (tm *Manager) removalCount() uint64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.removals
}

// needsLoad reports whether a load should build this trader. With backoff, a trader that
// another load is already building is left to it, so polled reads neither wait for a slow
// exchange nor pile more requests onto it.
func (tm *Manager) needsLoad(traderID string, backoff bool) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if _, exists := tm.traders[traderID]; exists {
		// Trader already loaded - this is normal, no need to log
		return false
	}
	if !backoff {
		return true
	}
	if tm.loading[traderID] > 0 {
		return false
	}
	failedAt, failed := tm.loadFailedAt[traderID]
	return !failed || tm.now().Sub(failedAt) >= loadRetryInterval
}

// LoadUserTradersFromStore loads traders from store for a specific user to
// memory, retrying every trader that is not loaded. Use it after the user
// changes a trader, model or exchange, or asks to start one.
func (tm *Manager) LoadUserTradersFromStore(userID string) error {
	return tm.loadUserTraders(userID, false)
}

// EnsureUserTradersLoaded loads the user's traders like LoadUserTradersFromStore
// but skips a trader whose load failed less than loadRetryInterval ago, so
// polled reads do not hit an unreachable exchange on every request.
func (tm *Manager) EnsureUserTradersLoaded(userID string) error {
	return tm.loadUserTraders(userID, true)
}

func (tm *Manager) loadUserTraders(userID string, backoff bool) error {
	st := tm.stores
	started := tm.removalCount()

	// Get all traders for the specified user
	traders, err := st.Traders.List(userID)
	if err != nil {
		return fmt.Errorf("failed to get trader list for user %s: %w", userID, err)
	}

	logger.Infof("📋 Loading trader configurations for user %s: %d traders", userID, len(traders))

	// Get model and exchange lists (query only once outside loop)
	agentModels, err := st.Models.List(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to get model config for user %s: %v", userID, err)
		return fmt.Errorf("failed to get model config: %w", err)
	}

	exchanges, err := st.Accounts.List(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to get exchange config for user %s: %v", userID, err)
		return fmt.Errorf("failed to get exchange config: %w", err)
	}

	// Load configuration for each trader
	for _, traderCfg := range traders {
		if !tm.needsLoad(traderCfg.ID, backoff) {
			continue
		}

		// Find model config from already queried list
		var agentModelCfg *agent.Model
		for _, model := range agentModels {
			if model.ID == traderCfg.AgentModelID {
				agentModelCfg = model
				break
			}
		}

		if agentModelCfg == nil {
			logger.Infof("⚠️ model %s for trader %s does not exist, skipping", traderCfg.AgentModelID, traderCfg.Name)
			continue
		}

		if !agentModelCfg.Enabled {
			logger.Infof("⚠️ model %s for trader %s is not enabled, skipping", traderCfg.AgentModelID, traderCfg.Name)
			continue
		}

		// Find exchange config from already queried list
		var accountCfg *account.Account
		for _, exchange := range exchanges {
			if exchange.ID == traderCfg.AccountID {
				accountCfg = exchange
				break
			}
		}

		if accountCfg == nil {
			logger.Infof("⚠️ Exchange %s for trader %s does not exist, skipping", traderCfg.AccountID, traderCfg.Name)
			continue
		}

		// Use existing method to load trader
		logger.Infof("📦 Loading trader %s (Model: %s, Exchange: %s/%s, Kind: %s)", traderCfg.Name, agentModelCfg.Provider, accountCfg.Venue, accountCfg.Name, traderCfg.Kind)
		err = tm.loadTrader(traderCfg, agentModelCfg, accountCfg, st, started)
		if err != nil {
			logger.Warnf("%s failed to load trader: %v", traderLogTag(traderCfg.ID, traderCfg.Name), err)
		}
	}

	return nil
}

// LoadTradersFromStore loads all traders from store to memory (new API)
func (tm *Manager) LoadTradersFromStore() error {
	st := tm.stores
	started := tm.removalCount()

	// Get all users
	userIDs, err := st.Users.GetAllIDs()
	if err != nil {
		return fmt.Errorf("failed to get user list: %w", err)
	}

	logger.Infof("📋 Found %d users, loading all trader configurations...", len(userIDs))

	var allTraders []*Trader
	for _, userID := range userIDs {
		// Get traders for each user
		traders, err := st.Traders.List(userID)
		if err != nil {
			logger.Infof("⚠️ Failed to get traders for user %s: %v", userID, err)
			continue
		}
		logger.Infof("📋 User %s: %d traders", userID, len(traders))
		allTraders = append(allTraders, traders...)
	}

	logger.Infof("📋 Total loaded trader configurations: %d", len(allTraders))

	// Get model and exchange configs for each trader
	for _, traderCfg := range allTraders {
		// Get model config
		agentModels, err := st.Models.List(traderCfg.UserID)
		if err != nil {
			logger.Infof("⚠️  Failed to get model config: %v", err)
			continue
		}

		var agentModelCfg *agent.Model
		// Prioritize exact match on model.ID
		for _, model := range agentModels {
			if model.ID == traderCfg.AgentModelID {
				agentModelCfg = model
				break
			}
		}

		if agentModelCfg == nil {
			logger.Infof("⚠️  model %s for trader %s does not exist, skipping", traderCfg.AgentModelID, traderCfg.Name)
			continue
		}

		if !agentModelCfg.Enabled {
			logger.Infof("⚠️  model %s for trader %s is not enabled, skipping", traderCfg.AgentModelID, traderCfg.Name)
			continue
		}

		// Get exchange config
		exchanges, err := st.Accounts.List(traderCfg.UserID)
		if err != nil {
			logger.Infof("⚠️  Failed to get exchange config: %v", err)
			continue
		}

		var accountCfg *account.Account
		for _, exchange := range exchanges {
			if exchange.ID == traderCfg.AccountID {
				accountCfg = exchange
				break
			}
		}

		if accountCfg == nil {
			logger.Infof("⚠️  Exchange %s for trader %s does not exist, skipping", traderCfg.AccountID, traderCfg.Name)
			continue
		}

		// Add to Manager
		err = tm.loadTrader(traderCfg, agentModelCfg, accountCfg, st, started)
		if err != nil {
			logger.Warnf("%s failed to add trader: %v", traderLogTag(traderCfg.ID, traderCfg.Name), err)
			continue
		}
	}

	logger.Infof("✓ Successfully loaded %d traders to memory", len(tm.GetTraderIDs()))
	return nil
}

// loadTrader builds a trader from store configuration and adds it to the manager. Building
// can call the exchange, so the lock is only taken around it. Two loads can build the same
// trader at once, and the first to finish wins. A load a RemoveTrader overtook since started
// drops what it built: its rows may be out of date, and whoever removed the trader reloads it.
func (tm *Manager) loadTrader(traderCfg *Trader, agentModelCfg *agent.Model, accountCfg *account.Account, st *Stores, started uint64) error {
	tm.mu.Lock()
	tm.loading[traderCfg.ID]++
	tm.mu.Unlock()

	at, err := tm.buildTrader(traderCfg, agentModelCfg, accountCfg, st)

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.loading[traderCfg.ID]--; tm.loading[traderCfg.ID] == 0 {
		delete(tm.loading, traderCfg.ID)
	}
	if tm.removedAt[traderCfg.ID] > started {
		logger.Infof("%s removed while loading, dropping this load", traderLogTag(traderCfg.ID, traderCfg.Name))
		return nil
	}
	if _, exists := tm.traders[traderCfg.ID]; exists {
		return nil
	}
	tm.recordLoad(traderCfg.ID, err)
	if err != nil {
		return err
	}

	tm.traders[traderCfg.ID] = at
	logger.Infof("✓ Trader '%s' (%s + %s/%s) loaded to memory", traderCfg.Name, agentModelCfg.Provider, accountCfg.Venue, accountCfg.Name)

	// Auto-start if trader was running before shutdown
	if traderCfg.IsRunning {
		logger.Infof("%s 🔄 Auto-starting trader (was running before shutdown)...", traderLogTag(traderCfg.ID, traderCfg.Name))
		go func(at *AutoTrader, traderName, traderID, userID string) {
			if err := at.Run(); err != nil {
				logger.Warnf("%s trader stopped with error: %v", traderLogTag(traderID, traderName), err)
				// Update database to reflect stopped state
				if st != nil {
					_ = st.Traders.UpdateStatus(userID, traderID, false)
				}
			}
		}(at, traderCfg.Name, traderCfg.ID, traderCfg.UserID)
		logger.Infof("%s ✅ Trader auto-started successfully", traderLogTag(traderCfg.ID, traderCfg.Name))
	}

	return nil
}

// buildTrader creates a trader instance from store configuration. It can call the exchange,
// so the caller must not hold the lock.
func (tm *Manager) buildTrader(traderCfg *Trader, agentModelCfg *agent.Model, accountCfg *account.Account, st *Stores) (*AutoTrader, error) {
	// The trader carries its own config
	config, err := kind.Decode(traderCfg.Kind, traderCfg.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to read the config of trader %s: %w", traderCfg.Name, err)
	}

	// Build AutoTraderConfig
	traderConfig := AutoTraderConfig{
		ID:               traderCfg.ID,
		Name:             traderCfg.Name,
		AgentModel:       agentModelCfg.Provider,
		AgentModelID:     agentModelCfg.ID,
		Venue:            accountCfg.Venue, // binance/okx
		AccountID:        accountCfg.ID,    // Account UUID (for multi-account)
		Testnet:          accountCfg.Testnet,
		Halt:             traderCfg.Halt(),
		BinanceAPIKey:    "",
		BinanceSecretKey: "",
		UseQwen:          agentModelCfg.Provider == "qwen",
		DeepSeekKey:      "",
		QwenKey:          "",
		CustomAPIURL:     agentModelCfg.CustomAPIURL,
		CustomModelName:  agentModelCfg.CustomModelName,
		ScanInterval:     time.Duration(traderCfg.ScanIntervalMinutes) * time.Minute,
		InitialBalance:   traderCfg.InitialBalance,
		IsCrossMargin:    traderCfg.IsCrossMargin(),
		ConfigRaw:        string(traderCfg.Config),
	}
	if traderCfg.ResumedAt != nil {
		traderConfig.ResumedAt = *traderCfg.ResumedAt
	}
	switch c := config.(type) {
	case *kind.AgentConfig:
		traderConfig.Agent = c
	case *kind.GridConfig:
		traderConfig.Grid = c
	}

	logger.Infof("📊 Loading trader %s: ScanIntervalMinutes=%d (from DB), ScanInterval=%v",
		traderCfg.Name, traderCfg.ScanIntervalMinutes, traderConfig.ScanInterval)

	// Set API keys based on exchange type (convert EncryptedString to string)
	switch accountCfg.Venue {
	case "binance":
		traderConfig.BinanceAPIKey = string(accountCfg.APIKey)
		traderConfig.BinanceSecretKey = string(accountCfg.SecretKey)
	case "okx":
		traderConfig.OKXAPIKey = string(accountCfg.APIKey)
		traderConfig.OKXSecretKey = string(accountCfg.SecretKey)
		traderConfig.OKXPassphrase = string(accountCfg.Passphrase)
	}

	// Set API keys based on model (convert EncryptedString to string)
	switch agentModelCfg.Provider {
	case "qwen":
		traderConfig.QwenKey = string(agentModelCfg.APIKey)
	case "deepseek":
		traderConfig.DeepSeekKey = string(agentModelCfg.APIKey)
	default:
		// For other providers (grok, openai, claude, gemini, kimi, etc.), use CustomAPIKey
		traderConfig.CustomAPIKey = string(agentModelCfg.APIKey)
	}

	// Create trader instance
	at, err := tm.newTrader(traderConfig, st, traderCfg.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to create trader: %w", err)
	}
	return at, nil
}
