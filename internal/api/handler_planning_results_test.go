package api

import (
	"testing"
	"time"

	"vergex/internal/planning/core"
	"vergex/internal/planning/live"
	"vergex/internal/trader/types"
)

func TestClosedPnLIsCreditedToThePlanThatContainsIt(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	run := func(id, symbol string, from, to time.Duration) live.View {
		start := base.Add(from)
		view := live.View{Run: live.Run{ID: id, StartedAt: &start, UpdatedAt: base.Add(to)}}
		view.Plan = core.Plan{Snapshot: core.Snapshot{Symbol: symbol}}
		return view
	}
	runs := []live.View{
		run("btc", "BTCUSDT", 0, time.Hour),
		run("eth", "ETH-USDT-SWAP", 0, time.Hour),
		run("btc-later", "BTCUSDT", 3*time.Hour, 4*time.Hour),
	}
	closes := []types.ClosedPnLRecord{
		{Symbol: "BTCUSDT", RealizedPnL: 10, ExitTime: base.Add(30 * time.Minute)},
		{Symbol: "BTCUSDT", RealizedPnL: -2, ExitTime: base.Add(61 * time.Minute)}, // Within the grace period.
		{Symbol: "ETHUSDT", RealizedPnL: 5, ExitTime: base.Add(30 * time.Minute)},  // Normalized to the OKX symbol.
		{Symbol: "BTCUSDT", RealizedPnL: 7, ExitTime: base.Add(3*time.Hour + time.Minute)},
		{Symbol: "BTCUSDT", RealizedPnL: 99, ExitTime: base.Add(2 * time.Hour)}, // Between plans: a manual close.
		{Symbol: "SOLUSDT", RealizedPnL: 99, ExitTime: base.Add(30 * time.Minute)},
	}

	results := attributeClosedPnL(runs, closes)

	if got := results["btc"]; got.RealizedPnL != 8 || got.Closes != 2 {
		t.Errorf("btc: %+v", got)
	}
	if got := results["eth"]; got.RealizedPnL != 5 {
		t.Errorf("eth: %+v", got)
	}
	if got := results["btc-later"]; got.RealizedPnL != 7 {
		t.Errorf("btc-later: %+v", got)
	}
	if len(results) != 3 {
		t.Errorf("a close outside every plan was credited: %+v", results)
	}
}
