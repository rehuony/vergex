package trader

import (
	"fmt"
	"sync"
	"time"
	"vergex/internal/agent"
	_ "vergex/internal/agent/provider"
	"vergex/internal/exchange/binance"
	"vergex/internal/exchange/okx"
	"vergex/internal/logger"
	"vergex/internal/store"
	traderagent "vergex/internal/trader/agent"
)

func (at *AutoTrader) logTag() string {
	if at == nil {
		return "[trader_id=unknown]"
	}
	if at.name != "" {
		return fmt.Sprintf("[trader_id=%s trader_name=%s]", at.id, at.name)
	}
	return fmt.Sprintf("[trader_id=%s]", at.id)
}

func (at *AutoTrader) logInfof(format string, args ...interface{}) {
	values := append([]interface{}{at.logTag()}, args...)
	logger.Infof("%s "+format, values...)
}

func (at *AutoTrader) logWarnf(format string, args ...interface{}) {
	values := append([]interface{}{at.logTag()}, args...)
	logger.Warnf("%s "+format, values...)
}

// extractInitialBalance prefers an exchange's explicit mark-to-market equity.
// Wallet-balance fields remain as compatibility fallbacks for integrations
// that do not expose total equity separately.
func extractInitialBalance(account map[string]interface{}) float64 {
	balanceKeys := []string{
		"totalEquity",
		"total_equity",
		"totalWalletBalance",
		"wallet_balance",
		"totalEq",
		"balance",
	}
	for _, key := range balanceKeys {
		if balance, ok := account[key].(float64); ok && balance > 0 {
			return balance
		}
	}
	return 0
}

func (at *AutoTrader) logErrorf(format string, args ...interface{}) {
	values := append([]interface{}{at.logTag()}, args...)
	logger.Errorf("%s "+format, values...)
}

// AutoTraderConfig auto trading configuration (simplified version - agent makes all decisions)
type AutoTraderConfig struct {
	// Trader identification
	ID         string // Trader unique identifier (for log directory, etc.)
	Name       string // Trader display name
	StrategyID string // Associated strategy ID used to refresh live strategy config
	AgentModel string // model: "qwen" or "deepseek"
	// AgentModelID is the model's row, which the trader's conversations point at.
	AgentModelID string

	// Trading platform selection
	Exchange   string // Exchange type: "binance" or "okx"
	ExchangeID string // Exchange account UUID (for multi-account support)

	// Binance API configuration
	BinanceAPIKey    string
	BinanceSecretKey string

	// OKX API configuration
	OKXAPIKey     string
	OKXSecretKey  string
	OKXPassphrase string

	// Agent configuration
	UseQwen     bool
	DeepSeekKey string
	QwenKey     string

	// Custom model API configuration
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// Scan configuration
	ScanInterval time.Duration // Scan interval (recommended 15 minutes)

	// Account configuration
	InitialBalance float64 // Initial balance (for P&L calculation, must be set manually)

	// Risk control (only as hints, agent can make autonomous decisions)
	MaxDailyLoss    float64       // Maximum daily loss percentage (hint)
	MaxDrawdown     float64       // Maximum drawdown percentage (hint)
	StopTradingTime time.Duration // Pause duration after risk control triggers

	// Position mode
	IsCrossMargin bool // true=cross margin mode, false=isolated margin mode

	// Strategy configuration (use complete strategy config)
	StrategyConfig    *store.StrategyConfig // Strategy configuration (includes coin sources, indicators, risk control, prompts, etc.)
	StrategyConfigRaw string                // Raw strategy config JSON from DB, used to detect live edits
}

