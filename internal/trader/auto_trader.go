package trader

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"vergex/internal/account"
	"vergex/internal/agent"
	_ "vergex/internal/agent/provider"
	"vergex/internal/logger"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/kind"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
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
// Wallet-balance fields are the fallback for an account that does not report
// total equity separately.
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
	AgentModel string // model: "qwen" or "deepseek"
	// AgentModelID is the model's row, which the trader's conversations point at.
	AgentModelID string

	// Trading platform selection
	Venue     string // "binance" or "okx"
	AccountID string // Account UUID (for multi-account support)
	Testnet   bool   // The account is on the venue's test environment

	// A hard stop in force when the trader was loaded, and when one last ended by hand
	Halt      *Halt
	ResumedAt time.Time

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
	BaseURL      string
	CustomAPIKey string
	ModelName    string

	// Scan configuration
	ScanInterval time.Duration // Scan interval (recommended 15 minutes)

	// Account configuration
	InitialBalance float64 // Initial balance (for P&L calculation, must be set manually)

	// Risk control (only as hints, agent can make autonomous decisions)

	// Position mode
	IsCrossMargin bool // true=cross margin mode, false=isolated margin mode

	// What its kind decides: one of these is set.
	Agent *kind.AgentConfig // Coin sources, indicators, risk control, prompts
	Grid  *kind.GridConfig
	// ConfigRaw is the stored config, used to detect live edits
	ConfigRaw string
}

