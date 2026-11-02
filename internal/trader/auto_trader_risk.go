package trader

import (
	"fmt"
	"math"
	"strings"
	"time"
	"vergex/internal/logger"
	"vergex/internal/market"
	traderagent "vergex/internal/trader/agent"
)

const (
	// The monitor arms only once the underlying PRICE has moved +5% in the
	// position's favor (leverage-independent — at 10x the old margin-basis
	// check armed at a +0.5% price wiggle and strangled every winner), then
	// closes if the position gives back 40% of its peak profit.
	drawdownClosePriceGainPct = 5.0
	drawdownCloseGivebackPct  = 40.0
)

// shouldDrawdownClose reports whether the profit-protection close should fire.
// pricePnLPct is the price-basis move in the position's favor; drawdownPct is
// the relative giveback from the position's peak profit.
func shouldDrawdownClose(pricePnLPct, drawdownPct float64) bool {
	return pricePnLPct > drawdownClosePriceGainPct && drawdownPct >= drawdownCloseGivebackPct
}

const (
	drawdownCheckEvery = time.Minute
	drawdownMarkEvery  = time.Second
	// drawdownConfirmGap keeps a close that keeps failing from asking the exchange
	// for positions every second.
	drawdownConfirmGap = 5 * time.Second
)

// heldPosition is what the monitor needs of a position between reads.
type heldPosition struct {
	symbol     string
	side       string
	entryPrice float64
	leverage   int
}

// startDrawdownMonitor starts drawdown monitoring. Positions are read from the
// exchange every minute, and that check alone decides to close. Between reads the
// live mark price keeps each position's peak current and can call that check within
// a second of a giveback. If the stream stops, the minute check carries on alone.
func (at *AutoTrader) startDrawdownMonitor() {
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()

		check := time.NewTicker(drawdownCheckEvery)
		defer check.Stop()
		marks := time.NewTicker(drawdownMarkEvery)
		defer marks.Stop()

		logger.Info("📊 Started position drawdown monitoring (every minute, and on live mark prices)")

		var held []heldPosition
		var lastCheck time.Time
		for {
			select {
			case <-check.C:
				held = at.checkPositionDrawdown(held)
				lastCheck = time.Now()
			case <-marks.C:
				if at.marksCallForCheck(held) && time.Since(lastCheck) >= drawdownConfirmGap {
					held = at.checkPositionDrawdown(held)
					lastCheck = time.Now()
				}
			case <-at.stopMonitorCh:
				logger.Info("⏹ Stopped position drawdown monitoring")
				return
			}
		}
	}()
}

// marksCallForCheck applies live mark prices to the held positions' peaks and
// reports whether any of them now looks due to close.
func (at *AutoTrader) marksCallForCheck(held []heldPosition) bool {
	due := false
	for _, pos := range held {
		mark, ok := market.MarkPrice(at.venue, pos.symbol)
		if !ok {
			continue
		}
		pricePnLPct, _, _, drawdownPct := at.trackDrawdown(pos, mark)
		if shouldDrawdownClose(pricePnLPct, drawdownPct) {
			due = true
		}
	}
	return due
}

// trackDrawdown works out where a position stands at a mark price and remembers a
// new peak.
func (at *AutoTrader) trackDrawdown(pos heldPosition, markPrice float64) (pricePnLPct, currentPnLPct, peakPnLPct, drawdownPct float64) {
	// Price-basis move drives the close decision so the trigger point does
	// not tighten as leverage grows; the margin-basis (leveraged) value is
	// only kept for the peak cache shown alongside margin-based PnL% in
	// prompts.
	if pos.side == "long" {
		pricePnLPct = ((markPrice - pos.entryPrice) / pos.entryPrice) * 100
	} else {
		pricePnLPct = ((pos.entryPrice - markPrice) / pos.entryPrice) * 100
	}
	currentPnLPct = pricePnLPct * float64(pos.leverage)

	// Construct unique position identifier (distinguish long/short)
	posKey := pos.symbol + "_" + pos.side

	// Get historical peak profit for this position
	at.peakPnLCacheMutex.RLock()
	peakPnLPct, exists := at.peakPnLCache[posKey]
	at.peakPnLCacheMutex.RUnlock()

	if !exists {
		// If no historical peak record, use current P&L as initial value
		peakPnLPct = currentPnLPct
	}
	at.UpdatePeakPnL(pos.symbol, pos.side, currentPnLPct)

	// Calculate drawdown (magnitude of decline from peak)
	if peakPnLPct > 0 && currentPnLPct < peakPnLPct {
		drawdownPct = ((peakPnLPct - currentPnLPct) / peakPnLPct) * 100
	}
	return pricePnLPct, currentPnLPct, peakPnLPct, drawdownPct
}

