package trader

import (
	"context"
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"math"
	"time"
	"vergex/internal/account"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/kind"
	"vergex/internal/venue/okx"
)

// saveEquitySnapshot saves equity snapshot independently (for drawing profit curve, decoupled from agent decision)
func (at *AutoTrader) saveEquitySnapshot(ctx *traderagent.Context) {
	if at.stores == nil || ctx == nil {
		return
	}

	snapshot := &EquitySnapshot{
		TraderID:         at.id,
		Timestamp:        time.Now().UTC(),
		TotalEquity:      ctx.Account.TotalEquity,
		Balance:          ctx.Account.TotalEquity - ctx.Account.UnrealizedPnL,
		AvailableBalance: ctx.Account.AvailableBalance,
		UnrealizedPnL:    ctx.Account.UnrealizedPnL,
		PositionCount:    ctx.Account.PositionCount,
		MarginUsedPct:    ctx.Account.MarginUsedPct,
	}

	if err := at.stores.Equity.Save(snapshot); err != nil {
		logger.Infof("⚠️ Failed to save equity snapshot: %v", err)
	}
}

// cycleSaveWait bounds saving a cycle. Its orders are already on the exchange, and a
// cycle's record points at its conversation, so a stuck conversation can hold this up.
const cycleSaveWait = 5 * time.Second

// saveDecision saves agent decision log to database (only records agent input/output, for debugging)
func (at *AutoTrader) saveDecision(record *DecisionRecord) error {
	summarizeDecisionExecution(record)
	if at.stores == nil {
		return nil
	}

	at.cycleNumber++
	record.CycleNumber = at.cycleNumber
	record.TraderID = at.id
	record.Limits = at.limits()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}

	ctx, cancel := context.WithTimeout(context.Background(), cycleSaveWait)
	defer cancel()
	if err := at.stores.Decisions.WithContext(ctx).LogDecision(record); err != nil {
		logger.Infof("⚠️ Failed to save decision record: %v", err)
		return err
	}

	logger.Infof("📝 Decision record saved: trader=%s, cycle=%d", at.id, at.cycleNumber)
	return nil
}

