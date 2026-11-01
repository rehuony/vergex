package store

import (
	"testing"
	"time"
)

// position books one through the ledger, closed unless closedAt is zero.
func (f *ledgerFixture) position(traderID string, instrumentID int64, openedAt, closedAt time.Time) {
	f.t.Helper()
	owner := Owner{Source: SourceTrader, SourceID: traderID}
	open := f.trade(SideBuy, SideLong, "1", "100")
	open.InstrumentID, open.ExecutedAt = instrumentID, openedAt
	f.record(open, owner)
	if closedAt.IsZero() {
		return
	}
	closing := f.trade(SideSell, SideLong, "1", "101")
	closing.InstrumentID, closing.ExecutedAt = instrumentID, closedAt
	f.record(closing, owner)
}

func TestGetOpenPositionBySymbolMatchesSideCaseInsensitively(t *testing.T) {
	f := newLedgerFixture(t)
	openedAt := time.Now().Add(-5 * time.Minute).Truncate(time.Millisecond)
	f.position("trader-1", f.btc, openedAt, time.Time{})

	got, err := NewPositionStore(f.db).GetOpenPositionBySymbol("trader-1", "BTCUSDT", "LONG")
	if err != nil {
		t.Fatalf("get open position: %v", err)
	}
	if got == nil {
		t.Fatal("expected open position")
	}
	if got.EntryTime != openedAt.UnixMilli() || got.Status != "OPEN" || got.Side != "LONG" {
		t.Fatalf("position = %+v", got)
	}
	if none, err := NewPositionStore(f.db).GetOpenPositionBySymbol("trader-1", "BTCUSDT", "short"); err != nil || none != nil {
		t.Fatalf("short = %+v, err %v", none, err)
	}
}

func TestClosedPositionQueriesScopeToOneTrader(t *testing.T) {
	f := newLedgerFixture(t)
	now := time.Now()
	f.position("current-trader", f.btc, now.Add(-3*time.Minute), now.Add(-2*time.Minute))
	f.position("other-trader", f.eth, now.Add(-5*time.Minute), now.Add(-4*time.Minute))

	positions := NewPositionStore(f.db)
	got, err := positions.GetClosedPositions("current-trader", 100)
	if err != nil {
		t.Fatalf("get closed positions: %v", err)
	}
	if len(got) != 1 || got[0].TraderID != "current-trader" {
		t.Fatalf("expected only the current trader's position, got %d", len(got))
	}
	// A closed position still says how much it held.
	if got[0].Quantity != 1 || got[0].EntryQuantity != 1 || got[0].Status != "CLOSED" || got[0].ExitPrice != 101 {
		t.Fatalf("position = %+v", got[0])
	}

	stats, err := positions.GetFullStats("current-trader", 0)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalTrades != 1 || stats.TotalPnL != 1 {
		t.Fatalf("unexpected stats: trades=%d pnl=%.2f", stats.TotalTrades, stats.TotalPnL)
	}
	recent, err := positions.GetRecentTrades("current-trader", 10)
	if err != nil || len(recent) != 1 || recent[0].Side != "long" || recent[0].PnLPct != 1 {
		t.Fatalf("recent = %+v, err %v", recent, err)
	}
}

func TestAnOrderReadsAsItWasPlaced(t *testing.T) {
	f := newLedgerFixture(t)
	f.record(f.trade(SideBuy, SideLong, "1", "100"), traderOwner)
	f.record(f.trade(SideSell, "", "1", "110"), traderOwner)

	orders, err := NewOrderStore(f.db).GetTraderOrders("t1", 10)
	if err != nil || len(orders) != 2 {
		t.Fatalf("orders = %d, err %v", len(orders), err)
	}
	closing, opening := orders[0], orders[1]
	if opening.OrderAction != "open_long" || opening.Side != "BUY" || opening.PositionSide != "LONG" || opening.Status != OrderFilled {
		t.Errorf("opening = %+v", opening)
	}
	// One-way mode has no position side to go by.
	if closing.OrderAction != "close_long" || closing.PositionSide != "" {
		t.Errorf("closing = %+v", closing)
	}
	if opening.Commission != 0.5 || opening.CommissionAsset != "USDT" || opening.AvgFillPrice != 100 || opening.Symbol != "BTCUSDT" {
		t.Errorf("opening = %+v", opening)
	}

	fills, err := NewOrderStore(f.db).GetOrderFills("t1", opening.ID)
	if err != nil || len(fills) != 1 || fills[0].Price != 100 || fills[0].Side != "BUY" {
		t.Fatalf("fills = %+v, err %v", fills, err)
	}
	if fills, err := NewOrderStore(f.db).GetOrderFills("t2", opening.ID); err != nil || len(fills) != 0 {
		t.Fatalf("another trader read fills: %+v, err %v", fills, err)
	}
}

func TestCalculateMaxDrawdownUsesRealBaseline(t *testing.T) {
	// +50 then -100: peak 550, trough 450 on a 500 account → 100/550 ≈ 18.18%.
	pnls := []float64{50, -100}

	got := calculateMaxDrawdownFromPnls(pnls, 500)
	if got < 18.1 || got > 18.3 {
		t.Fatalf("expected ~18.18%% drawdown on a 500 baseline, got %.2f", got)
	}

	// Unknown baseline falls back to the neutral 10k curve: 100/10050 ≈ 1%.
	fallback := calculateMaxDrawdownFromPnls(pnls, 0)
	if fallback < 0.9 || fallback > 1.1 {
		t.Fatalf("expected ~1%% drawdown on the 10k fallback, got %.2f", fallback)
	}

	if calculateMaxDrawdownFromPnls(nil, 500) != 0 {
		t.Fatalf("no trades must mean zero drawdown")
	}
}

func TestClosedPositionPagesCoverEverythingOnceEvenWithEqualTimes(t *testing.T) {
	f := newLedgerFixture(t)
	closedAt := time.UnixMilli(1_700_000_000_000).UTC()
	for _, exit := range []time.Time{closedAt, closedAt, closedAt, closedAt.Add(time.Millisecond), closedAt.Add(-time.Millisecond)} {
		f.position("t1", f.btc, closedAt.Add(-time.Hour), exit)
	}
	f.position("t2", f.btc, closedAt.Add(-time.Hour), closedAt)
	positions := NewPositionStore(f.db)
	others, err := positions.GetClosedPositions("t2", 1)
	if err != nil || len(others) != 1 {
		t.Fatalf("other trader = %+v, err %v", others, err)
	}

	page, err := positions.GetClosedPositions("t1", 2)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	var lastExit int64
	for len(page) > 0 {
		for _, position := range page {
			if seen[position.ID] {
				t.Fatalf("position %d came twice", position.ID)
			}
			if lastExit != 0 && position.ExitTime > lastExit {
				t.Fatalf("position %d is out of order", position.ID)
			}
			seen[position.ID] = true
			lastExit = position.ExitTime
		}
		page, err = positions.GetClosedPositionsBefore("t1", page[len(page)-1].ID, 2)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("pages covered %d positions, want 5", len(seen))
	}

	if _, err := positions.GetClosedPositionsBefore("t1", others[0].ID, 2); err == nil {
		t.Fatal("another trader's position was accepted as a place to continue from")
	}
}