// checkPositionDrawdown reads positions from the exchange and closes what has given
// back too much. It returns the positions for the live marks to watch until the next
// read, or the previous ones if the read failed.
func (at *AutoTrader) checkPositionDrawdown(previous []heldPosition) []heldPosition {
	// Get current positions
	positions, err := at.trader.GetPositions()
	if err != nil {
		logger.Infof("❌ Drawdown monitoring: failed to get positions: %v", err)
		return previous
	}

	held := make([]heldPosition, 0, len(positions))
	heldKeys := make(map[string]bool, len(positions))
	for _, row := range positions {
		symbol := row["symbol"].(string)
		side := row["side"].(string)
		entryPrice := row["entryPrice"].(float64)
		markPrice := row["markPrice"].(float64)
		heldKeys[symbol+"_"+side] = true

		// Guard: skip if entry price is zero (prevents division by zero panic)
		if entryPrice <= 0 {
			logger.Warnf("⚠️ Drawdown monitoring: %s %s has zero entry price, skipping", symbol, side)
			continue
		}

		// Calculate current P&L percentage
		leverage := 10 // Default value
		if lev, ok := row["leverage"].(float64); ok {
			leverage = int(lev)
		}
		pos := heldPosition{symbol: symbol, side: side, entryPrice: entryPrice, leverage: leverage}
		pricePnLPct, currentPnLPct, peakPnLPct, drawdownPct := at.trackDrawdown(pos, markPrice)

		// Check close position condition: price move > +5% and drawdown >= 40%
		if shouldDrawdownClose(pricePnLPct, drawdownPct) {
			logger.Infof("🚨 Drawdown close position condition triggered: %s %s | Price move: %.2f%% | Current profit: %.2f%% | Peak profit: %.2f%% | Drawdown: %.2f%%",
				symbol, side, pricePnLPct, currentPnLPct, peakPnLPct, drawdownPct)

			// Execute close position
			if err := at.emergencyClosePosition(symbol, side); err != nil {
				logger.Infof("❌ Drawdown close position failed (%s %s): %v", symbol, side, err)
			} else {
				logger.Infof("✅ Drawdown close position succeeded: %s %s", symbol, side)
				// Clear cache for this position after closing
				at.ClearPeakPnLCache(symbol, side)
				continue
			}
		} else if pricePnLPct > drawdownClosePriceGainPct {
			// Record situations close to close position condition (for debugging)
			logger.Infof("📊 Drawdown monitoring: %s %s | Price move: %.2f%% | Profit: %.2f%% | Peak: %.2f%% | Drawdown: %.2f%%",
				symbol, side, pricePnLPct, currentPnLPct, peakPnLPct, drawdownPct)
		}
		market.WatchMark(at.venue, symbol)
		held = append(held, pos)
	}
	at.forgetPeaksNotHeld(heldKeys)
	return held
}

// emergencyClosePosition emergency close position function
func (at *AutoTrader) emergencyClosePosition(symbol, side string) error {
	switch side {
	case "long":
		order, err := at.trader.CloseLong(symbol, 0) // 0 = close all
		if err != nil {
			return err
		}
		logger.Infof("✅ Emergency close long position succeeded, order ID: %v", order["orderId"])
	case "short":
		order, err := at.trader.CloseShort(symbol, 0) // 0 = close all
		if err != nil {
			return err
		}
		logger.Infof("✅ Emergency close short position succeeded, order ID: %v", order["orderId"])
	default:
		return fmt.Errorf("unknown position direction: %s", side)
	}

	return nil
}

