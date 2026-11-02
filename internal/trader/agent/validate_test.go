package agent

import (
	"testing"
)

// TestLeverageFallback tests automatic correction when leverage exceeds limit
func TestLeverageFallback(t *testing.T) {
	tests := []struct {
		name            string
		decision        Decision
		accountEquity   float64
		btcEthLeverage  int
		altcoinLeverage int
		wantLeverage    int // Expected leverage after correction
		wantError       bool
	}{
		{
			name: "Altcoin leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        20, // Exceeds limit
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5, // Limit 5x
			wantLeverage:    5, // Should be corrected to 5
			wantError:       false,
		},
		{
			name: "BTC leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        20, // Exceeds limit
				PositionSizeUSD: 1000,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   100,
			btcEthLeverage:  10, // Limit 10x
			altcoinLeverage: 5,
			wantLeverage:    10, // Should be corrected to 10
			wantError:       false,
		},
		{
			name: "Leverage within limit - no correction",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "open_short",
				Leverage:        5, // Not exceeded
				PositionSizeUSD: 500,
				StopLoss:        4000,
				TakeProfit:      3000,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantLeverage:    5, // Stays unchanged
			wantError:       false,
		},
		{
			name: "Leverage is 0 - should error",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        0, // Invalid
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantLeverage:    0,
			wantError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use default position value ratios for testing (10x for BTC/ETH, 1.5x for altcoins)
			err := validateDecision(&tt.decision, tt.accountEquity, tt.btcEthLeverage, tt.altcoinLeverage, 10.0, 1.5)

			// Check error status
			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// If shouldn't error, check if leverage was correctly corrected
			if !tt.wantError && tt.decision.Leverage != tt.wantLeverage {
				t.Errorf("Leverage not corrected: got %d, want %d", tt.decision.Leverage, tt.wantLeverage)
			}
		})
	}
}

func TestFixedExitDecisionStillRequiresTakeProfit(t *testing.T) {
	decision := Decision{
		Symbol:          "BTCUSDT",
		Action:          "open_long",
		Leverage:        10,
		PositionSizeUSD: 300,
		StopLoss:        95000,
		TakeProfit:      0,
	}

	if err := validateDecision(&decision, 30, 10, 10, 10.0, 10.0); err == nil {
		t.Fatal("ordinary fixed-exit strategy should still reject zero take profit")
	}
}

func TestABadDecisionTakesOnlyItselfOut(t *testing.T) {
	reply := `<decision>[
		{"symbol":"BTCUSDT","action":"close_long","reasoning":"take the gain"},
		{"symbol":"SOLUSDT","action":"open_long","leverage":0,"position_size_usd":100,"stop_loss":50,"take_profit":200,"reasoning":"no leverage"},
		{"symbol":"ETHUSDT","action":"dance","reasoning":"no such thing"}
	]</decision>`
	full, err := parseFullDecisionResponse(reply, 1000, 5, 5, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Decisions) != 3 {
		t.Fatalf("decisions: %+v", full.Decisions)
	}
	if full.Decisions[0].Rejected != "" {
		t.Fatalf("the close was refused: %s", full.Decisions[0].Rejected)
	}
	if full.Decisions[1].Rejected == "" || full.Decisions[2].Rejected == "" {
		t.Fatalf("bad decisions passed: %+v", full.Decisions)
	}
}
