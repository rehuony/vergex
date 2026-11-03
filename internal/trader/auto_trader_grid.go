package trader

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"vergex/internal/logger"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/grid"
	"vergex/internal/trader/kind"
)

// ============================================================================
// Grid Trading State Management
// ============================================================================

// GridState holds the runtime state for grid trading
type GridState struct {
	mu sync.RWMutex

	// Configuration
	Config *kind.GridConfig

	// Grid levels
	Levels []traderagent.GridLevelInfo

	// Calculated bounds
	UpperPrice  float64
	LowerPrice  float64
	GridSpacing float64

	// State flags
	IsPaused      bool
	IsInitialized bool

	// Performance tracking
	TotalProfit   float64
	TotalTrades   int
	WinningTrades int
	MaxDrawdown   float64
	PeakEquity    float64
	DailyPnL      float64 // Realized today (UTC), from the ledger

	// Order tracking
	OrderBook map[string]int // OrderID -> LevelIndex

	// Box state
	ShortBoxUpper float64
	ShortBoxLower float64
	MidBoxUpper   float64
	MidBoxLower   float64
	LongBoxUpper  float64
	LongBoxLower  float64

	// Breakout state
	BreakoutLevel        string
	BreakoutDirection    string
	BreakoutConfirmCount int

	// Position reduction (0 = normal, 50 = reduced after false breakout)
	PositionReductionPct float64

	// Current regime level
	CurrentRegimeLevel string

	// Grid direction adjustment
	CurrentDirection     grid.Direction
	DirectionChangedAt   time.Time
	DirectionChangeCount int
}

// NewGridState creates a new grid state
func NewGridState(config *kind.GridConfig) *GridState {
	return &GridState{
		Config:           config,
		Levels:           make([]traderagent.GridLevelInfo, 0),
		OrderBook:        make(map[string]int),
		CurrentDirection: grid.DirectionNeutral,
	}
}

// ============================================================================
// Breakout Detection (price vs grid boundary)
// ============================================================================

// BreakoutType represents the type of price breakout
type BreakoutType string

const (
	BreakoutNone  BreakoutType = "none"
	BreakoutUpper BreakoutType = "upper"
	BreakoutLower BreakoutType = "lower"
)

// checkBreakout detects if price has broken out of grid range
// Returns breakout type and percentage beyond boundary
func (at *AutoTrader) checkBreakout() (BreakoutType, float64) {
	gridConfig := at.config.Grid

	currentPrice, err := at.trader.GetMarketPrice(gridConfig.Symbol)
	if err != nil {
		return BreakoutNone, 0
	}

	at.gridState.mu.RLock()
	upper := at.gridState.UpperPrice
	lower := at.gridState.LowerPrice
	at.gridState.mu.RUnlock()

	if upper <= 0 || lower <= 0 {
		return BreakoutNone, 0
	}

	// Check upper breakout
	if currentPrice > upper {
		breakoutPct := (currentPrice - upper) / upper * 100
		return BreakoutUpper, breakoutPct
	}

	// Check lower breakout
	if currentPrice < lower {
		breakoutPct := (lower - currentPrice) / lower * 100
		return BreakoutLower, breakoutPct
	}

	return BreakoutNone, 0
}

// checkMaxDrawdown checks if current drawdown exceeds maximum allowed
// Returns: (exceeded bool, currentDrawdown float64)
func (at *AutoTrader) checkMaxDrawdown() (bool, float64) {
	gridConfig := at.config.Grid
	if gridConfig.MaxDrawdownPct <= 0 {
		return false, 0
	}

	// Get current equity
	balance, err := at.trader.GetBalance()
	if err != nil {
		return false, 0
	}

	currentEquity := 0.0
	if equity, ok := balance["total_equity"].(float64); ok {
		currentEquity = equity
	} else if total, ok := balance["totalWalletBalance"].(float64); ok {
		if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
			currentEquity = total + unrealized
		}
	}

	if currentEquity <= 0 {
		return false, 0
	}

	// Update peak equity
	at.gridState.mu.Lock()
	if currentEquity > at.gridState.PeakEquity {
		at.gridState.PeakEquity = currentEquity
	}
	peakEquity := at.gridState.PeakEquity
	at.gridState.mu.Unlock()

	if peakEquity <= 0 {
		return false, 0
	}

	// Calculate current drawdown
	drawdown := (peakEquity - currentEquity) / peakEquity * 100

	// Update max drawdown tracking
	at.gridState.mu.Lock()
	if drawdown > at.gridState.MaxDrawdown {
		at.gridState.MaxDrawdown = drawdown
	}
	at.gridState.mu.Unlock()

	return drawdown >= gridConfig.MaxDrawdownPct, drawdown
}

