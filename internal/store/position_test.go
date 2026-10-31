package store

import (
	"testing"
	"time"

	"vergex/internal/testdb"
)

func TestGetOpenPositionBySymbolMatchesSideCaseInsensitively(t *testing.T) {
	db := testdb.Open(t)

	positions := NewPositionStore(db)
	if err := positions.InitTables(); err != nil {
		t.Fatalf("init position table: %v", err)
	}

	entryTime := time.Now().Add(-5 * time.Minute).UnixMilli()
	if err := positions.Create(&TraderPosition{
		TraderID:   "trader-1",
		Symbol:     "AAVEUSDT",
		Side:       "LONG",
		Quantity:   0.27,
		EntryPrice: 88.519,
		EntryTime:  entryTime,
	}); err != nil {
		t.Fatalf("create position: %v", err)
	}

	got, err := positions.GetOpenPositionBySymbol("trader-1", "AAVEUSDT", "long")
	if err != nil {
		t.Fatalf("get open position: %v", err)
	}
	if got == nil {
		t.Fatal("expected open position")
	}
	if got.EntryTime != entryTime {
		t.Fatalf("entry time mismatch: got %d want %d", got.EntryTime, entryTime)
	}
}

func TestClosedPositionQueriesScopeToOneTrader(t *testing.T) {
	db := testdb.Open(t)

	positions := NewPositionStore(db)
	if err := positions.InitTables(); err != nil {
		t.Fatalf("init position table: %v", err)
	}

	now := time.Now().UnixMilli()
	rows := []*TraderPosition{
		{
			TraderID:     "current-trader",
			Symbol:       "SOLUSDT",
			Side:         "LONG",
			Quantity:     1,
			EntryPrice:   100,
			EntryTime:    now - 3000,
			ExitPrice:    101,
			ExitTime:     now - 2000,
			RealizedPnL:  1,
			Status:       "CLOSED",
			CreatedAt:    now - 3000,
			UpdatedAt:    now - 2000,
			CloseReason:  "sync",
			ExchangeType: "binance",
		},
		{
			TraderID:     "other-trader",
			Symbol:       "AAVEUSDT",
			Side:         "LONG",
			Quantity:     2,
			EntryPrice:   50,
			EntryTime:    now - 5000,
			ExitPrice:    49,
			ExitTime:     now - 4000,
			RealizedPnL:  -2,
			Status:       "CLOSED",
			CreatedAt:    now - 5000,
			UpdatedAt:    now - 4000,
			CloseReason:  "sync",
			ExchangeType: "binance",
		},
		{
			TraderID:     "third-trader",
			Symbol:       "LITUSDT",
			Side:         "LONG",
			Quantity:     3,
			EntryPrice:   10,
			EntryTime:    now - 7000,
			ExitPrice:    12,
			ExitTime:     now - 6000,
			RealizedPnL:  6,
			Status:       "CLOSED",
			CreatedAt:    now - 7000,
			UpdatedAt:    now - 6000,
			CloseReason:  "sync",
			ExchangeType: "binance",
		},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create position: %v", err)
		}
	}

	got, err := positions.GetClosedPositions("current-trader", 100)
	if err != nil {
		t.Fatalf("get closed positions: %v", err)
	}
	if len(got) != 1 || got[0].TraderID != "current-trader" {
		t.Fatalf("expected only the current trader's position, got %d", len(got))
	}

	stats, err := positions.GetFullStats("current-trader", 0)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalTrades != 1 || stats.TotalPnL != 1 {
		t.Fatalf("unexpected stats: trades=%d pnl=%.2f", stats.TotalTrades, stats.TotalPnL)
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
	db := testdb.Open(t)
	positions := NewPositionStore(db)
	if err := positions.InitTables(); err != nil {
		t.Fatalf("init position table: %v", err)
	}
	const closedAt = int64(1_700_000_000_000)
	for i, exit := range []int64{closedAt, closedAt, closedAt, closedAt + 1, closedAt - 1} {
		position := &TraderPosition{TraderID: "t1", Symbol: "BTCUSDT", Side: "LONG", Status: "CLOSED", Quantity: 1, EntryPrice: 1, EntryTime: closedAt - 1000, ExitTime: exit, CreatedAt: int64(i)}
		if err := db.Create(position).Error; err != nil {
			t.Fatal(err)
		}
	}
	other := &TraderPosition{TraderID: "t2", Symbol: "BTCUSDT", Side: "LONG", Status: "CLOSED", Quantity: 1, EntryPrice: 1, EntryTime: closedAt - 1000, ExitTime: closedAt}
	if err := db.Create(other).Error; err != nil {
		t.Fatal(err)
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

	if _, err := positions.GetClosedPositionsBefore("t1", other.ID, 2); err == nil {
		t.Fatal("another trader's position was accepted as a place to continue from")
	}
}