// GetPeakPnLCache gets peak profit cache
func (at *AutoTrader) GetPeakPnLCache() map[string]float64 {
	at.peakPnLCacheMutex.RLock()
	defer at.peakPnLCacheMutex.RUnlock()

	// Return a copy of the cache
	cache := make(map[string]float64)
	for k, v := range at.peakPnLCache {
		cache[k] = v
	}
	return cache
}

// peakSaveStep is how far a peak rises before it is written down again. Marks arrive every
// second, and a restart can afford to lose this much.
const peakSaveStep = 1.0

// UpdatePeakPnL updates peak profit cache
func (at *AutoTrader) UpdatePeakPnL(symbol, side string, currentPnLPct float64) {
	posKey := symbol + "_" + side
	at.peakPnLCacheMutex.Lock()
	peak, exists := at.peakPnLCache[posKey]
	raised := !exists || currentPnLPct > peak
	if raised {
		at.peakPnLCache[posKey] = currentPnLPct
	}
	// Only a profit can be given back, so a position that has made none needs no record.
	save := raised && currentPnLPct > 0 && at.peakPnLSaved != nil && currentPnLPct >= at.peakPnLSaved[posKey]+peakSaveStep
	if save {
		at.peakPnLSaved[posKey] = currentPnLPct
	}
	at.peakPnLCacheMutex.Unlock()

	if save && at.stores != nil {
		err := at.stores.Peaks.Save(PositionPeak{TraderID: at.id, Symbol: symbol, Side: side, PeakPnLPct: currentPnLPct})
		if err != nil {
			logger.Warnf("Trader %s: save the peak of %s %s: %v", at.id, symbol, side, err)
		}
	}
}

// ClearPeakPnLCache clears peak cache for specified position
func (at *AutoTrader) ClearPeakPnLCache(symbol, side string) {
	posKey := symbol + "_" + side
	at.peakPnLCacheMutex.Lock()
	delete(at.peakPnLCache, posKey)
	delete(at.peakPnLSaved, posKey)
	at.peakPnLCacheMutex.Unlock()

	if at.stores != nil {
		if err := at.stores.Peaks.Delete(at.id, symbol, side); err != nil {
			logger.Warnf("Trader %s: forget the peak of %s %s: %v", at.id, symbol, side, err)
		}
	}
}

// loadStoredPeaks picks up what the monitor knew before a restart.
func (at *AutoTrader) loadStoredPeaks() {
	if at.stores == nil {
		return
	}
	peaks, err := at.stores.Peaks.Load(at.id)
	if err != nil {
		logger.Warnf("Trader %s: read stored peaks: %v", at.id, err)
		return
	}
	at.peakPnLCacheMutex.Lock()
	defer at.peakPnLCacheMutex.Unlock()
	for _, peak := range peaks {
		posKey := peak.Symbol + "_" + peak.Side
		if current, known := at.peakPnLCache[posKey]; !known || peak.PeakPnLPct > current {
			at.peakPnLCache[posKey] = peak.PeakPnLPct
		}
		at.peakPnLSaved[posKey] = peak.PeakPnLPct
	}
}

// forgetPeaksNotHeld drops the peaks of positions that are gone, closed by a stop or by
// hand, so a later position on the same coin and side does not inherit one.
func (at *AutoTrader) forgetPeaksNotHeld(held map[string]bool) {
	at.peakPnLCacheMutex.RLock()
	var gone []string
	for posKey := range at.peakPnLCache {
		if !held[posKey] {
			gone = append(gone, posKey)
		}
	}
	at.peakPnLCacheMutex.RUnlock()
	for _, posKey := range gone {
		if i := strings.LastIndex(posKey, "_"); i > 0 {
			at.ClearPeakPnLCache(posKey[:i], posKey[i+1:])
		}
	}
}