// AutoTrader automatic trader
type AutoTrader struct {
	id                       string // Trader unique identifier
	name                     string // Trader display name
	agentModel               string // model name
	exchange                 string // Trading platform type (binance/okx)
	exchangeID               string // Exchange account UUID
	config                   AutoTraderConfig
	trader                   Trader // Use Trader interface (supports multiple platforms)
	agentClient              agent.Client
	store                    *store.Store                // Data storage (decision records, etc.)
	strategyEngine           *traderagent.StrategyEngine // Strategy engine (uses strategy configuration)
	cycleNumber              int                         // Current cycle number
	initialBalance           float64
	dailyPnL                 float64
	lastResetTime            time.Time
	stopUntil                time.Time
	isRunning                bool
	isRunningMutex           sync.RWMutex       // Mutex to protect isRunning flag
	startTime                time.Time          // System start time
	callCount                int                // model call count
	positionFirstSeenTime    map[string]int64   // Position first seen time (symbol_side -> timestamp in milliseconds)
	stopMonitorCh            chan struct{}      // Used to stop monitoring goroutine
	monitorWg                sync.WaitGroup     // Used to wait for monitoring goroutine to finish
	peakPnLCache             map[string]float64 // Peak profit cache (symbol -> peak P&L percentage)
	peakPnLCacheMutex        sync.RWMutex       // Cache read-write lock
	lastBalanceSyncTime      time.Time          // Last balance sync time
	userID                   string             // User ID
	gridState                *GridState         // Grid trading state (only used when StrategyType == "grid_trading")
	consecutiveAgentFailures int                // Consecutive model call failures
	runtimeHealthMu          sync.RWMutex       // Guards safe mode + trading authorization state (loop writes, API reads)
	tradingBlocked           bool
	tradingError             string
	tradingCheckedAt         time.Time
	safeMode                 bool   // Safe mode: no new positions, protect existing ones
	safeModeReason           string // Why safe mode was activated
}

// NewAutoTrader creates an automatic trader
// st parameter is used to store decision records to database
func NewAutoTrader(config AutoTraderConfig, st *store.Store, userID string) (*AutoTrader, error) {
	// Check before constructing exchange clients; legacy constructors may change
	// account settings even before their Run loop starts.
	if st != nil && config.ExchangeID != "" {
		ex, err := st.Exchange().GetByID(userID, config.ExchangeID)
		if err != nil {
			return nil, fmt.Errorf("cannot verify planning account ownership: %w", err)
		}
		if leased, err := st.Exchange().Leased(ex.ID); err != nil || leased {
			return nil, fmt.Errorf("account has an unresolved planning lease or lease lookup failed")
		}
	}

	// Set default values
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.AgentModel == "" {
		if config.UseQwen {
			config.AgentModel = "qwen"
		} else {
			config.AgentModel = "deepseek"
		}
	}

	// Initialize model client based on provider
	var agentClient agent.Client
	agentModel := config.AgentModel
	if config.UseQwen && agentModel == "" {
		agentModel = "qwen"
	}

	// Resolve API key (provider-specific overrides)
	apiKey := config.CustomAPIKey
	customURL := config.CustomAPIURL
	switch agentModel {
	case "qwen":
		if config.QwenKey != "" {
			apiKey = config.QwenKey
		}
	case "deepseek", "":
		if config.DeepSeekKey != "" {
			apiKey = config.DeepSeekKey
		}
	}

	// Create client via registry (covers all registered providers)
	if agentModel == "custom" {
		agentClient = agent.New()
	} else if agentModel == "" {
		agentModel = "deepseek"
		agentClient = agent.NewClientByProvider(agentModel)
	} else {
		agentClient = agent.NewClientByProvider(agentModel)
	}
	if agentClient == nil {
		agentClient = agent.New()
	}

	agentClient.SetAPIKey(apiKey, customURL, config.CustomModelName)
	// Attribute calls (agent.calls) to this trader
	agent.TagUsage(agentClient, agent.FeatureTrader, config.ID)
	logger.Infof("🤖 [%s] Using %s agent", config.Name, agentModel)

	if config.CustomAPIURL != "" || config.CustomModelName != "" {
		logger.Infof("🔧 [%s] Custom config - URL: %s, Model: %s", config.Name, config.CustomAPIURL, config.CustomModelName)
	}

	// Set default trading platform
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// Create corresponding trader based on configuration
	var trader Trader

	// Record position mode (general)
	marginModeStr := "Cross Margin"
	if !config.IsCrossMargin {
		marginModeStr = "Isolated Margin"
	}
	logger.Infof("📊 [%s] Position mode: %s", config.Name, marginModeStr)

	switch config.Exchange {
	case "binance":
		logger.Infof("🏦 [%s] Using Binance Futures trading", config.Name)
		trader = binance.NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey)
	case "okx":
		logger.Infof("🏦 [%s] Using OKX Futures trading", config.Name)
		trader = okx.NewOKXTrader(config.OKXAPIKey, config.OKXSecretKey, config.OKXPassphrase)
	default:
		return nil, fmt.Errorf("unsupported trading platform: %s", config.Exchange)
	}

	// Validate initial balance configuration, auto-fetch from exchange if 0
	if config.InitialBalance <= 0 {
		logger.Infof("📊 [%s] Initial balance not set, attempting to fetch current balance from exchange...", config.Name)
		account, err := trader.GetBalance()
		if err != nil {
			return nil, fmt.Errorf("initial balance not set and unable to fetch balance from exchange: %w", err)
		}
		// Try multiple balance field names (different exchanges return different formats).
		foundBalance := extractInitialBalance(account)
		if foundBalance > 0 {
			config.InitialBalance = foundBalance
			logger.Infof("✓ [%s] Auto-fetched initial balance: %.2f USDT", config.Name, foundBalance)
			// Save to database so it persists across restarts
			if st != nil {
				if err := st.Trader().UpdateInitialBalance(userID, config.ID, foundBalance); err != nil {
					logger.Infof("⚠️  [%s] Failed to save initial balance to database: %v", config.Name, err)
				} else {
					logger.Infof("✓ [%s] Initial balance saved to database", config.Name)
				}
			}
		} else {
			return nil, fmt.Errorf("initial balance must be greater than 0, please set InitialBalance in config or ensure exchange account has balance")
		}
	}

	// Get last cycle number (for recovery)
	var cycleNumber int
	if st != nil {
		cycleNumber, _ = st.Decision().GetLastCycleNumber(config.ID)
		logger.Infof("📊 [%s] Decision records will be stored to database", config.Name)
	}

	// Create strategy engine (must have strategy config)
	if config.StrategyConfig == nil {
		return nil, fmt.Errorf("[%s] strategy not configured", config.Name)
	}
	strategyEngine := traderagent.NewStrategyEngine(config.StrategyConfig).WithVenue(config.Exchange)
	logger.Infof("✓ [%s] Using strategy engine (strategy configuration loaded)", config.Name)

	return &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		agentModel:            config.AgentModel,
		exchange:              config.Exchange,
		exchangeID:            config.ExchangeID,
		config:                config,
		trader:                trader,
		agentClient:           agentClient,
		store:                 st,
		strategyEngine:        strategyEngine,
		cycleNumber:           cycleNumber,
		initialBalance:        config.InitialBalance,
		lastResetTime:         time.Now(),
		startTime:             time.Now(),
		callCount:             0,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		stopMonitorCh:         make(chan struct{}),
		monitorWg:             sync.WaitGroup{},
		peakPnLCache:          make(map[string]float64),
		peakPnLCacheMutex:     sync.RWMutex{},
		lastBalanceSyncTime:   time.Now(),
		userID:                userID,
	}, nil
}

