package trader

import (
	"fmt"
	"strings"
	"time"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/trader/agent"
)

// runCycle runs one trading cycle (using agent full decision-making)
func (at *AutoTrader) runCycle() error {
	at.callCount++

	logger.Info("\n" + strings.Repeat("=", 70) + "\n")
	logger.Infof("⏰ %s - agent decision cycle #%d", time.Now().Format("2006-01-02 15:04:05"), at.callCount)
	logger.Info(strings.Repeat("=", 70))

	// 0. Check if trader is stopped (early exit to prevent trades after Stop() is called)
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		at.logInfof("⏹ Trader is stopped, aborting cycle #%d", at.callCount)
		return nil
	}

	if err := at.reloadStrategyConfigIfChanged(); err != nil {
		at.logWarnf("⚠️ Strategy refresh failed, using current in-memory config: %v", err)
	}

	// Create decision record
	record := &store.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
	}
	// Check permission before paid data/model calls. Reading account balances
	// does not prove that the configured signing agent can execute trades.
	if err := at.checkTradingAuthorization(); err != nil {
		record.Success = false
		record.ErrorMessage = "Trading blocked: " + err.Error()
		record.ExecutionLog = append(record.ExecutionLog, record.ErrorMessage)
		if saveErr := at.saveDecision(record); saveErr != nil {
			at.logWarnf("Failed to save blocked cycle: %v", saveErr)
		}
		return fmt.Errorf("trading authorization check failed: %w", err)
	}

	// 1. Check if trading needs to be stopped
	if time.Now().Before(at.stopUntil) {
		remaining := at.stopUntil.Sub(time.Now())
		at.logWarnf("⏸ Risk control: Trading paused, remaining %.0f minutes", remaining.Minutes())
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Risk control paused, remaining %.0f minutes", remaining.Minutes())
		if err := at.saveDecision(record); err != nil {
			at.logWarnf("⚠ Failed to save decision record: %v", err)
		}
		return nil
	}

	// 2. Reset daily P&L (reset every day)
	if time.Since(at.lastResetTime) > 24*time.Hour {
		at.dailyPnL = 0
		at.lastResetTime = time.Now()
		logger.Info("📅 Daily P&L reset")
	}

	// 4. Collect trading context
	ctx, err := at.buildTradingContext()
	if err != nil {
		at.logErrorf("failed to build trading context: %v", err)
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Failed to build trading context: %v", err)
		if saveErr := at.saveDecision(record); saveErr != nil {
			at.logWarnf("⚠ Failed to save decision record: %v", saveErr)
		}
		return fmt.Errorf("failed to build trading context: %w", err)
	}

	// Save equity snapshot independently (decoupled from agent decision, used for drawing profit curve)
	// NOTE: Must be called BEFORE candidate coins check to ensure equity is always recorded
	at.saveEquitySnapshot(ctx)
	record.AccountState = &store.AccountSnapshot{
		TotalBalance:          ctx.Account.TotalEquity,
		AvailableBalance:      ctx.Account.AvailableBalance,
		TotalUnrealizedProfit: ctx.Account.UnrealizedPnL,
		PositionCount:         ctx.Account.PositionCount,
		MarginUsedPct:         ctx.Account.MarginUsedPct,
		InitialBalance:        at.initialBalance,
	}

	// If no candidate coins available, log but do not error
	if len(ctx.CandidateCoins) == 0 {
		at.logInfof("ℹ️ No candidate coins available, skipping this cycle")
		record.Success = true // Not an error, just no candidate coins
		record.ExecutionLog = append(record.ExecutionLog, "No candidate coins available, cycle skipped")
		if err := at.saveDecision(record); err != nil {
			at.logWarnf("⚠ Failed to save decision record: %v", err)
		}
		return nil
	}

	logger.Info(strings.Repeat("=", 70))
	for _, coin := range ctx.CandidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}

	at.logInfof("📊 Account equity: %.2f USDT | Available: %.2f USDT | Positions: %d",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance, ctx.Account.PositionCount)

	// 5. Use strategy engine to call agent for decision
	at.logInfof("🤖 Requesting agent analysis and decision... [Strategy Engine]")
	agentDecision, err := agent.GetFullDecisionWithStrategy(ctx, at.agentClient, at.strategyEngine, "balanced", at.openConversation)
	at.keepConversation(agentDecision)

	if agentDecision != nil && agentDecision.AgentDurationMs > 0 {
		record.AgentDurationMs = agentDecision.AgentDurationMs
		at.logInfof("⏱️ model call duration: %.2f seconds", float64(record.AgentDurationMs)/1000)
		record.ExecutionLog = append(record.ExecutionLog,
			fmt.Sprintf("model call duration: %d ms", record.AgentDurationMs))
	}

	// The prompts and reply are in the conversation, kept even if there's an error
	if agentDecision != nil {
		record.ConversationID = agentDecision.ConversationID
	}

	if err != nil {
		at.consecutiveAgentFailures++
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("Failed to get agent decision: %v", err)

		// Activate safe mode after 3 consecutive failures
		if at.consecutiveAgentFailures >= 3 && !at.isSafeMode() {
			at.setSafeMode(true, fmt.Sprintf("Model failed %d consecutive times: %v", at.consecutiveAgentFailures, err))
			at.logErrorf("🛡️ SAFE MODE ACTIVATED — Model failed %d times in a row. No new positions will be opened. Existing positions are protected with current stop-loss settings.", at.consecutiveAgentFailures)
			at.logErrorf("🛡️ Reason: %v", err)
			at.logErrorf("🛡️ Action: Will keep trying the model each cycle. Safe mode auto-deactivates when the model recovers.")
		}

		// Print system prompt and agent chain of thought (output even with errors for debugging)
		if agentDecision != nil {
			logger.Info("\n" + strings.Repeat("=", 70) + "\n")
			logger.Infof("📋 System prompt (error case)")
			logger.Info(strings.Repeat("=", 70))
			logger.Info(agentDecision.SystemPrompt)
			logger.Info(strings.Repeat("=", 70))

			if agentDecision.CoTTrace != "" {
				logger.Info("\n" + strings.Repeat("-", 70) + "\n")
				logger.Info("💭 agent chain of thought analysis (error case):")
				logger.Info(strings.Repeat("-", 70))
				logger.Info(agentDecision.CoTTrace)
				logger.Info(strings.Repeat("-", 70))
			}
		}

		if saveErr := at.saveDecision(record); saveErr != nil {
			at.logWarnf("⚠ Failed to save decision record: %v", saveErr)
		}

		// In safe mode, don't return error — keep the loop running to retry next cycle
		if at.isSafeMode() {
			at.logWarnf("🛡️ Safe mode: skipping this cycle, will retry in %v", at.config.ScanInterval)
			return nil
		}

		return fmt.Errorf("failed to get agent decision: %w", err)
	}

	// Agent succeeded — reset failure counter and deactivate safe mode
	if at.consecutiveAgentFailures > 0 {
		at.logInfof("✅ Model recovered after %d consecutive failures", at.consecutiveAgentFailures)
	}
	at.consecutiveAgentFailures = 0
	if at.isSafeMode() {
		at.logInfof("🛡️ SAFE MODE DEACTIVATED — the model is working again. Resuming normal trading.")
		at.setSafeMode(false, "")
	}

	// // 5. Print system prompt
	// logger.Infof("\n" + strings.Repeat("=", 70))
	// logger.Info(strings.Repeat("=", 70))
	// logger.Info(decision.SystemPrompt)
	// logger.Infof(strings.Repeat("=", 70) + "\n")

	// 6. Print agent chain of thought
	// logger.Infof("\n" + strings.Repeat("-", 70))
	// logger.Info("💭 agent chain of thought analysis:")
	// logger.Info(strings.Repeat("-", 70))
	// logger.Info(decision.CoTTrace)
	// logger.Infof(strings.Repeat("-", 70) + "\n")

	// 7. Print agent decisions
	// logger.Infof("📋 agent decision list (%d items):\n", len(agent.Decisions))
	// for i, d := range agent.Decisions {
	//     logger.Infof("  [%d] %s: %s - %s", i+1, d.Symbol, d.Action, d.Reasoning)
	//     if d.Action == "open_long" || d.Action == "open_short" {
	//        logger.Infof("      Leverage: %dx | Position: %.2f USDT | Stop loss: %.4f | Take profit: %.4f",
	//           d.Leverage, d.PositionSizeUSD, d.StopLoss, d.TakeProfit)
	//     }
	// }
	logger.Info()
	logger.Info(strings.Repeat("-", 70))
	// 8. Sort decisions: ensure close positions first, then open positions (prevent position stacking overflow)
	logger.Info(strings.Repeat("-", 70))

	// 8. Sort decisions: ensure close positions first, then open positions (prevent position stacking overflow)
	sortedDecisions := sortDecisionsByPriority(agentDecision.Decisions)
	sortedDecisions = at.filterDecisionsToStrategyUniverse(sortedDecisions, ctx)
	sortedDecisions = sortDecisionsByPriority(sortedDecisions)

	logger.Info("🔄 Execution order (optimized): Close positions first → Open positions later")
	for i, d := range sortedDecisions {
		logger.Infof("  [%d] %s %s", i+1, d.Symbol, d.Action)
	}
	logger.Info()

	// Check if trader is stopped before executing any decisions (prevent trades after Stop())
	at.isRunningMutex.RLock()
	running = at.isRunning
	at.isRunningMutex.RUnlock()
	if !running {
		at.logInfof("⏹ Trader stopped before decision execution, aborting cycle #%d", at.callCount)
		return nil
	}

	// Safe mode: filter out open positions, only allow close/hold
	if at.isSafeMode() {
		filtered := make([]agent.Decision, 0)
		for _, d := range sortedDecisions {
			if d.Action == "open_long" || d.Action == "open_short" {
				at.logWarnf("🛡️ Safe mode: BLOCKED %s %s (no new positions allowed)", d.Action, d.Symbol)
				continue
			}
			filtered = append(filtered, d)
		}
		sortedDecisions = filtered
		if len(sortedDecisions) == 0 {
			at.logInfof("🛡️ Safe mode: all decisions were open positions, nothing to execute")
		}
	}

	// Execute decisions and record results. Position and exit safeguards are
	// applied immediately before order placement.
	for _, d := range sortedDecisions {
		// Check if trader is stopped before each decision (allow immediate stop during execution)
		at.isRunningMutex.RLock()
		running = at.isRunning
		at.isRunningMutex.RUnlock()
		if !running {
			at.logInfof("⏹ Trader stopped during decision execution, aborting remaining decisions")
			break
		}

		actionRecord := store.DecisionAction{
			Action:     d.Action,
			Symbol:     d.Symbol,
			Quantity:   0,
			Leverage:   d.Leverage,
			Price:      0,
			StopLoss:   d.StopLoss,
			TakeProfit: d.TakeProfit,
			Confidence: d.Confidence,
			Reasoning:  d.Reasoning,
			Timestamp:  time.Now().UTC(),
			Success:    false,
		}

		if reason := at.tradeThrottleReason(d, ctx); reason != "" {
			at.logWarnf("🧊 %s %s blocked: %s", d.Symbol, d.Action, reason)
			actionRecord.Error = reason
			actionRecord.Blocked = true
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("🧊 %s %s blocked: %s", d.Symbol, d.Action, reason))
			record.Decisions = append(record.Decisions, actionRecord)
			continue
		}
		if err := at.executeDecisionWithRecord(&d, &actionRecord); err != nil {
			at.logErrorf("❌ Failed to execute decision (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("❌ %s %s failed: %v", d.Symbol, d.Action, err))
		} else {
			actionRecord.Success = true
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ %s %s succeeded", d.Symbol, d.Action))
			// Brief delay after successful execution
			time.Sleep(1 * time.Second)
		}

		record.Decisions = append(record.Decisions, actionRecord)
	}

	// 9. Save decision record
	if err := at.saveDecision(record); err != nil {
		at.logWarnf("⚠ Failed to save decision record: %v", err)
	}

	return nil
}