// ============================================================================
// Risk Control Helpers
// ============================================================================

// riskBlock is a rule stopping an order before it is placed, rather than a failure to
// place it, so it is recorded as blocked and raises no alert.
type riskBlock struct{ reason string }

func (b riskBlock) Error() string { return b.reason }

// isBTCETH checks if a symbol is BTC or ETH, and not one that only starts the same, such
// as ETHFI.
func isBTCETH(symbol string) bool {
	symbol = strings.ToUpper(symbol)
	return symbol == "BTCUSDT" || symbol == "ETHUSDT"
}

// isMajorAsset returns true for assets that should use the BTC/ETH higher
// position-value tier rather than the altcoin (1x equity) tier.
func isMajorAsset(symbol string) bool {
	return isBTCETH(symbol)
}

// enforcePositionValueRatio checks and enforces position value ratio limits (CODE ENFORCED)
// Returns the adjusted position size (capped if necessary) and whether the position was capped
// positionSizeUSD: the original position size in USD
// equity: the account equity
// symbol: the trading symbol
func (at *AutoTrader) enforcePositionValueRatio(positionSizeUSD float64, equity float64, symbol string) (float64, bool) {
	if at.config.Agent == nil {
		return positionSizeUSD, false
	}

	riskControl := at.config.Agent.RiskControl

	// Get the appropriate position value ratio limit. BTC/ETH use the higher
	// tier; altcoins use the lower tier.
	var maxPositionValueRatio float64
	if isMajorAsset(symbol) {
		maxPositionValueRatio = riskControl.BTCETHMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 5.0 // Default: 5x for BTC/ETH
		}
	} else {
		maxPositionValueRatio = riskControl.AltcoinMaxPositionValueRatio
		if maxPositionValueRatio <= 0 {
			maxPositionValueRatio = 1.0 // Default: 1x for altcoins
		}
	}

	// Calculate max allowed position value = equity × ratio
	maxPositionValue := equity * maxPositionValueRatio

	// Check if position size exceeds limit
	if positionSizeUSD > maxPositionValue {
		logger.Infof("  ⚠️ [RISK CONTROL] Position %.2f USDT exceeds limit (equity %.2f × %.1fx = %.2f USDT max for %s), capping",
			positionSizeUSD, equity, maxPositionValueRatio, maxPositionValue, symbol)
		return maxPositionValue, true
	}

	return positionSizeUSD, false
}

// enforceMinPositionSize checks minimum position size (CODE ENFORCED)
func (at *AutoTrader) enforceMinPositionSize(positionSizeUSD float64) error {
	if at.config.Agent == nil {
		return nil
	}

	minSize := at.config.Agent.RiskControl.MinPositionSize
	if minSize <= 0 {
		minSize = 12 // Default: 12 USDT
	}

	if positionSizeUSD < minSize {
		return riskBlock{fmt.Sprintf("❌ [RISK CONTROL] Position %.2f USDT below minimum (%.2f USDT)", positionSizeUSD, minSize)}
	}
	return nil
}

// enforceMaxPositions checks maximum positions count (CODE ENFORCED)
func (at *AutoTrader) enforceMaxPositions(currentPositionCount int) error {
	if at.config.Agent == nil {
		return nil
	}

	maxPositions := at.config.Agent.RiskControl.MaxPositions
	if maxPositions <= 0 {
		maxPositions = 3 // Default: 3 positions
	}

	if currentPositionCount >= maxPositions {
		return riskBlock{fmt.Sprintf("❌ [RISK CONTROL] Already at max positions (%d/%d)", currentPositionCount, maxPositions)}
	}
	return nil
}