func (at *AutoTrader) reloadStrategyConfigIfChanged() error {
	if at == nil || at.store == nil || at.config.StrategyID == "" {
		return nil
	}

	strategy, err := at.store.Strategy().Get(at.userID, at.config.StrategyID)
	if err != nil {
		return fmt.Errorf("failed to load strategy %s: %w", at.config.StrategyID, err)
	}

	if at.strategyEngine != nil && strategy.Config == at.config.StrategyConfigRaw {
		return nil
	}

	strategyConfig, err := strategy.ParseConfig()
	if err != nil {
		return fmt.Errorf("failed to parse strategy %s: %w", strategy.Name, err)
	}
	strategyConfig.ClampLimits()

	// NOTE: this used to hardcode the Autopilot book shape (6 positions ×
	// equity×1.2 notional), silently overriding whatever the user configured in
	// their strategy. Sizing now comes from the strategy's own RiskControl —
	// ClampLimits above bounds it (ratio 0.5–10, leverage caps), and the
	// margin auto-reduce at order time keeps the book solvent.

	at.config.StrategyConfig = strategyConfig
	at.config.StrategyConfigRaw = strategy.Config
	at.strategyEngine = traderagent.NewStrategyEngine(strategyConfig).WithVenue(at.exchange)
	at.logInfof("🔄 Strategy config refreshed from DB: %s", strategy.Name)
	return nil
}