// checkDailyLossLimit checks if what the ledger has realized today (UTC) exceeds the limit
// Returns: (exceeded bool, dailyLossPct float64)
func (at *AutoTrader) checkDailyLossLimit() (bool, float64) {
	gridConfig := at.config.Grid
	if gridConfig.DailyLossLimitPct <= 0 || at.stores == nil {
		return false, 0
	}

	startOfDay := time.Now().UTC().Truncate(24 * time.Hour).UnixMilli()
	dailyPnL, err := at.stores.Positions.RealizedOfTrader(at.id, startOfDay)
	if err != nil {
		logger.Warnf("[Grid] Failed to read today's realized P/L: %v", err)
		return false, 0
	}
	at.gridState.mu.Lock()
	at.gridState.DailyPnL = dailyPnL
	at.gridState.mu.Unlock()

	// Calculate daily loss as percentage of total investment
	dailyLossPct := 0.0
	if gridConfig.TotalInvestment > 0 && dailyPnL < 0 {
		dailyLossPct = (-dailyPnL) / gridConfig.TotalInvestment * 100
	}

	return dailyLossPct >= gridConfig.DailyLossLimitPct, dailyLossPct
}

// emergencyExit cancels the grid's orders and closes its positions. It reports what it
// could not do, and is run again until it has done it all.
func (at *AutoTrader) emergencyExit(reason string) error {
	gridConfig := at.config.Grid

	logger.Errorf("[Grid] EMERGENCY EXIT: %s", reason)

	var failures []error
	// Cancel all orders
	if err := at.cancelAllGridOrders(); err != nil {
		logger.Errorf("[Grid] Failed to cancel orders in emergency: %v", err)
		failures = append(failures, err)
	}

	// Close all positions
	positions, err := at.trader.GetPositions()
	if err != nil {
		failures = append(failures, err)
	}
	for _, pos := range positions {
		if sym, ok := pos["symbol"].(string); ok && sym == gridConfig.Symbol {
			if size, ok := pos["positionAmt"].(float64); ok && size != 0 {
				var closeErr error
				if size > 0 {
					_, closeErr = at.trader.CloseLong(gridConfig.Symbol, size)
				} else {
					_, closeErr = at.trader.CloseShort(gridConfig.Symbol, -size)
				}
				if closeErr != nil {
					logger.Errorf("[Grid] Failed to close position in emergency: %v", closeErr)
					failures = append(failures, closeErr)
				}
			}
		}
	}

	// Pause grid
	at.gridState.mu.Lock()
	at.gridState.IsPaused = true
	at.gridState.mu.Unlock()

	at.exitPending.Store(len(failures) > 0)
	return errors.Join(failures...)
}

// handleBreakout handles price breakout from grid range
func (at *AutoTrader) handleBreakout(breakoutType BreakoutType, breakoutPct float64) error {
	logger.Warnf("[Grid] BREAKOUT DETECTED: %s, %.2f%% beyond boundary", breakoutType, breakoutPct)

	// If breakout exceeds 2%, pause grid and cancel orders
	if breakoutPct >= 2.0 {
		logger.Warnf("[Grid] Significant breakout (%.2f%%), pausing grid and canceling orders", breakoutPct)

		// Cancel all pending orders to prevent further losses
		if err := at.cancelAllGridOrders(); err != nil {
			logger.Errorf("[Grid] Failed to cancel orders on breakout: %v", err)
		}

		// Pause grid trading
		at.gridState.mu.Lock()
		at.gridState.IsPaused = true
		at.gridState.mu.Unlock()

		return fmt.Errorf("grid paused due to %s breakout (%.2f%%)", breakoutType, breakoutPct)
	}

	// If breakout is minor (< 2%), consider adjusting grid
	if breakoutPct >= 1.0 {
		logger.Infof("[Grid] Minor breakout (%.2f%%), considering grid adjustment", breakoutPct)
		// Let agent decide whether to adjust
	}

	return nil
}

// ============================================================================
// AutoTrader Grid Lifecycle
// ============================================================================

