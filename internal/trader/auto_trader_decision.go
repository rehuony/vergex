package trader

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	"math"
	"time"
	"vergex/internal/exchange/okx"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/trader/agent"
)

// saveEquitySnapshot saves equity snapshot independently (for drawing profit curve, decoupled from agent decision)
func (at *AutoTrader) saveEquitySnapshot(ctx *agent.Context) {
	if at.store == nil || ctx == nil {
		return
	}

	snapshot := &store.EquitySnapshot{
		TraderID:         at.id,
		Timestamp:        time.Now().UTC(),
		TotalEquity:      ctx.Account.TotalEquity,
		Balance:          ctx.Account.TotalEquity - ctx.Account.UnrealizedPnL,
		AvailableBalance: ctx.Account.AvailableBalance,
		UnrealizedPnL:    ctx.Account.UnrealizedPnL,
		PositionCount:    ctx.Account.PositionCount,
		MarginUsedPct:    ctx.Account.MarginUsedPct,
	}

	if err := at.store.Equity().Save(snapshot); err != nil {
		logger.Infof("⚠️ Failed to save equity snapshot: %v", err)
	}
}

// cycleSaveWait bounds saving a cycle. Its orders are already on the exchange, and a
// cycle's record points at its conversation, so a stuck conversation can hold this up.
const cycleSaveWait = 5 * time.Second

// saveDecision saves agent decision log to database (only records agent input/output, for debugging)
func (at *AutoTrader) saveDecision(record *store.DecisionRecord) error {
	summarizeDecisionExecution(record)
	if at.store == nil {
		return nil
	}

	at.cycleNumber++
	record.CycleNumber = at.cycleNumber
	record.TraderID = at.id

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}

	ctx, cancel := context.WithTimeout(context.Background(), cycleSaveWait)
	defer cancel()
	if err := at.store.Decision().WithContext(ctx).LogDecision(record); err != nil {
		logger.Infof("⚠️ Failed to save decision record: %v", err)
		return err
	}

	logger.Infof("📝 Decision record saved: trader=%s, cycle=%d", at.id, at.cycleNumber)
	return nil
}

func summarizeDecisionExecution(record *store.DecisionRecord) {
	failed, attempted := 0, 0
	for _, action := range record.Decisions {
		if action.Action == "hold" || action.Action == "wait" {
			continue
		}
		attempted++
		if !action.Success {
			failed++
		}
	}
	if failed > 0 {
		record.Success = false
		if record.ErrorMessage == "" {
			record.ErrorMessage = fmt.Sprintf("%d of %d trade actions failed or were blocked", failed, attempted)
		}
	}
}

// GetStatus gets system status (for API)
func (at *AutoTrader) GetStatus() map[string]interface{} {
	agentProvider := "DeepSeek"
	if at.config.UseQwen {
		agentProvider = "Qwen"
	}

	at.isRunningMutex.RLock()
	isRunning := at.isRunning
	at.isRunningMutex.RUnlock()

	result := map[string]interface{}{
		"trader_id":       at.id,
		"trader_name":     at.name,
		"agent_model":     at.agentModel,
		"exchange":        at.exchange,
		"is_running":      isRunning,
		"start_time":      at.startTime.Format(time.RFC3339),
		"runtime_minutes": int(time.Since(at.startTime).Minutes()),
		"call_count":      at.callCount,
		"initial_balance": at.initialBalance,
		"scan_interval":   at.config.ScanInterval.String(),
		"stop_until":      at.stopUntil.Format(time.RFC3339),
		"last_reset_time": at.lastResetTime.Format(time.RFC3339),
		"agent_provider":  agentProvider,
	}

	// Add strategy info
	if at.config.StrategyConfig != nil {
		result["strategy_type"] = at.config.StrategyConfig.StrategyType
		if at.config.StrategyConfig.GridConfig != nil {
			result["grid_symbol"] = at.config.StrategyConfig.GridConfig.Symbol
		}
	}

	// Runtime health: safe mode + agent fee wallet, so the dashboard can show a
	// persistent banner instead of the user digging through logs.
	safeMode, safeModeReason := at.safeModeState()
	result["safe_mode"] = safeMode
	at.runtimeHealthMu.RLock()
	result["trading_blocked"] = at.tradingBlocked
	result["trading_error"] = at.tradingError
	if !at.tradingCheckedAt.IsZero() {
		result["trading_checked_at"] = at.tradingCheckedAt.Format(time.RFC3339)
	}
	at.runtimeHealthMu.RUnlock()
	if safeModeReason != "" {
		result["safe_mode_reason"] = safeModeReason
	}

	return result
}

