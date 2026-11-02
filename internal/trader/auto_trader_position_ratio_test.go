package trader

import (
	"testing"
	"vergex/internal/trader/kind"
)

func TestEnforcePositionValueRatioUsesConfigCaps(t *testing.T) {
	cfg := kind.DefaultAgent()
	cfg.RiskControl.BTCETHMaxPositionValueRatio = 10
	cfg.RiskControl.AltcoinMaxPositionValueRatio = 2
	at := &AutoTrader{config: AutoTraderConfig{Agent: cfg}}

	if adjusted, capped := at.enforcePositionValueRatio(900, 100, "BTCUSDT"); capped || adjusted != 900 {
		t.Fatalf("BTC within the 10x cap should pass, got capped=%v adjusted=%.2f", capped, adjusted)
	}
	if adjusted, capped := at.enforcePositionValueRatio(900, 100, "DOGEUSDT"); !capped || adjusted != 200 {
		t.Fatalf("altcoin should be capped at 2x equity, got capped=%v adjusted=%.2f", capped, adjusted)
	}
}