// InitializeGrid initializes the grid state and calculates levels
func (at *AutoTrader) InitializeGrid() error {
	if at.config.Grid == nil {
		return fmt.Errorf("grid configuration not found")
	}

	gridConfig := at.config.Grid
	at.gridState = NewGridState(gridConfig)
	at.gridState.PeakEquity = at.storedPeakEquity()

	// A crash leaves its orders behind, and a new grid knows nothing of them.
	if err := at.cancelAllGridOrders(); err != nil {
		return fmt.Errorf("failed to clear orders left from before: %w", err)
	}

	// Get current market price
	price, err := at.trader.GetMarketPrice(gridConfig.Symbol)
	if err != nil {
		return fmt.Errorf("failed to get market price: %w", err)
	}

	// Calculate grid bounds
	if gridConfig.UseATRBounds {
		// Get ATR for bound calculation
		mktData, err := traderagent.GetWithTimeframes(gridConfig.Symbol, []string{"4h"}, "4h", 20)
		if err != nil {
			logger.Warnf("Failed to get market data for ATR: %v, using default bounds", err)
			at.calculateDefaultBounds(price, gridConfig)
		} else {
			at.calculateATRBounds(price, mktData, gridConfig)
		}
	} else {
		// Use manual bounds
		at.gridState.UpperPrice = gridConfig.UpperPrice
		at.gridState.LowerPrice = gridConfig.LowerPrice
	}

	// Calculate grid spacing
	at.gridState.GridSpacing = (at.gridState.UpperPrice - at.gridState.LowerPrice) / float64(gridConfig.GridCount-1)

	// Initialize grid levels
	at.initializeGridLevels(price, gridConfig)

	at.gridState.IsInitialized = true

	// Keep grid orders aligned with the trader's configured cross/isolated mode.
	if err := at.trader.SetMarginMode(gridConfig.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Warnf("[Grid] Failed to set margin mode for %s: %v", gridConfig.Symbol, err)
	} else {
		marginMode := "cross"
		if !at.config.IsCrossMargin {
			marginMode = "isolated"
		}
		logger.Infof("[Grid] Margin mode set to %s for %s", marginMode, gridConfig.Symbol)
	}

	// CRITICAL: Set leverage on exchange before trading
	if err := at.trader.SetLeverage(gridConfig.Symbol, gridConfig.Leverage); err != nil {
		logger.Warnf("[Grid] Failed to set leverage %dx on exchange: %v", gridConfig.Leverage, err)
		// Not fatal - continue with default leverage
	} else {
		logger.Infof("[Grid] Leverage set to %dx for %s", gridConfig.Leverage, gridConfig.Symbol)
	}

	logger.Infof("[Grid] Initialized: %d levels, $%.2f - $%.2f, spacing $%.2f",
		gridConfig.GridCount, at.gridState.LowerPrice, at.gridState.UpperPrice, at.gridState.GridSpacing)

	return nil
}

// RunGridCycle executes one grid trading cycle
func (at *AutoTrader) RunGridCycle() error {
	// Check if trader is stopped (early exit to prevent trades after Stop() is called)
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		logger.Infof("[Grid] Trader is stopped, aborting grid cycle")
		return nil
	}

	at.saveGridEquity()

	// A hard stop in force holds across restarts: nothing below runs until it ends.
	if halt := at.ActiveHalt(); halt != nil {
		logger.Infof("[Grid] Halted, skipping cycle: %s", halt.Reason)
		if at.exitPending.Load() {
			return at.emergencyExit(halt.Reason)
		}
		return nil
	}

	if at.gridState == nil || !at.gridState.IsInitialized {
		if err := at.InitializeGrid(); err != nil {
			return fmt.Errorf("failed to initialize grid: %w", err)
		}
	}

	// CRITICAL: Check for breakout before executing any trades
	breakoutType, breakoutPct := at.checkBreakout()
	if breakoutType != BreakoutNone {
		if err := at.handleBreakout(breakoutType, breakoutPct); err != nil {
			return err // Grid paused due to breakout
		}
	}

	// CRITICAL: Check max drawdown
	exceeded, drawdown := at.checkMaxDrawdown()
	if exceeded {
		reason := fmt.Sprintf("max drawdown exceeded: %.2f%%", drawdown)
		at.tripHalt(reason, nil)
		return at.emergencyExit(reason)
	}

	// CRITICAL: Check daily loss limit
	dailyExceeded, dailyLossPct := at.checkDailyLossLimit()
	if dailyExceeded {
		// Open orders could only add to today's loss. Positions stay, with their stops.
		until := nextUTCDay(time.Now())
		at.tripHalt(fmt.Sprintf("daily loss limit exceeded: %.2f%%", dailyLossPct), &until)
		if err := at.cancelAllGridOrders(); err != nil {
			return fmt.Errorf("daily loss limit exceeded, but orders remain: %w", err)
		}
		return nil
	}

	// Check multi-period box breakout
	if err := at.checkBoxBreakout(); err != nil {
		logger.Infof("Box breakout check error: %v", err)
	}

	// Check for false breakout recovery
	if err := at.checkFalseBreakoutRecovery(); err != nil {
		logger.Infof("False breakout recovery check error: %v", err)
	}

	// Check if grid is paused
	at.gridState.mu.RLock()
	isPaused := at.gridState.IsPaused
	at.gridState.mu.RUnlock()
	if isPaused {
		logger.Infof("[Grid] Grid is paused, skipping cycle")
		return nil
	}

	gridConfig := at.config.Grid

	// Build grid context
	gridCtx, err := at.buildGridContext()
	if err != nil {
		return fmt.Errorf("failed to build grid context: %w", err)
	}

	// Get agent decisions
	decision, err := traderagent.GetGridDecisions(gridCtx, at.agentClient, gridConfig, at.openConversation)
	at.keepConversation(decision)
	if err != nil {
		return fmt.Errorf("failed to get grid decisions: %w", err)
	}

	// Check if trader is stopped before executing any decisions (prevent trades after Stop())
	at.isRunningMutex.RLock()
	running = at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		logger.Infof("[Grid] Trader stopped before decision execution, aborting grid cycle")
		return nil
	}

	// Execute decisions
	actions := make([]DecisionAction, 0, len(decision.Decisions))
	for i := range decision.Decisions {
		d := &decision.Decisions[i]
		// Check if trader is still running before each decision
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()
		if !running {
			logger.Infof("[Grid] Trader stopped, skipping remaining %d decisions", len(decision.Decisions)-i)
			break
		}

		err := at.executeGridDecision(d)
		if err != nil {
			logger.Warnf("[Grid] Failed to execute decision %s: %v", d.Action, err)
		}
		actions = append(actions, gridDecisionAction(d, err))
	}

	// Sync state with exchange
	at.syncGridState()

	// Save decision record
	at.saveGridDecisionRecord(decision, actions)

	return nil
}