// enforceEntryRules checks what a decision must meet to open, at the price it would open at (CODE ENFORCED)
func (at *AutoTrader) enforceEntryRules(decision *traderagent.Decision, price float64) error {
	if at.config.Agent == nil {
		return nil
	}
	risk := at.config.Agent.RiskControl
	if decision.Confidence < risk.MinConfidence {
		return riskBlock{fmt.Sprintf("❌ [RISK CONTROL] Confidence %d below minimum (%d)", decision.Confidence, risk.MinConfidence)}
	}
	if ratio := rewardToRisk(decision.Action, price, decision.StopLoss, decision.TakeProfit); ratio < risk.MinRiskRewardRatio {
		return riskBlock{fmt.Sprintf("❌ [RISK CONTROL] Reward to risk %.2f at price %.8f below minimum (%.2f)", ratio, price, risk.MinRiskRewardRatio)}
	}
	return nil
}

// rewardToRisk is what an open stands to gain against what it stands to lose, at a price.
func rewardToRisk(action string, price, stopLoss, takeProfit float64) float64 {
	risk, reward := price-stopLoss, takeProfit-price
	if action == "open_short" {
		risk, reward = stopLoss-price, price-takeProfit
	}
	if risk <= 0 {
		return 0
	}
	return reward / risk
}

// enforceMarginUsage caps a position to the margin left under the limit once it is open (CODE ENFORCED)
func (at *AutoTrader) enforceMarginUsage(positionSizeUSD float64, leverage int, equity float64, positions []map[string]interface{}) (float64, bool) {
	if at.config.Agent == nil || leverage <= 0 {
		return positionSizeUSD, false
	}
	limit := at.config.Agent.RiskControl.MaxMarginUsage
	if limit <= 0 {
		return positionSizeUSD, false
	}
	left := math.Max(equity*limit-marginInUse(positions), 0)
	maxSize := left * float64(leverage)
	if positionSizeUSD > maxSize {
		logger.Infof("  ⚠️ [RISK CONTROL] Position %.2f USDT would take margin past %.0f%% of equity %.2f, capping to %.2f",
			positionSizeUSD, limit*100, equity, maxSize)
		return maxSize, true
	}
	return positionSizeUSD, false
}

// marginInUse estimates the margin open positions hold.
func marginInUse(positions []map[string]interface{}) float64 {
	total := 0.0
	for _, pos := range positions {
		quantity, _ := pos["positionAmt"].(float64)
		markPrice, _ := pos["markPrice"].(float64)
		leverage := 10.0 // Default value
		if lev, ok := pos["leverage"].(float64); ok && lev > 0 {
			leverage = lev
		}
		total += math.Abs(quantity) * markPrice / leverage
	}
	return total
}

// checkLossLimits halts a trader whose account has lost more than its settings allow (CODE
// ENFORCED). It reads the snapshots each cycle stores, so a restart forgets nothing, and a
// resume starts both counts over.
func (at *AutoTrader) checkLossLimits(equity float64) {
	if at.config.Agent == nil || at.stores == nil || equity <= 0 {
		return
	}
	risk := at.config.Agent.RiskControl
	if risk.MaxDrawdownPct > 0 {
		if peak := at.storedPeakEquity(); peak > 0 {
			if drawdown := (peak - equity) / peak * 100; drawdown >= risk.MaxDrawdownPct {
				at.tripHalt(fmt.Sprintf("max drawdown exceeded: %.2f%%", drawdown), nil)
				return
			}
		}
	}
	if risk.MaxDailyLossPct > 0 {
		now := time.Now().UTC()
		since := now.Truncate(24 * time.Hour)
		at.runtimeHealthMu.RLock()
		if at.resumedAt.After(since) {
			since = at.resumedAt
		}
		at.runtimeHealthMu.RUnlock()
		start, err := at.stores.Equity.FirstSince(at.id, since)
		if err != nil {
			logger.Warnf("Trader %s: read today's first equity: %v", at.id, err)
			return
		}
		if start > 0 {
			if lost := (start - equity) / start * 100; lost >= risk.MaxDailyLossPct {
				until := nextUTCDay(now)
				at.tripHalt(fmt.Sprintf("daily loss limit exceeded: %.2f%%", lost), &until)
			}
		}
	}
}