func normalizeUniverseSymbol(symbol string) string {
	return instrument.Normalize(strings.TrimSpace(symbol))
}

// universeBaseKey returns the bare base ticker used for fuzzy candidate
// matching. The agent sometimes echoes a candidate as "BTC", "BTC-USDT" or
// "BTCUSDC" instead of the canonical "BTCUSDT" we supplied in the prompt, so
// we accept any of them when the base matches an allowed candidate's base.
func universeBaseKey(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" {
		return ""
	}
	// Drop common quote suffixes in order of specificity.
	for _, suf := range []string{"-USDC", "-USDT", "USDC", "USDT", "USD"} {
		if strings.HasSuffix(s, suf) && len(s) > len(suf) {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	return s
}

func isOpenDecision(action string) bool {
	a := strings.ToLower(strings.TrimSpace(action))
	return a == "open_long" || a == "open_short"
}

func (at *AutoTrader) filterDecisionsToStrategyUniverse(decisions []agent.Decision, ctx *agent.Context) []agent.Decision {
	if ctx == nil || len(decisions) == 0 {
		return decisions
	}

	allowed := make(map[string]bool, len(ctx.CandidateCoins))
	allowedBases := make(map[string]bool, len(ctx.CandidateCoins))
	for _, coin := range ctx.CandidateCoins {
		allowed[normalizeUniverseSymbol(coin.Symbol)] = true
		if base := universeBaseKey(coin.Symbol); base != "" {
			allowedBases[base] = true
		}
	}

	positions := make(map[string]bool, len(ctx.Positions))
	positionBases := make(map[string]bool, len(ctx.Positions))
	for _, pos := range ctx.Positions {
		positions[normalizeUniverseSymbol(pos.Symbol)] = true
		if base := universeBaseKey(pos.Symbol); base != "" {
			positionBases[base] = true
		}
	}

	filtered := make([]agent.Decision, 0, len(decisions))
	for _, d := range decisions {
		sym := normalizeUniverseSymbol(d.Symbol)
		if sym == "" || sym == "ALL" {
			filtered = append(filtered, d)
			continue
		}

		if allowed[sym] || positions[sym] {
			filtered = append(filtered, d)
			continue
		}

		// Fall back to base-level match so agent outputs like "QNTUSDC" still
		// resolve to candidate "QNTUSDT". Rewrite the decision's symbol to
		// the canonical form so downstream order placement works.
		if base := universeBaseKey(d.Symbol); base != "" {
			if allowedBases[base] || positionBases[base] {
				if canonical := canonicalUniverseSymbolForBase(ctx, base); canonical != "" {
					d.Symbol = canonical
				}
				filtered = append(filtered, d)
				continue
			}
		}

		if isOpenDecision(d.Action) {
			at.logWarnf("🚫 Blocked agent %s for %s: symbol is outside strategy candidate universe", d.Action, d.Symbol)
		} else {
			at.logWarnf("🚫 Dropped agent decision for %s: symbol is outside strategy candidate universe", d.Symbol)
		}
	}
	return filtered
}

// canonicalUniverseSymbolForBase finds the canonical candidate or position
// symbol that has the given base. Used to rewrite an agent decision's symbol
// (e.g. "QNTUSDC") to the form our order pipeline expects ("QNTUSDT").
func canonicalUniverseSymbolForBase(ctx *agent.Context, base string) string {
	for _, coin := range ctx.CandidateCoins {
		if universeBaseKey(coin.Symbol) == base {
			return coin.Symbol
		}
	}
	for _, pos := range ctx.Positions {
		if universeBaseKey(pos.Symbol) == base {
			return pos.Symbol
		}
	}
	return ""
}

// buildTradingContext builds trading context
func (at *AutoTrader) buildTradingContext() (*agent.Context, error) {
	// 1. Get account information
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get account balance: %w", err)
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

	// 2. Get position information
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	var positionInfos []agent.PositionInfo
	totalMarginUsed := 0.0

	// Current position key set (for cleaning up closed position records)
	currentPositionKeys := make(map[string]bool)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // Short position quantity is negative, convert to positive
		}

		// Skip closed positions (quantity = 0), prevent "ghost positions" from being passed to agent
		if quantity == 0 {
			continue
		}

		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		// Calculate margin used (estimated)
		leverage := 10 // Default value, should actually be fetched from position info
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed

		// Calculate P&L percentage (based on margin, considering leverage)
		pnlPct := calculatePnLPercentage(unrealizedPnl, marginUsed)

		// Get position open time from exchange (preferred) or fallback to local tracking
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true

		var updateTime int64
		// Priority 1: Get from database (trader_positions table) - most accurate
		if at.store != nil {
			if dbPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, symbol, side); err == nil && dbPos != nil {
				if dbPos.EntryTime > 0 {
					updateTime = dbPos.EntryTime
				}
			}
		}
		// Priority 2: Get from exchange API (OKX: createdTime)
		if updateTime == 0 {
			if createdTime, ok := pos["createdTime"].(int64); ok && createdTime > 0 {
				updateTime = createdTime
			}
		}
		// Priority 3: Fallback to local tracking
		if updateTime == 0 {
			if _, exists := at.positionFirstSeenTime[posKey]; !exists {
				at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()
			}
			updateTime = at.positionFirstSeenTime[posKey]
		}

		// Get peak profit rate for this position
		at.peakPnLCacheMutex.RLock()
		peakPnlPct := at.peakPnLCache[posKey]
		at.peakPnLCacheMutex.RUnlock()

		positionInfos = append(positionInfos, agent.PositionInfo{
			Symbol:           symbol,
			Side:             side,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			Quantity:         quantity,
			Leverage:         leverage,
			UnrealizedPnL:    unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			PeakPnLPct:       peakPnlPct,
			LiquidationPrice: liquidationPrice,
			MarginUsed:       marginUsed,
			UpdateTime:       updateTime,
		})
	}

	// Clean up closed position records
	for key := range at.positionFirstSeenTime {
		if !currentPositionKeys[key] {
			delete(at.positionFirstSeenTime, key)
		}
	}

	// 3. Use strategy engine to get candidate coins (must have strategy engine)
	var candidateCoins []agent.CandidateCoin
	if at.strategyEngine == nil {
		at.logWarnf("⚠️ No strategy engine configured, skipping candidate coins")
	} else {
		coins, err := at.strategyEngine.GetCandidateCoins()
		if err != nil {
			at.logWarnf("⚠️ Failed to get candidate coins: %v (will use empty list)", err)
		} else {
			candidateCoins = coins
			logger.Infof("📋 [%s] Strategy engine fetched candidate coins: %d", at.name, len(candidateCoins))
		}
	}

	// 4. Calculate total P&L
	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	// 5. Get leverage from strategy config
	strategyConfig := at.strategyEngine.GetConfig()
	btcEthLeverage := strategyConfig.RiskControl.BTCETHMaxLeverage
	altcoinLeverage := strategyConfig.RiskControl.AltcoinMaxLeverage
	logger.Infof("📋 [%s] Strategy leverage config: BTC/ETH=%dx, Altcoin=%dx", at.name, btcEthLeverage, altcoinLeverage)

	// 6. Build context
	ctx := &agent.Context{
		CurrentTime:     time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes:  int(time.Since(at.startTime).Minutes()),
		CallCount:       at.callCount,
		BTCETHLeverage:  btcEthLeverage,
		AltcoinLeverage: altcoinLeverage,
		Account: agent.AccountInfo{
			TotalEquity:      totalEquity,
			AvailableBalance: availableBalance,
			UnrealizedPnL:    totalUnrealizedProfit,
			TotalPnL:         totalPnL,
			TotalPnLPct:      totalPnLPct,
			MarginUsed:       totalMarginUsed,
			MarginUsedPct:    marginUsedPct,
			PositionCount:    len(positionInfos),
		},
		Positions:      positionInfos,
		CandidateCoins: candidateCoins,
	}

	// 7. Add recent closed trades (if store is available)
	if at.store != nil {
		// Get recent 10 closed trades for agent context
		recentTrades, err := at.store.Position().GetRecentTrades(at.id, 10)
		if err != nil {
			at.logWarnf("⚠️ Failed to get recent trades: %v", err)
		} else {
			logger.Infof("📊 [%s] Found %d recent closed trades for agent context", at.name, len(recentTrades))
			for _, trade := range recentTrades {
				// Convert Unix timestamps to formatted strings for agent readability
				entryTimeStr := ""
				if trade.EntryTime > 0 {
					entryTimeStr = time.Unix(trade.EntryTime, 0).UTC().Format("01-02 15:04 UTC")
				}
				exitTimeStr := ""
				if trade.ExitTime > 0 {
					exitTimeStr = time.Unix(trade.ExitTime, 0).UTC().Format("01-02 15:04 UTC")
				}

				ctx.RecentOrders = append(ctx.RecentOrders, agent.RecentOrder{
					Symbol:       trade.Symbol,
					Side:         trade.Side,
					EntryPrice:   trade.EntryPrice,
					ExitPrice:    trade.ExitPrice,
					RealizedPnL:  trade.RealizedPnL,
					PnLPct:       trade.PnLPct,
					EntryTime:    entryTimeStr,
					ExitTime:     exitTimeStr,
					HoldDuration: trade.HoldDuration,
				})
			}
		}
		// Get trading statistics for agent context
		stats, err := at.store.Position().GetFullStats(at.id, at.initialBalance)
		if err != nil {
			at.logWarnf("⚠️ Failed to get trading stats: %v", err)
		} else if stats == nil {
			at.logWarnf("⚠️ GetFullStats returned nil")
		} else if stats.TotalTrades == 0 {
			at.logWarnf("⚠️ GetFullStats returned 0 trades")
		} else {
			ctx.TradingStats = &agent.TradingStats{
				TotalTrades:    stats.TotalTrades,
				WinRate:        stats.WinRate,
				ProfitFactor:   stats.ProfitFactor,
				SharpeRatio:    stats.SharpeRatio,
				TotalPnL:       stats.TotalPnL,
				AvgWin:         stats.AvgWin,
				AvgLoss:        stats.AvgLoss,
				MaxDrawdownPct: stats.MaxDrawdownPct,
			}
			logger.Infof("📈 [%s] Trading stats: %d trades, %.1f%% win rate, PF=%.2f, Sharpe=%.2f, DD=%.1f%%",
				at.name, stats.TotalTrades, stats.WinRate, stats.ProfitFactor, stats.SharpeRatio, stats.MaxDrawdownPct)
		}
	} else {
		at.logWarnf("⚠️ Store is nil, cannot get recent trades")
	}

	return ctx, nil
}

// sortDecisionsByPriority sorts decisions: close positions first, then open positions, finally hold/wait
// This avoids position stacking overflow when changing positions
func sortDecisionsByPriority(decisions []agent.Decision) []agent.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	// Define priority
	getActionPriority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1 // Highest priority: close positions first
		case "open_long", "open_short":
			return 2 // Second priority: open positions later
		case "hold", "wait":
			return 3 // Lowest priority: wait
		default:
			return 999 // Unknown actions at the end
		}
	}

	// Copy decision list
	sorted := make([]agent.Decision, len(decisions))
	copy(sorted, decisions)

	// Sort by priority
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if getActionPriority(sorted[i].Action) > getActionPriority(sorted[j].Action) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}