// summarizeDecisionExecution marks a cycle as failed if an action on the exchange failed.
// A rule stopping an action is the rule working, not a fault.
func summarizeDecisionExecution(record *DecisionRecord) {
	failed, attempted := 0, 0
	for _, action := range record.Decisions {
		if action.Action == "hold" || action.Action == "wait" || action.Blocked {
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
			record.ErrorMessage = fmt.Sprintf("%d of %d trade actions failed", failed, attempted)
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
		"venue":           at.venue,
		"is_running":      isRunning,
		"start_time":      at.startTime.Format(time.RFC3339),
		"runtime_minutes": int(time.Since(at.startTime).Minutes()),
		"call_count":      at.callCount,
		"initial_balance": at.initialBalance,
		"scan_interval":   at.config.ScanInterval.String(),
		"agent_provider":  agentProvider,
	}

	// Add what kind of trader it is
	if at.config.Grid != nil {
		result["kind"] = kind.Grid
		result["grid_symbol"] = at.config.Grid.Symbol
	} else {
		result["kind"] = kind.Agent
	}

	if halt := at.ActiveHalt(); halt != nil {
		result["halted"] = halt
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

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	info := map[string]interface{}{
		// Core fields
		"total_equity":      totalEquity,           // Account equity = wallet + unrealized
		"wallet_balance":    totalWalletBalance,    // Wallet balance (excluding unrealized P&L)
		"unrealized_profit": totalUnrealizedProfit, // Unrealized P&L (official value from exchange API)
		"available_balance": availableBalance,      // Available balance

		"initial_balance": at.initialBalance, // Initial balance

		// Position information
		"position_count":  len(positions),  // Position count
		"margin_used":     totalMarginUsed, // Margin used
		"margin_used_pct": marginUsedPct,   // Margin usage rate
	}

	// P&L statistics. Without them the interface shows a dash rather than a wrong figure.
	if totalPnL, err := at.ownProfit(positions); err != nil {
		logger.Warnf("Trader %s: read its profit: %v", at.id, err)
	} else {
		info["total_pnl"] = totalPnL
		if at.initialBalance > 0 {
			info["total_pnl_pct"] = totalPnL / at.initialBalance * 100
		} else {
			logger.Infof("⚠️ Initial Balance abnormal: %.2f, cannot calculate P&L percentage", at.initialBalance)
		}
	}
	return info, nil
}

// ownProfit is what this trader has made by the ledger: what it realized after fees and
// what the positions it holds stand at on the exchange. The account's equity is not used,
// since other traders, Futures and transfers all move it.
func (at *AutoTrader) ownProfit(positions []map[string]interface{}) (float64, error) {
	if at.stores == nil {
		return 0, errors.New("no ledger")
	}
	profit, err := at.stores.Positions.NetOfTrader(at.id)
	if err != nil {
		return 0, err
	}
	for _, position := range positions {
		symbol, _ := position["symbol"].(string)
		side, _ := position["side"].(string)
		held, err := at.stores.Positions.GetOpenPositionBySymbol(at.id, symbol, side)
		if err != nil {
			return 0, err
		}
		if held != nil {
			unrealized, _ := position["unRealizedProfit"].(float64)
			profit += unrealized
		}
	}
	return profit, nil
}

// recordOrder puts a placed order in the ledger, with what only placing it knows: who
// placed it, why, and at what leverage. Its fills reach the ledger through the account's
// sync, whichever gets there first. A failure is only logged: the order is already on
// the exchange, and without this record its fills still arrive. It returns the order's
// ID in the ledger, or 0 without one.
// action: open_long, open_short, close_long, close_short
func (at *AutoTrader) recordOrder(orderResult map[string]interface{}, symbol, action string, quantity float64, leverage int) int64 {
	if at.stores == nil {
		return 0
	}
	orderID := orderIDOf(orderResult["orderId"])
	if orderID == "" || orderID == "0" {
		logger.Infof("  ⚠️ Order ID is empty, skipping record")
		return 0
	}
	clientOrderID, _ := orderResult["clientOrderId"].(string)

	order := account.LedgerOrder{
		AccountID:       at.accountID,
		Source:          account.SourceTrader,
		SourceID:        at.id,
		Role:            account.RoleEntry,
		ExchangeOrderID: orderID,
		ClientOrderID:   clientOrderID,
		Side:            account.SideBuy,
		PositionSide:    account.SideLong,
		Type:            "MARKET",
		Status:          account.OrderNew,
		Quantity:        decimal.NewFromFloat(quantity),
	}
	switch action {
	case "open_short":
		order.Side, order.PositionSide = account.SideSell, account.SideShort
	case "close_long":
		order.Role, order.Side, order.ReduceOnly = account.RoleClose, account.SideSell, true
	case "close_short":
		order.Role, order.PositionSide, order.ReduceOnly = account.RoleClose, account.SideShort, true
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
	return MarginModeOf(at.config.IsCrossMargin)
}

// saveOrder records an order on the instrument a symbol names on this trader's venue.
func (at *AutoTrader) saveOrder(order *account.LedgerOrder, symbol string) error {
	symbol = instrument.Normalize(symbol)
	native := symbol
	if at.venue == "okx" {
		native = okx.InstrumentID(symbol)
	}
	instrumentID, err := instrument.Ensure(at.stores.DB, at.venue, instrument.KindPerpetual, symbol, native)
	if err != nil {
		return err
	}
	order.InstrumentID = instrumentID
	// The exchange took the quantity rounded to the market's step.
	if at.trader != nil {
		if formatted, err := at.trader.FormatQuantity(symbol, order.Quantity.InexactFloat64()); err == nil {
			if rounded, err := decimal.NewFromString(formatted); err == nil && rounded.IsPositive() {
				order.Quantity = rounded
			}
		}
	}
	return at.stores.Ledger.RecordOrder(order)
}
