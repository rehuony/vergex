package trader

import (
	"time"
	"vergex/internal/logger"
)

// Halt is a hard stop that has tripped. It lives on the trader's row, so a restart or a
// change of settings cannot quietly start trading again. With Until it ends by itself
// then; without it, only the user ends it.
type Halt struct {
	At     time.Time  `json:"at"`
	Reason string     `json:"reason"`
	Until  *time.Time `json:"until,omitempty"`
}

// Halt returns the trader's stored halt, if it has one.
func (t *Trader) Halt() *Halt {
	if t.HaltedAt == nil {
		return nil
	}
	return &Halt{At: *t.HaltedAt, Reason: t.HaltedReason, Until: t.HaltedUntil}
}

// nextUTCDay is when a limit counted per UTC day starts over.
func nextUTCDay(now time.Time) time.Time {
	return now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
}

// ActiveHalt returns the halt in force, ending one whose time has come.
func (at *AutoTrader) ActiveHalt() *Halt {
	at.runtimeHealthMu.Lock()
	defer at.runtimeHealthMu.Unlock()
	if at.halt == nil {
		return nil
	}
	if at.halt.Until != nil && !time.Now().Before(*at.halt.Until) {
		at.halt = nil
		if at.stores != nil {
			if err := at.stores.Traders.ClearHalt(at.id, nil); err != nil {
				logger.Warnf("Trader %s: clear a halt that ended: %v", at.id, err)
			}
		}
		return nil
	}
	return at.halt
}

// tripHalt records a hard stop. A halt already in force stays, with its first reason.
func (at *AutoTrader) tripHalt(reason string, until *time.Time) {
	at.runtimeHealthMu.Lock()
	defer at.runtimeHealthMu.Unlock()
	if at.halt != nil {
		return
	}
	at.halt = &Halt{At: time.Now().UTC(), Reason: reason, Until: until}
	logger.Errorf("Trader %s halted: %s", at.id, reason)
	if at.stores != nil {
		if err := at.stores.Traders.SetHalt(at.id, *at.halt); err != nil {
			// It still holds until this process ends, and the log says why it may not after.
			logger.Errorf("Trader %s: save a halt: %v", at.id, err)
		}
	}
}

// Resume ends a halt at the user's word. Peak equity starts over from here.
func (at *AutoTrader) Resume() error {
	now := time.Now().UTC()
	if at.stores != nil {
		if err := at.stores.Traders.ClearHalt(at.id, &now); err != nil {
			return err
		}
	}
	at.runtimeHealthMu.Lock()
	at.halt = nil
	at.resumedAt = now
	at.runtimeHealthMu.Unlock()

	at.exitPending.Store(false)
	if at.gridState != nil {
		at.gridState.mu.Lock()
		at.gridState.PeakEquity = 0
		at.gridState.IsPaused = false
		at.gridState.mu.Unlock()
	}
	return nil
}

// storedPeakEquity is the highest equity recorded since the trader was last resumed.
func (at *AutoTrader) storedPeakEquity() float64 {
	if at.stores == nil {
		return 0
	}
	at.runtimeHealthMu.RLock()
	since := at.resumedAt
	at.runtimeHealthMu.RUnlock()
	peak, err := at.stores.Equity.PeakSince(at.id, since)
	if err != nil {
		logger.Warnf("Trader %s: read peak equity: %v", at.id, err)
		return 0
	}
	return peak
}