// AutoTrader automatic trader
type AutoTrader struct {
	id                       string // Trader unique identifier
	name                     string // Trader display name
	agentModel               string // model name
	venue                    string // Trading platform type (binance/okx)
	accountID                string // Account UUID
	config                   AutoTraderConfig
	trader                   account.Trading // Use the Trading interface (supports multiple platforms)
	agentClient              agent.Client
	stores                   *Stores             // Data storage (decision records, etc.)
	engine                   *traderagent.Engine // Builds an agent trader's prompts; nil on a grid trader
	cycleNumber              int                 // Current cycle number
	initialBalance           float64
	halt                     *Halt       // Hard stop in force, under runtimeHealthMu
	resumedAt                time.Time   // When a halt last ended by hand, under runtimeHealthMu
	exitPending              atomic.Bool // A grid's emergency exit has not finished yet
	stopErr                  error       // What the last Stop left undone, under runtimeHealthMu
	isRunning                bool
	isRunningMutex           sync.RWMutex       // Mutex to protect isRunning flag
	startTime                time.Time          // System start time
	callCount                int                // model call count
	positionFirstSeenTime    map[string]int64   // Position first seen time (symbol_side -> timestamp in milliseconds)
	stopMonitorCh            chan struct{}      // Used to stop monitoring goroutine
	monitorWg                sync.WaitGroup     // Used to wait for monitoring goroutine to finish
	peakPnLCache             map[string]float64 // Peak profit cache (symbol -> peak P&L percentage)
	peakPnLCacheMutex        sync.RWMutex       // Cache read-write lock
	peakPnLSaved             map[string]float64 // The peak last written to the store, under the same lock
	lastBalanceSyncTime      time.Time          // Last balance sync time
	userID                   string             // User ID
	gridState                *GridState         // Grid trading state (only used by a grid trader)
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
func NewAutoTrader(config AutoTraderConfig, st *Stores, userID string) (*AutoTrader, error) {
	// Check before constructing exchange clients: a constructor may change account
	// settings even before the Run loop starts.
	if st != nil && config.AccountID != "" {
		ex, err := st.Accounts.GetByID(userID, config.AccountID)
		if err != nil {
			return nil, fmt.Errorf("cannot verify planning account ownership: %w", err)
		}
		if leased, err := st.Accounts.Leased(ex.ID); err != nil || leased {
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
	customURL := config.BaseURL
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
		agentClient = agent.NewClient()
	} else if agentModel == "" {
		agentModel = "deepseek"
		agentClient = agent.NewClientByProvider(agentModel)
	} else {
		agentClient = agent.NewClientByProvider(agentModel)
	}
	if agentClient == nil {
		agentClient = agent.NewClient()
	}

	agentClient.SetAPIKey(apiKey, customURL, config.ModelName)
	// Attribute calls (agent.calls) to this trader
	agent.TagUsage(agentClient, agent.FeatureTrader, config.ID)
	logger.Infof("🤖 [%s] Using %s agent", config.Name, agentModel)

	if config.BaseURL != "" || config.ModelName != "" {
		logger.Infof("🔧 [%s] Custom config - URL: %s, Model: %s", config.Name, config.BaseURL, config.ModelName)
	}

	// Set default trading platform
	if config.Venue == "" {
		config.Venue = "binance"
	}

	// Create corresponding trader based on configuration
	var trader account.Trading

	// Record position mode (general)
	marginModeStr := "Cross Margin"
	if !config.IsCrossMargin {
		marginModeStr = "Isolated Margin"
	}
	logger.Infof("📊 [%s] Position mode: %s", config.Name, marginModeStr)

	switch config.Venue {
	case "binance":
		logger.Infof("🏦 [%s] Using Binance Futures trading", config.Name)
		trader = binance.NewAccount(config.BinanceAPIKey, config.BinanceSecretKey, config.Testnet)
	case "okx":
		logger.Infof("🏦 [%s] Using OKX Futures trading", config.Name)
		trader = okx.NewAccount(config.OKXAPIKey, config.OKXSecretKey, config.OKXPassphrase, config.Testnet)
	default:
		return nil, fmt.Errorf("unsupported trading platform: %s", config.Venue)
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
				if err := st.Traders.UpdateInitialBalance(userID, config.ID, foundBalance); err != nil {
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
		cycleNumber, _ = st.Decisions.LastSeq(config.ID)
		logger.Infof("📊 [%s] Decision records will be stored to database", config.Name)
	}

	// An agent trader builds its prompts from its config
	var engine *traderagent.Engine
	switch {
	case config.Agent != nil:
		engine = traderagent.NewEngine(config.Agent).WithVenue(config.Venue)
	case config.Grid == nil:
		return nil, fmt.Errorf("[%s] config not set", config.Name)
	}

	return &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		agentModel:            config.AgentModel,
		venue:                 config.Venue,
		accountID:             config.AccountID,
		config:                config,
		trader:                trader,
		agentClient:           agentClient,
		stores:                st,
		engine:                engine,
		cycleNumber:           cycleNumber,
		initialBalance:        config.InitialBalance,
		startTime:             time.Now(),
		callCount:             0,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		stopMonitorCh:         make(chan struct{}),
		monitorWg:             sync.WaitGroup{},
		peakPnLCache:          make(map[string]float64),
		peakPnLCacheMutex:     sync.RWMutex{},
		peakPnLSaved:          make(map[string]float64),
		lastBalanceSyncTime:   time.Now(),
		userID:                userID,
		halt:                  config.Halt,
		resumedAt:             config.ResumedAt,
	}, nil
}

// reloadConfigIfChanged rereads an agent trader's own row, so an edit takes effect on
// its next cycle. Sizing comes from the config's RiskControl, which Decode bounds
// (ratio 0.5–10, leverage caps), and the margin auto-reduce at order time keeps the book
// solvent.
func (at *AutoTrader) reloadConfigIfChanged() error {
	if at == nil || at.stores == nil || at.config.ID == "" {
		return nil
	}

	row, err := at.stores.Traders.Get(at.userID, at.config.ID)
	if err != nil {
		return fmt.Errorf("failed to load trader %s: %w", at.config.ID, err)
	}

	if at.engine != nil && string(row.Config) == at.config.ConfigRaw {
		return nil
	}

	decoded, err := kind.Decode(row.Kind, row.Config)
	if err != nil {
		return fmt.Errorf("failed to read the config of %s: %w", row.Name, err)
	}
	config, ok := decoded.(*kind.AgentConfig)
	if !ok {
		return fmt.Errorf("%s is not an agent trader", row.Name)
	}

	at.config.Agent = config
	at.config.ConfigRaw = string(row.Config)
	at.engine = traderagent.NewEngine(config).WithVenue(at.venue)
	at.logInfof("🔄 Config refreshed from DB: %s", row.Name)
	return nil
}

// Run runs the automatic trading main loop
func (at *AutoTrader) Run() error {
	// All launch paths (API, restoration and manager) respect a planning lease.
	var unlock func()
	if at.stores != nil && at.accountID != "" {
		ex, err := at.stores.Accounts.GetByID(at.userID, at.accountID)
		if err != nil {
			return fmt.Errorf("cannot verify planning account ownership: %w", err)
		}
		unlock = account.Lock(ex.ID)
		if leased, err := at.stores.Accounts.Leased(ex.ID); err != nil || leased {
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

	// Start drawdown monitoring, from what it knew before any restart
	at.loadStoredPeaks()
	at.startDrawdownMonitor()

	// Check if this is a grid trader
	isGrid := at.IsGrid()
	if isGrid {
		at.logInfof("🔲 Grid trader, initializing grid...")
		if err := at.InitializeGrid(); err != nil {
			at.logErrorf("❌ Failed to initialize grid: %v", err)
			return fmt.Errorf("grid initialization failed: %w", err)
		}
	}

	// Execute immediately on first run
	at.logInfof("▶️ Running first trading cycle immediately; next cycle starts after %v", at.config.ScanInterval)
	if isGrid {
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
			if isGrid {
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
	at.cancelGridOrdersOnStop()
	logger.Info("⏹ Automatic trading system stopped")
}

// cancelGridOrdersOnStop takes a grid's resting orders with it: once it stops, nothing
// watches what they would open. Positions stay, as they do for any trader.
func (at *AutoTrader) cancelGridOrdersOnStop() {
	if at.config.Grid == nil || at.gridState == nil {
		return
	}
	err := at.cancelAllGridOrders()
	if err != nil {
		logger.Errorf("[Grid] Stopped, but its orders may still be on the exchange: %v", err)
	}
	at.runtimeHealthMu.Lock()
	at.stopErr = err
	at.runtimeHealthMu.Unlock()
}

// StopError says what stopping left undone, such as a grid's orders it could not cancel.
func (at *AutoTrader) StopError() error {
	at.runtimeHealthMu.RLock()
	defer at.runtimeHealthMu.RUnlock()
	return at.stopErr
}

// GetInitialBalance returns the account baseline used for performance metrics
// (e.g. the drawdown equity curve).
func (at *AutoTrader) GetInitialBalance() float64 {
	return at.initialBalance
}

// GetName gets trader name
func (at *AutoTrader) GetName() string {
	return at.name
}

// calculatePnLPercentage calculates P&L percentage (based on margin, automatically considers leverage)
// Return rate = Unrealized P&L / Margin x 100%
func calculatePnLPercentage(unrealizedPnl, marginUsed float64) float64 {
	if marginUsed > 0 {
		return (unrealizedPnl / marginUsed) * 100
	}
	return 0.0
}
