package trader

import (
	"errors"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/database/testdb"
	"vergex/internal/trader/kind"
)

// haltTrader is a saved grid trader with an exchange that counts what is done to it.
func haltTrader(t *testing.T) (*AutoTrader, *Stores, *haltExchange) {
	t.Helper()
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	saved := testdb.AddTrader(t, st.DB, &testdb.Trader{UserID: user, Name: "Grid", Kind: "grid"})
	exchange := &haltExchange{equity: 1000, positions: []map[string]any{{"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.01, "markPrice": 80000.0, "leverage": 5.0}}}
	grid := &kind.GridConfig{Symbol: "BTCUSDT", TotalInvestment: 1000, Leverage: 5, MaxDrawdownPct: 10, DailyLossLimitPct: 5}
	return &AutoTrader{id: saved.ID, stores: st, trader: exchange, isRunning: true, config: AutoTraderConfig{Grid: grid}}, st, exchange
}

type haltExchange struct {
	account.Trading
	equity     float64
	positions  []map[string]any
	closes     int
	cancels    int
	priceReads int
	closeErr   error
	cancelErr  error
}

func (e *haltExchange) GetBalance() (map[string]any, error) {
	return map[string]any{"totalWalletBalance": e.equity, "totalUnrealizedProfit": 0.0, "availableBalance": e.equity}, nil
}
func (e *haltExchange) GetPositions() ([]map[string]any, error) { return e.positions, nil }
func (e *haltExchange) CancelAllOrders(string) error {
	e.cancels++
	return e.cancelErr
}
func (e *haltExchange) GetMarketPrice(string) (float64, error) {
	e.priceReads++
	return 80000, nil
}
func (e *haltExchange) CloseLong(string, float64) (map[string]any, error) {
	if e.closeErr != nil {
		return nil, e.closeErr
	}
	e.closes++
	e.positions = nil
	return map[string]any{}, nil
}

func reload(t *testing.T, st *Stores, at *AutoTrader) *Halt {
	t.Helper()
	var saved Trader
	if err := st.DB.First(&saved, "id = ?", at.id).Error; err != nil {
		t.Fatal(err)
	}
	return saved.Halt()
}

func TestAHaltOutlivesARestartAndEndsOnlyByHand(t *testing.T) {
	at, st, exchange := haltTrader(t)
	at.tripHalt("max drawdown exceeded", nil)

	saved := reload(t, st, at)
	if saved == nil || saved.Reason != "max drawdown exceeded" || saved.Until != nil {
		t.Fatalf("saved halt: %+v", saved)
	}

	// A restart rebuilds the trader from its row, with no grid state.
	restarted := &AutoTrader{id: at.id, stores: st, trader: exchange, isRunning: true, config: at.config, halt: saved}
	if err := restarted.RunGridCycle(); err != nil {
		t.Fatal(err)
	}
	if restarted.gridState != nil || exchange.priceReads != 0 {
		t.Fatal("a halted grid started up again")
	}

	if err := restarted.Resume(); err != nil {
		t.Fatal(err)
	}
	if restarted.ActiveHalt() != nil || reload(t, st, at) != nil {
		t.Fatal("resuming did not end the halt")
	}
	var row Trader
	if err := st.DB.First(&row, "id = ?", at.id).Error; err != nil || row.ResumedAt == nil {
		t.Fatalf("resumed_at: %+v, %v", row.ResumedAt, err)
	}
}

func TestAHaltWithAnEndTimeEndsByItself(t *testing.T) {
	at, st, _ := haltTrader(t)
	future := time.Now().Add(time.Hour)
	at.tripHalt("daily loss limit exceeded", &future)
	if at.ActiveHalt() == nil {
		t.Fatal("not yet")
	}
	past := time.Now().Add(-time.Minute)
	at.halt = &Halt{At: past, Reason: "daily loss limit exceeded", Until: &past}
	if at.ActiveHalt() != nil || reload(t, st, at) != nil {
		t.Fatal("a halt past its end time is still in force")
	}
}

func TestPeakEquityComesBackFromSnapshotsAndStartsOverOnResume(t *testing.T) {
	at, st, exchange := haltTrader(t)
	for _, equity := range []float64{1000, 1500, 1200} {
		// Snapshots are keyed by trader and time, so each needs a time of its own.
		time.Sleep(time.Millisecond)
		if err := st.Equity.Save(&EquitySnapshot{TraderID: at.id, TotalEquity: equity, Timestamp: time.Now().Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	at.gridState = NewGridState(at.config.Grid)
	at.gridState.PeakEquity = at.storedPeakEquity()
	exchange.equity = 1300 // 13.3% below 1500
	if exceeded, drawdown := at.checkMaxDrawdown(); !exceeded || drawdown < 13 {
		t.Fatalf("drawdown from the stored peak: %v, %.2f", exceeded, drawdown)
	}
	if err := at.Resume(); err != nil {
		t.Fatal(err)
	}
	at.gridState.PeakEquity = at.storedPeakEquity()
	if exceeded, _ := at.checkMaxDrawdown(); exceeded {
		t.Fatal("a resumed trader tripped on the old peak")
	}
}

func TestAFailedExitIsTriedAgainWhileHalted(t *testing.T) {
	at, _, exchange := haltTrader(t)
	at.gridState = NewGridState(at.config.Grid)
	exchange.closeErr = errors.New("exchange down")
	at.tripHalt("max drawdown exceeded", nil)
	if err := at.emergencyExit("max drawdown exceeded"); err == nil {
		t.Fatal("a failed close went unreported")
	}
	if !at.exitPending.Load() {
		t.Fatal("nothing will try again")
	}
	exchange.closeErr = nil
	if err := at.RunGridCycle(); err != nil {
		t.Fatal(err)
	}
	if exchange.closes != 1 || at.exitPending.Load() {
		t.Fatalf("closes %d, pending %v", exchange.closes, at.exitPending.Load())
	}
	if err := at.RunGridCycle(); err != nil {
		t.Fatal(err)
	}
	if exchange.closes != 1 {
		t.Fatal("a finished exit ran again")
	}
}

func TestTheDailyLimitReadsWhatTheLedgerRealized(t *testing.T) {
	at, st, _ := haltTrader(t)
	at.gridState = NewGridState(at.config.Grid)
	if exceeded, _ := at.checkDailyLossLimit(); exceeded {
		t.Fatal("nothing has been lost yet")
	}

	ledger := accounttest.NewLedger(t, st.DB)
	now := time.Now()
	ledger.Open(at.id, "BTCUSDT", account.SideLong, 1, 100, now.Add(-time.Minute))
	ledger.Close(at.id, "BTCUSDT", account.SideLong, 1, 40, -60, now) // 6% of 1000
	exceeded, loss := at.checkDailyLossLimit()
	if !exceeded || loss < 5.9 {
		t.Fatalf("exceeded %v, loss %.2f", exceeded, loss)
	}
	if at.gridState.DailyPnL >= 0 {
		t.Fatalf("daily P/L: %.2f", at.gridState.DailyPnL)
	}
}

func lossLimitTrader(t *testing.T) (*AutoTrader, *Stores) {
	t.Helper()
	at, st, _ := haltTrader(t)
	at.config.Grid = nil
	at.config.Agent = kind.DefaultAgent()
	at.config.Agent.RiskControl.MaxDailyLossPct = 5
	at.config.Agent.RiskControl.MaxDrawdownPct = 15
	return at, st
}

func snapshotAt(t *testing.T, st *Stores, at *AutoTrader, equity float64, when time.Time) {
	t.Helper()
	if err := st.Equity.Save(&EquitySnapshot{TraderID: at.id, TotalEquity: equity, Timestamp: when}); err != nil {
		t.Fatal(err)
	}
}

func TestAnAgentTraderHaltsAtItsLossLimits(t *testing.T) {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	t.Run("daily loss ends on the next UTC day", func(t *testing.T) {
		at, st := lossLimitTrader(t)
		at.config.Agent.RiskControl.MaxDrawdownPct = 0
		snapshotAt(t, st, at, 2000, today.Add(-time.Hour)) // yesterday's equity does not count
		snapshotAt(t, st, at, 1000, today.Add(time.Second))
		at.checkLossLimits(960)
		if at.ActiveHalt() != nil {
			t.Fatalf("4%% lost today: %+v", at.halt)
		}
		at.checkLossLimits(940)
		halt := at.ActiveHalt()
		if halt == nil || halt.Until == nil || !halt.Until.Equal(today.Add(24*time.Hour)) {
			t.Fatalf("halt: %+v", halt)
		}
	})

	t.Run("drawdown holds until resumed, and resuming starts it over", func(t *testing.T) {
		at, st := lossLimitTrader(t)
		at.config.Agent.RiskControl.MaxDailyLossPct = 0
		snapshotAt(t, st, at, 1000, time.Now().Add(-time.Hour))
		at.checkLossLimits(860)
		if at.ActiveHalt() != nil {
			t.Fatal("14% below peak is within 15%")
		}
		at.checkLossLimits(840)
		halt := at.ActiveHalt()
		if halt == nil || halt.Until != nil {
			t.Fatalf("halt: %+v", halt)
		}
		if err := at.Resume(); err != nil {
			t.Fatal(err)
		}
		at.checkLossLimits(840)
		if at.ActiveHalt() != nil {
			t.Fatal("a resumed trader tripped on the old peak")
		}
	})

	t.Run("no limit, no halt", func(t *testing.T) {
		at, st := lossLimitTrader(t)
		at.config.Agent.RiskControl.MaxDailyLossPct, at.config.Agent.RiskControl.MaxDrawdownPct = 0, 0
		snapshotAt(t, st, at, 1000, time.Now().Add(-time.Hour))
		at.checkLossLimits(100)
		if at.ActiveHalt() != nil {
			t.Fatal("limits that are off tripped")
		}
	})
}

func TestAPeakOutlivesARestartAndGoesWithItsPosition(t *testing.T) {
	at, st, _ := haltTrader(t)
	at.peakPnLCache, at.peakPnLSaved = map[string]float64{}, map[string]float64{}

	at.UpdatePeakPnL("BTCUSDT", "long", 10)
	at.UpdatePeakPnL("BTCUSDT", "long", 10.5) // Within a step: not written
	at.UpdatePeakPnL("ETHUSDT", "long", -3)   // Nothing to give back: not written

	restarted := &AutoTrader{id: at.id, stores: st, peakPnLCache: map[string]float64{}, peakPnLSaved: map[string]float64{}}
	restarted.loadStoredPeaks()
	if got := restarted.peakPnLCache["BTCUSDT_long"]; got != 10 {
		t.Fatalf("stored peak: %v", got)
	}
	if _, stored := restarted.peakPnLCache["ETHUSDT_long"]; stored {
		t.Fatal("a position with no profit was stored")
	}

	_, _, peak, drawdown := restarted.trackDrawdown(heldPosition{symbol: "BTCUSDT", side: "long", entryPrice: 100, leverage: 1}, 105)
	if peak != 10 || drawdown != 50 {
		t.Fatalf("after a restart: peak %v, drawdown %v", peak, drawdown)
	}

	restarted.forgetPeaksNotHeld(map[string]bool{"ETHUSDT_long": true})
	if _, known := restarted.peakPnLCache["BTCUSDT_long"]; known {
		t.Fatal("the peak of a position that is gone stayed")
	}
	stored, err := st.Peaks.Load(at.id)
	if err != nil || len(stored) != 0 {
		t.Fatalf("stored after it went: %+v, %v", stored, err)
	}
}

func TestStoppingAGridCancelsItsOrders(t *testing.T) {
	at, _, exchange := haltTrader(t)
	at.stopMonitorCh = make(chan struct{})
	at.gridState = NewGridState(at.config.Grid)
	at.Stop()
	if exchange.cancels != 1 || at.StopError() != nil {
		t.Fatalf("cancels %d, error %v", exchange.cancels, at.StopError())
	}

	failing, _, broken := haltTrader(t)
	failing.stopMonitorCh = make(chan struct{})
	failing.gridState = NewGridState(failing.config.Grid)
	broken.cancelErr = errors.New("exchange down")
	failing.Stop()
	if failing.StopError() == nil {
		t.Fatal("orders left behind went unreported")
	}

	// A grid that never started has no orders of its own to take with it.
	idle, _, untouched := haltTrader(t)
	idle.stopMonitorCh = make(chan struct{})
	idle.Stop()
	if untouched.cancels != 0 {
		t.Fatal("cancelled orders for a grid that never ran")
	}
}