// buildGridContext builds the context for agent grid decisions
func (at *AutoTrader) buildGridContext() (*traderagent.GridContext, error) {
	gridConfig := at.config.Grid

	// Get market data
	mktData, err := traderagent.GetWithTimeframes(gridConfig.Symbol, []string{"5m", "4h"}, "5m", 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data: %w", err)
	}

	// Build base context from market data
	ctx := traderagent.BuildGridContextFromMarketData(mktData, gridConfig)

	// Add grid state
	at.gridState.mu.RLock()
	ctx.Levels = at.gridState.Levels
	ctx.UpperPrice = at.gridState.UpperPrice
	ctx.LowerPrice = at.gridState.LowerPrice
	ctx.GridSpacing = at.gridState.GridSpacing
	ctx.IsPaused = at.gridState.IsPaused
	ctx.TotalProfit = at.gridState.TotalProfit
	ctx.TotalTrades = at.gridState.TotalTrades
	ctx.WinningTrades = at.gridState.WinningTrades
	ctx.MaxDrawdown = at.gridState.MaxDrawdown
	ctx.DailyPnL = at.gridState.DailyPnL

	// Count active orders and filled levels
	for _, level := range at.gridState.Levels {
		if level.State == "pending" {
			ctx.ActiveOrderCount++
		} else if level.State == "filled" {
			ctx.FilledLevelCount++
		}
	}
	at.gridState.mu.RUnlock()

	// Get account info
	balance, err := at.trader.GetBalance()
	if err == nil {
		if equity, ok := balance["total_equity"].(float64); ok {
			ctx.TotalEquity = equity
		}
		if available, ok := balance["availableBalance"].(float64); ok {
			ctx.AvailableBalance = available
		}
		if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
			ctx.UnrealizedPnL = unrealized
		}
	}

	// Get current position
	positions, err := at.trader.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if sym, ok := pos["symbol"].(string); ok && sym == gridConfig.Symbol {
				if size, ok := pos["positionAmt"].(float64); ok {
					ctx.CurrentPosition = size
				}
			}
		}
	}

	return ctx, nil
}