// GetAccountInfo gets account information (for API)
func (at *AutoTrader) GetAccountInfo() (map[string]interface{}, error) {
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Get account fields
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0
	totalEquity := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Use totalEquity directly if provided by trader (more accurate)
	if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
		totalEquity = eq
	} else {
		// Fallback: Total Equity = Wallet balance + Unrealized profit
		totalEquity = totalWalletBalance + totalUnrealizedProfit
	}

	// Get positions to calculate total margin
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	totalMarginUsed := 0.0
	totalUnrealizedPnLCalculated := 0.0
	for _, pos := range positions {
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		totalUnrealizedPnLCalculated += unrealizedPnl

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed
	}

	// Verify unrealized P&L consistency (API value vs calculated from positions)
	diff := math.Abs(totalUnrealizedProfit - totalUnrealizedPnLCalculated)
	if diff > 5.0 { // Only warn if difference is significant (> 5 USDT)
		logger.Infof("⚠️ Unrealized P&L inconsistency: API=%.4f, Calculated=%.4f, Diff=%.4f",
			totalUnrealizedProfit, totalUnrealizedPnLCalculated, diff)
	}

	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	} else {
		logger.Infof("⚠️ Initial Balance abnormal: %.2f, cannot calculate P&L percentage", at.initialBalance)
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	return map[string]interface{}{
		// Core fields
		"total_equity":      totalEquity,           // Account equity = wallet + unrealized
		"wallet_balance":    totalWalletBalance,    // Wallet balance (excluding unrealized P&L)
		"unrealized_profit": totalUnrealizedProfit, // Unrealized P&L (official value from exchange API)
		"available_balance": availableBalance,      // Available balance

		// P&L statistics
		"total_pnl":       totalPnL,          // Total P&L = equity - initial
		"total_pnl_pct":   totalPnLPct,       // Total P&L percentage
		"initial_balance": at.initialBalance, // Initial balance
		"daily_pnl":       at.dailyPnL,       // Daily P&L

		// Position information
		"position_count":  len(positions),  // Position count
		"margin_used":     totalMarginUsed, // Margin used
		"margin_used_pct": marginUsedPct,   // Margin usage rate
	}, nil
}

// GetPositions gets position list (for API)
func (at *AutoTrader) GetPositions() ([]map[string]interface{}, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		// Calculate margin used
		marginUsed := (quantity * markPrice) / float64(leverage)

		// Calculate P&L percentage (based on margin)
		pnlPct := calculatePnLPercentage(unrealizedPnl, marginUsed)

		result = append(result, map[string]interface{}{
			"symbol":             symbol,
			"side":               side,
			"entry_price":        entryPrice,
			"mark_price":         markPrice,
			"quantity":           quantity,
			"leverage":           leverage,
			"unrealized_pnl":     unrealizedPnl,
			"unrealized_pnl_pct": pnlPct,
			"liquidation_price":  liquidationPrice,
			"margin_used":        marginUsed,
		})
	}

	return result, nil
}

// recordOrder puts a placed order in the ledger, with what only placing it knows: who
// placed it, why, and at what leverage. Its fills reach the ledger through the account's
// sync, whichever gets there first. A failure is only logged: the order is already on
// the exchange, and without this record its fills still arrive. It returns the order's
// ID in the ledger, or 0 without one.
// action: open_long, open_short, close_long, close_short
func (at *AutoTrader) recordOrder(orderResult map[string]interface{}, symbol, action string, quantity float64, leverage int) int64 {
	if at.store == nil {
		return 0
	}
	orderID := orderIDOf(orderResult["orderId"])
	if orderID == "" || orderID == "0" {
		logger.Infof("  ⚠️ Order ID is empty, skipping record")
		return 0
	}
	clientOrderID, _ := orderResult["clientOrderId"].(string)

	order := store.LedgerOrder{
		AccountID:       at.exchangeID,
		Source:          store.SourceTrader,
		SourceID:        at.id,
		Role:            store.RoleEntry,
		ExchangeOrderID: orderID,
		ClientOrderID:   clientOrderID,
		Side:            store.SideBuy,
		PositionSide:    store.SideLong,
		Type:            "MARKET",
		Status:          store.OrderNew,
		Quantity:        decimal.NewFromFloat(quantity),
	}
	switch action {
	case "open_short":
		order.Side, order.PositionSide = store.SideSell, store.SideShort
	case "close_long":
		order.Role, order.Side, order.ReduceOnly = store.RoleClose, store.SideSell, true
	case "close_short":
		order.Role, order.PositionSide, order.ReduceOnly = store.RoleClose, store.SideShort, true
	}
	if leverage > 0 {
		order.Leverage = &leverage
		order.MarginMode = at.marginMode()
	}
	if err := at.saveOrder(&order, symbol); err != nil {
		logger.Infof("  ⚠️ Failed to record order %s: %v", orderID, err)
		return 0
	}
	logger.Infof("  📝 Order recorded: %s [%s] %s", orderID, action, symbol)
	return order.ID
}

func orderIDOf(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (at *AutoTrader) marginMode() string {
	return store.MarginModeOf(at.config.IsCrossMargin)
}

// saveOrder records an order on the instrument a symbol names on this trader's venue.
func (at *AutoTrader) saveOrder(order *store.LedgerOrder, symbol string) error {
	symbol = instrument.Normalize(symbol)
	native := symbol
	if at.exchange == "okx" {
		native = okx.InstrumentID(symbol)
	}
	instrumentID, err := instrument.Ensure(at.store.GormDB(), at.exchange, instrument.KindPerpetual, symbol, native)
	if err != nil {
		return err
	}
	order.InstrumentID = instrumentID
	return at.store.Ledger().RecordOrder(order)
}

// GetOpenOrders returns open orders (pending SL/TP) from exchange
func (at *AutoTrader) GetOpenOrders(symbol string) ([]OpenOrder, error) {
	return at.trader.GetOpenOrders(symbol)
}