// Run runs the automatic trading main loop
func (at *AutoTrader) Run() error {
	// All launch paths (API, restoration and manager) respect a planning lease.
	var unlock func()
	if at.store != nil && at.exchangeID != "" {
		ex, err := at.store.Exchange().GetByID(at.userID, at.exchangeID)
		if err != nil {
			return fmt.Errorf("cannot verify planning account ownership: %w", err)
		}
		unlock = store.LockAccount(ex.ID)
		if leased, err := at.store.Exchange().Leased(ex.ID); err != nil || leased {
			unlock()
			return fmt.Errorf("account has an unresolved planning lease or lease lookup failed")
		}
	}

	at.isRunningMutex.Lock()
	at.isRunning = true
	at.isRunningMutex.Unlock()
	if unlock != nil {
		unlock()
	}

	at.stopMonitorCh = make(chan struct{})
	at.startTime = time.Now()

	logger.Info("🚀 Agent-driven automatic trading system started")
	at.logInfof("💰 Initial balance: %.2f USDT", at.initialBalance)
	at.logInfof("⚙️  Scan interval: %v", at.config.ScanInterval)
	logger.Info("🤖 The agent will make full decisions on leverage, position size, stop loss/take profit, etc.")

	at.monitorWg.Add(1)
	defer at.monitorWg.Done()

	// Start drawdown monitoring
	at.startDrawdownMonitor()

	// Check if this is a grid trading strategy
	isGridStrategy := at.IsGridStrategy()
	if isGridStrategy {
		at.logInfof("🔲 Grid trading strategy detected, initializing grid...")
		if err := at.InitializeGrid(); err != nil {
			at.logErrorf("❌ Failed to initialize grid: %v", err)
			return fmt.Errorf("grid initialization failed: %w", err)
		}
	}

	// Execute immediately on first run
	at.logInfof("▶️ Running first trading cycle immediately; next cycle starts after %v", at.config.ScanInterval)
	if isGridStrategy {
		if err := at.RunGridCycle(); err != nil {
			at.logErrorf("❌ Grid execution failed: %v", err)
		}
	} else {
		if err := at.runCycle(); err != nil {
			at.logErrorf("❌ Execution failed: %v", err)
		}
	}

	ticker := time.NewTicker(at.config.ScanInterval)
	defer ticker.Stop()

	for {
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()

		if !running {
			break
		}

		select {
		case <-ticker.C:
			if isGridStrategy {
				if err := at.RunGridCycle(); err != nil {
					at.logErrorf("❌ Grid execution failed: %v", err)
				}
			} else {
				if err := at.runCycle(); err != nil {
					at.logErrorf("❌ Execution failed: %v", err)
				}
			}
		case <-at.stopMonitorCh:
			at.logInfof("⏹ Stop signal received, exiting automatic trading main loop")
			return nil
		}
	}

	return nil
}

// Stop stops the automatic trading
func (at *AutoTrader) Stop() {
	at.isRunningMutex.Lock()
	if !at.isRunning {
		at.isRunningMutex.Unlock()
		return
	}
	at.isRunning = false
	at.isRunningMutex.Unlock()

	close(at.stopMonitorCh) // Notify monitoring goroutine to stop
	at.monitorWg.Wait()     // Wait for monitoring goroutine to finish
	logger.Info("⏹ Automatic trading system stopped")
}

// GetID gets trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetInitialBalance returns the account baseline used for performance metrics
// (e.g. the drawdown equity curve).
func (at *AutoTrader) GetInitialBalance() float64 {
	return at.initialBalance
}

// GetUnderlyingTrader returns the underlying Trader interface implementation
// This is used by grid trading and other components that need direct exchange access
func (at *AutoTrader) GetUnderlyingTrader() Trader {
	return at.trader
}

// GetName gets trader name
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAgentModel gets model
func (at *AutoTrader) GetAgentModel() string {
	return at.agentModel
}

// GetExchange gets exchange
func (at *AutoTrader) GetExchange() string {
	return at.exchange
}

// GetCandidateCoins returns the current candidate coin set from the trader's strategy engine.
func (at *AutoTrader) GetCandidateCoins() ([]traderagent.CandidateCoin, error) {
	if at.strategyEngine == nil {
		return nil, fmt.Errorf("strategy engine not configured")
	}
	return at.strategyEngine.GetCandidateCoins()
}

// GetStrategyConfig returns the current strategy config used by the trader.
func (at *AutoTrader) GetStrategyConfig() *store.StrategyConfig {
	if at.strategyEngine == nil {
		return at.config.StrategyConfig
	}
	return at.strategyEngine.GetConfig()
}

// GetStore gets data store (for external access to decision records, etc.)
func (at *AutoTrader) GetStore() *store.Store {
	return at.store
}

// calculatePnLPercentage calculates P&L percentage (based on margin, automatically considers leverage)
// Return rate = Unrealized P&L / Margin x 100%
func calculatePnLPercentage(unrealizedPnl, marginUsed float64) float64 {
	if marginUsed > 0 {
		return (unrealizedPnl / marginUsed) * 100
	}
	return 0.0
}
