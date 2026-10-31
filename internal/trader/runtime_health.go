package trader

import (
	"time"
)

// Runtime health state written by the run-loop goroutine and read by the API
// layer (GetStatus). Everything here goes through runtimeHealthMu so the
// dashboard can poll without racing the trading loop.

func (at *AutoTrader) checkTradingAuthorization() error {
	checker, ok := at.trader.(interface{ CheckTradingAuthorization() error })
	if !ok {
		return nil
	}
	err := checker.CheckTradingAuthorization()
	at.runtimeHealthMu.Lock()
	defer at.runtimeHealthMu.Unlock()
	at.tradingBlocked = err != nil
	at.tradingError = ""
	if err != nil {
		at.tradingError = err.Error()
	}
	at.tradingCheckedAt = time.Now().UTC()
	return err
}

func (at *AutoTrader) setSafeMode(active bool, reason string) {
	at.runtimeHealthMu.Lock()
	at.safeMode = active
	at.safeModeReason = reason
	at.runtimeHealthMu.Unlock()
}

func (at *AutoTrader) isSafeMode() bool {
	at.runtimeHealthMu.RLock()
	defer at.runtimeHealthMu.RUnlock()
	return at.safeMode
}

func (at *AutoTrader) safeModeState() (bool, string) {
	at.runtimeHealthMu.RLock()
	defer at.runtimeHealthMu.RUnlock()
	return at.safeMode, at.safeModeReason
}
