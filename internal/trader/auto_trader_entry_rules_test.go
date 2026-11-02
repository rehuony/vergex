package trader

import (
	"errors"
	"testing"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/kind"
)

func entryRulesTrader() *AutoTrader {
	cfg := kind.DefaultAgent()
	cfg.RiskControl.MinConfidence = 75
	cfg.RiskControl.MinRiskRewardRatio = 3
	cfg.RiskControl.MaxMarginUsage = 0.9
	return &AutoTrader{config: AutoTraderConfig{Agent: cfg}}
}

func TestEntryRulesJudgeAnOpenAtTheCurrentPrice(t *testing.T) {
	at := entryRulesTrader()
	for _, tt := range []struct {
		name       string
		action     string
		confidence int
		price      float64
		stop       float64
		target     float64
		allowed    bool
	}{
		{"long at the minimum", "open_long", 75, 100, 99, 103, true},
		{"long short of the minimum", "open_long", 75, 100, 99, 102, false},
		{"price ran towards the target", "open_long", 90, 101.5, 99, 103, false},
		{"short at the minimum", "open_short", 75, 100, 101, 97, true},
		{"short short of the minimum", "open_short", 75, 100, 101, 98, false},
		{"low confidence", "open_long", 74, 100, 99, 110, false},
		{"no confidence given", "open_long", 0, 100, 99, 110, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := at.enforceEntryRules(&traderagent.Decision{Action: tt.action, Confidence: tt.confidence, StopLoss: tt.stop, TakeProfit: tt.target}, tt.price)
			if tt.allowed && err != nil {
				t.Fatal(err)
			}
			var block riskBlock
			if !tt.allowed && !errors.As(err, &block) {
				t.Fatalf("want a risk block, got %v", err)
			}
		})
	}
}

func TestMarginUsageCapsAnOpenToWhatIsLeft(t *testing.T) {
	at := entryRulesTrader()
	// 1000 of value at 2x holds 500 of margin, leaving 400 under 90% of 1000.
	held := []map[string]interface{}{{"positionAmt": -1.0, "markPrice": 1000.0, "leverage": 2.0}}
	if size, capped := at.enforceMarginUsage(1500, 5, 1000, held); capped || size != 1500 {
		t.Fatalf("within the limit: %.2f, %v", size, capped)
	}
	if size, capped := at.enforceMarginUsage(2500, 5, 1000, held); !capped || size != 2000 {
		t.Fatalf("over the limit: %.2f, %v", size, capped)
	}
	full := []map[string]interface{}{{"positionAmt": 2.0, "markPrice": 1000.0, "leverage": 2.0}}
	if size, capped := at.enforceMarginUsage(100, 5, 1000, full); !capped || size != 0 {
		t.Fatalf("no margin left: %.2f, %v", size, capped)
	}
	if err := at.enforceMinPositionSize(0); !errors.As(err, new(riskBlock)) {
		t.Fatalf("a position cut to nothing should be blocked, got %v", err)
	}
}

func TestOnlyBTCAndETHAreMajors(t *testing.T) {
	for symbol, want := range map[string]bool{"BTCUSDT": true, "ETHUSDT": true, "ETHFIUSDT": false, "BTCDOMUSDT": false, "SOLUSDT": false} {
		if isMajorAsset(symbol) != want {
			t.Errorf("%s: want %v", symbol, want)
		}
	}
}
