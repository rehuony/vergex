package trader

import (
	"testing"

	"vergex/internal/account"
	"vergex/internal/trader/kind"
)

func TestGridRiskHasNoReadingUnlessTheGridIsRunning(t *testing.T) {
	config := AutoTraderConfig{Grid: &kind.GridConfig{Symbol: "BTCUSDT", TotalInvestment: 1000, Leverage: 5}}
	ran := NewGridState(config.Grid)
	ran.IsInitialized = true

	// None of them has an exchange: a read would panic.
	cases := map[string]*AutoTrader{
		"stopped, never run":            {config: config},
		"stopped after a run":           {config: config, gridState: ran},
		"started, grid not created yet": {config: config, isRunning: true},
		"started, grid not initialized": {config: config, isRunning: true, gridState: NewGridState(config.Grid)},
		"not a grid trader":             {isRunning: true, gridState: ran},
	}
	for name, at := range cases {
		t.Run(name, func(t *testing.T) {
			if risk := at.GetGridRiskInfo(); risk != nil {
				t.Fatalf("risk = %+v, want no reading", risk)
			}
		})
	}
}

type gridRiskExchange struct{ account.Trading }

func (gridRiskExchange) GetMarketPrice(string) (float64, error) { return 80000, nil }

func (gridRiskExchange) GetPositions() ([]map[string]any, error) {
	return []map[string]any{{"symbol": "BTCUSDT", "positionAmt": 0.01, "entryPrice": 80000.0}}, nil
}

func TestGridRiskReadsARunningGrid(t *testing.T) {
	config := AutoTraderConfig{Grid: &kind.GridConfig{Symbol: "BTCUSDT", TotalInvestment: 1000, Leverage: 5}}
	state := NewGridState(config.Grid)
	state.IsInitialized = true
	state.ShortBoxUpper = 81000
	at := &AutoTrader{config: config, isRunning: true, gridState: state, trader: gridRiskExchange{}}

	risk := at.GetGridRiskInfo()
	if risk == nil {
		t.Fatal("a running grid has a reading")
	}
	if risk.CurrentLeverage != 5 || risk.CurrentPrice != 80000 || risk.CurrentPosition != 800 || risk.ShortBoxUpper != 81000 {
		t.Fatalf("risk = %+v", risk)
	}
}