// executeGridDecision executes a single grid decision
func (at *AutoTrader) executeGridDecision(d *traderagent.Decision) error {
	switch d.Action {
	case "place_buy_limit":
		return at.placeGridLimitOrder(d, "BUY")
	case "place_sell_limit":
		return at.placeGridLimitOrder(d, "SELL")
	case "cancel_order":
		return at.cancelGridOrder(d)
	case "cancel_all_orders":
		return at.cancelAllGridOrders()
	case "pause_grid":
		return at.pauseGrid(d.Reasoning)
	case "resume_grid":
		return at.resumeGrid()
	case "adjust_grid":
		return at.adjustGrid(d)
	case "hold":
		logger.Infof("[Grid] Holding current state: %s", d.Reasoning)
		return nil
	// Support standard actions for closing positions
	case "close_long":
		_, err := at.trader.CloseLong(d.Symbol, d.Quantity)
		return err
	case "close_short":
		_, err := at.trader.CloseShort(d.Symbol, d.Quantity)
		return err
	default:
		logger.Warnf("[Grid] Unknown action: %s", d.Action)
		return nil
	}
}

// IsGrid returns true for a grid trader
func (at *AutoTrader) IsGrid() bool {
	return at.config.Grid != nil
}

// gridDecisionAction is how a decision turned out, for the record. A rule stopping it is
// not a failure: nothing reached the exchange.
func gridDecisionAction(d *traderagent.Decision, err error) DecisionAction {
	action := DecisionAction{
		Action:     d.Action,
		Symbol:     d.Symbol,
		Quantity:   d.Quantity,
		Leverage:   d.Leverage,
		Price:      d.Price,
		StopLoss:   d.StopLoss,
		TakeProfit: d.TakeProfit,
		Confidence: d.Confidence,
		Reasoning:  d.Reasoning,
		Timestamp:  time.Now().UTC(),
		Success:    err == nil,
	}
	if err != nil {
		action.Error = err.Error()
		action.Blocked = errors.As(err, new(riskBlock))
	}
	return action
}

// saveGridDecisionRecord saves the grid decision and what became of it to database
func (at *AutoTrader) saveGridDecisionRecord(decision *traderagent.FullDecision, actions []DecisionAction) {
	record := &DecisionRecord{
		Timestamp:       time.Now().UTC(),
		ConversationID:  decision.ConversationID,
		AgentDurationMs: decision.AgentDurationMs,
		Success:         true,
		Decisions:       actions,
	}
	record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("Grid cycle completed with %d decisions", len(actions)))
	if err := at.saveDecision(record); err != nil {
		logger.Warnf("[Grid] Failed to save decision record: %v", err)
	}
}

// GridRiskInfo contains risk information for frontend display
type GridRiskInfo struct {
	CurrentLeverage     int     `json:"current_leverage"`
	EffectiveLeverage   float64 `json:"effective_leverage"`
	RecommendedLeverage int     `json:"recommended_leverage"`

	CurrentPosition float64 `json:"current_position"`
	MaxPosition     float64 `json:"max_position"`
	PositionPercent float64 `json:"position_percent"`

	LiquidationPrice    float64 `json:"liquidation_price"`
	LiquidationDistance float64 `json:"liquidation_distance"`

	RegimeLevel string `json:"regime_level"`

	ShortBoxUpper float64 `json:"short_box_upper"`
	ShortBoxLower float64 `json:"short_box_lower"`
	MidBoxUpper   float64 `json:"mid_box_upper"`
	MidBoxLower   float64 `json:"mid_box_lower"`
	LongBoxUpper  float64 `json:"long_box_upper"`
	LongBoxLower  float64 `json:"long_box_lower"`
	CurrentPrice  float64 `json:"current_price"`

	BreakoutLevel     string `json:"breakout_level"`
	BreakoutDirection string `json:"breakout_direction"`

	// Grid direction
	CurrentGridDirection  string `json:"current_grid_direction"`
	DirectionChangeCount  int    `json:"direction_change_count"`
	EnableDirectionAdjust bool   `json:"enable_direction_adjust"`
}

// saveGridEquity stores what the account holds each cycle, as an agent trader does.
func (at *AutoTrader) saveGridEquity() {
	if at.stores == nil {
		return
	}
	balance, err := at.trader.GetBalance()
	if err != nil {
		logger.Warnf("[Grid] Failed to read balance for equity snapshot: %v", err)
		return
	}
	wallet, _ := balance["totalWalletBalance"].(float64)
	unrealized, _ := balance["totalUnrealizedProfit"].(float64)
	equity, ok := balance["totalEquity"].(float64)
	if !ok || equity <= 0 {
		equity = wallet + unrealized
	}
	if equity <= 0 {
		return
	}
	snapshot := &EquitySnapshot{TraderID: at.id, TotalEquity: equity, UnrealizedPnL: unrealized}
	if err := at.stores.Equity.Save(snapshot); err != nil {
		logger.Warnf("[Grid] Failed to save equity snapshot: %v", err)
	}
}
