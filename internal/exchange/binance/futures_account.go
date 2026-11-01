package binance

import (
	"context"
	"fmt"
	"strconv"
	"time"
	"vergex/internal/logger"
	"vergex/internal/trading"
)

// GetBalance gets account balance (with cache)
func (t *FuturesTrader) GetBalance() (map[string]interface{}, error) {
	// First check if cache is valid
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		logger.Infof("✓ Using cached account balance (cache age: %.1f seconds ago)", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	// Cache expired or doesn't exist, call API
	logger.Infof("🔄 Cache expired, calling Binance API to get account balance...")
	account, err := t.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		logger.Infof("❌ Binance API call failed: %v", err)
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	result := make(map[string]interface{})
	for field, value := range map[string]string{
		"totalWalletBalance":    account.TotalWalletBalance,
		"availableBalance":      account.AvailableBalance,
		"totalUnrealizedProfit": account.TotalUnrealizedProfit,
	} {
		parsed, parseErr := trading.ParseFloatField(field, value)
		if parseErr != nil {
			return nil, parseErr
		}
		result[field] = parsed
	}

	logger.Infof("✓ Binance API returned: total balance=%s, available=%s, unrealized PnL=%s",
		account.TotalWalletBalance,
		account.AvailableBalance,
		account.TotalUnrealizedProfit)

	// Update cache
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
}

// GetClosedPnL retrieves recent closing trades from Binance Futures
// Note: Binance does NOT have a position history API, only trade history.
// This returns individual closing trades (realizedPnl != 0) for real-time position closure detection.
// NOT suitable for historical position reconstruction - use only for matching recent closures.
func (t *FuturesTrader) GetClosedPnL(startTime time.Time, limit int) ([]trading.ClosedPnLRecord, error) {
	trades, err := t.GetTrades(startTime, limit)
	if err != nil {
		return nil, err
	}

	// Filter only closing trades (realizedPnl != 0) and convert to ClosedPnLRecord
	var records []trading.ClosedPnLRecord
	for _, trade := range trades {
		if trade.RealizedPnL == 0 {
			continue // Skip opening trades
		}

		// Determine side from trade
		side := "long"
		if trade.PositionSide == "SHORT" || trade.PositionSide == "short" {
			side = "short"
		} else if trade.PositionSide == "BOTH" || trade.PositionSide == "" {
			// One-way mode: selling closes long, buying closes short
			if trade.Side == "SELL" || trade.Side == "Sell" {
				side = "long"
			} else {
				side = "short"
			}
		}

		// Calculate entry price from PnL (mathematically accurate for this trade)
		var entryPrice float64
		if trade.Quantity > 0 {
			if side == "long" {
				entryPrice = trade.Price - trade.RealizedPnL/trade.Quantity
			} else {
				entryPrice = trade.Price + trade.RealizedPnL/trade.Quantity
			}
		}

		records = append(records, trading.ClosedPnLRecord{
			Symbol:      trade.Symbol,
			Side:        side,
			EntryPrice:  entryPrice,
			ExitPrice:   trade.Price,
			Quantity:    trade.Quantity,
			RealizedPnL: trade.RealizedPnL,
			Fee:         trade.Fee,
			ExitTime:    trade.Time,
			EntryTime:   trade.Time, // Approximate
			OrderID:     trade.TradeID,
			ExchangeID:  trade.TradeID,
			CloseType:   "unknown",
		})
	}

	return records, nil
}

// GetTrades retrieves trade history from Binance Futures using Income API
// Note: Income API has delays (~minutes), for real-time use GetTradesForSymbol instead
func (t *FuturesTrader) GetTrades(startTime time.Time, limit int) ([]trading.TradeRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Use Income API to get REALIZED_PNL records (all symbols)
	incomes, err := t.client.NewGetIncomeHistoryService().
		IncomeType("REALIZED_PNL").
		StartTime(startTime.UnixMilli()).
		Limit(int64(limit)).
		Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get income history: %w", err)
	}

	var trades []trading.TradeRecord
	for _, income := range incomes {
		pnl, _ := strconv.ParseFloat(income.Income, 64)
		if pnl == 0 {
			continue // Skip zero PnL records
		}

		// Income API doesn't provide full trade details, create a minimal record
		// This is mainly used for detecting recent closures, not historical reconstruction
		trade := trading.TradeRecord{
			TradeID:     strconv.FormatInt(income.TranID, 10),
			Symbol:      income.Symbol,
			RealizedPnL: pnl,
			Time:        time.UnixMilli(income.Time).UTC(),
			// Note: Income API doesn't provide price, quantity, side, fee
			// For accurate data, use GetTradesForSymbol with specific symbol
		}
		trades = append(trades, trade)
	}

	return trades, nil
}

func timeFromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// incomePage is the most income records one read returns.
const incomePage = 1000

// GetCommissionSymbols returns symbols that have new commission records since lastSyncTime
// COMMISSION income is generated for every trade, so this is more reliable than REALIZED_PNL
func (t *FuturesTrader) GetCommissionSymbols(lastSyncTime time.Time) ([]string, error) {
	symbols, err := t.incomeSymbols("COMMISSION", lastSyncTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get commission history: %w", err)
	}
	return symbols, nil
}

// GetPnLSymbols returns symbols that have REALIZED_PNL records since lastSyncTime
// This is a fallback when COMMISSION detection fails (VIP users, BNB fee discount)
func (t *FuturesTrader) GetPnLSymbols(lastSyncTime time.Time) ([]string, error) {
	symbols, err := t.incomeSymbols("REALIZED_PNL", lastSyncTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get PnL history: %w", err)
	}
	return symbols, nil
}

// incomeSymbols lists the symbols with income of a kind since a time.
func (t *FuturesTrader) incomeSymbols(kind string, since time.Time) ([]string, error) {
	found := make(map[string]bool)
	if err := t.readIncome(kind, since.UnixMilli(), time.Now().UnixMilli(), found); err != nil {
		return nil, err
	}
	symbols := make([]string, 0, len(found))
	for symbol := range found {
		symbols = append(symbols, symbol)
	}
	return symbols, nil
}

// readIncome adds the symbols with income in a span. Binance does not say which end of a
// span a full page comes from, so a full page is read again in halves.
func (t *FuturesTrader) readIncome(kind string, start, end int64, found map[string]bool) error {
	incomes, err := t.client.NewGetIncomeHistoryService().IncomeType(kind).StartTime(start).EndTime(end).Limit(incomePage).Do(context.Background())
	if err != nil {
		return err
	}
	if len(incomes) >= incomePage {
		if end <= start {
			return fmt.Errorf("a full page of %s income at one moment (%d) cannot be split", kind, start)
		}
		middle := start + (end-start)/2
		if err := t.readIncome(kind, start, middle, found); err != nil {
			return err
		}
		return t.readIncome(kind, middle+1, end, found)
	}
	for _, income := range incomes {
		if income.Symbol != "" {
			found[income.Symbol] = true
		}
	}
	return nil
}
