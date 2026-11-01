package trader

import (
	"fmt"
	"strings"
	"vergex/internal/logger"
	"vergex/internal/trading"
)

type positionCacheInvalidator interface {
	InvalidatePositionCache()
}

func validateProtectionPrices(action string, marketPrice, stopLoss, takeProfit float64) error {
	if marketPrice <= 0 || stopLoss <= 0 {
		return fmt.Errorf("market price and stop loss must be positive")
	}
	switch action {
	case "open_long":
		if stopLoss >= marketPrice {
			return fmt.Errorf("long stop loss %.8f must be below market price %.8f", stopLoss, marketPrice)
		}
		if takeProfit <= marketPrice {
			return fmt.Errorf("long take profit %.8f must be above market price %.8f", takeProfit, marketPrice)
		}
	case "open_short":
		if stopLoss <= marketPrice {
			return fmt.Errorf("short stop loss %.8f must be above market price %.8f", stopLoss, marketPrice)
		}
		if takeProfit <= 0 || takeProfit >= marketPrice {
			return fmt.Errorf("short take profit %.8f must be positive and below market price %.8f", takeProfit, marketPrice)
		}
	default:
		return fmt.Errorf("unsupported open action %q", action)
	}
	return nil
}

func (at *AutoTrader) closeUnprotectedPosition(symbol, side string, quantity float64, protectionErr error) error {
	logger.Infof("  🚨 %v; emergency-closing %s %s", protectionErr, symbol, side)
	if closeErr := at.emergencyClosePositionAndVerify(symbol, side, quantity); closeErr != nil {
		return fmt.Errorf("%w; emergency close failed: %v", protectionErr, closeErr)
	}
	return fmt.Errorf("%w; opened position was emergency-closed", protectionErr)
}

func (at *AutoTrader) emergencyClosePositionAndVerify(symbol, side string, quantity float64) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		var err error
		if side == "long" {
			_, err = at.trader.CloseLong(symbol, quantity)
		} else if side == "short" {
			_, err = at.trader.CloseShort(symbol, quantity)
		} else {
			return fmt.Errorf("unknown position direction: %s", side)
		}
		if err != nil {
			lastErr = err
		} else {
			positions, err := getFreshPositions(at.trader)
			if err != nil {
				lastErr = fmt.Errorf("verify flat position: %w", err)
			} else {
				residual := false
				for _, pos := range positions {
					if universeBaseKey(fmt.Sprint(pos["symbol"])) == universeBaseKey(symbol) && strings.EqualFold(fmt.Sprint(pos["side"]), side) {
						residual = true
						break
					}
				}
				if !residual {
					if err := at.trader.CancelAllOrders(symbol); err != nil {
						return fmt.Errorf("position is flat but protection-order cleanup failed: %w", err)
					}
					openOrders, err := at.trader.GetOpenOrders(symbol)
					if err != nil {
						return fmt.Errorf("position is flat but protection-order verification failed: %w", err)
					}
					if len(openOrders) != 0 {
						return fmt.Errorf("position is flat but %d open orders remain for %s", len(openOrders), symbol)
					}
					return nil
				}
				lastErr = fmt.Errorf("residual %s position remains after close attempt %d", side, attempt)
			}
		}
	}
	return lastErr
}

func getFreshPositions(tr trading.Trader) ([]map[string]interface{}, error) {
	if invalidator, ok := tr.(positionCacheInvalidator); ok {
		invalidator.InvalidatePositionCache()
	}
	return tr.GetPositions()
}
