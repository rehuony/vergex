package trader

import (
	"errors"
	"testing"

	tradertypes "vergex/internal/trader/types"
)

type emergencyCloseTestTrader struct {
	tradertypes.Trader
	closeCalls    int
	invalidations int
	cancelCalls   int
	positions     []map[string]interface{}
	openOrders    []tradertypes.OpenOrder
	cancelErr     error
}

func (t *emergencyCloseTestTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	t.closeCalls++
	return map[string]interface{}{"status": "submitted"}, nil
}

func (t *emergencyCloseTestTrader) CloseShort(string, float64) (map[string]interface{}, error) {
	t.closeCalls++
	return map[string]interface{}{"status": "submitted"}, nil
}

func (t *emergencyCloseTestTrader) InvalidatePositionCache() { t.invalidations++ }
func (t *emergencyCloseTestTrader) GetPositions() ([]map[string]interface{}, error) {
	return t.positions, nil
}
func (t *emergencyCloseTestTrader) CancelAllOrders(string) error {
	t.cancelCalls++
	return t.cancelErr
}
func (t *emergencyCloseTestTrader) GetOpenOrders(string) ([]tradertypes.OpenOrder, error) {
	return t.openOrders, nil
}

func TestValidateProtectionPrices(t *testing.T) {
	tests := []struct {
		name, action   string
		market, sl, tp float64
		wantErr        bool
	}{
		{"long valid", "open_long", 100, 95, 120, false},
		{"long missing target", "open_long", 100, 95, 0, true},
		{"long stop above market", "open_long", 100, 101, 120, true},
		{"long target below market", "open_long", 100, 95, 99, true},
		{"short valid", "open_short", 100, 105, 80, false},
		{"short missing target", "open_short", 100, 105, 0, true},
		{"short stop below market", "open_short", 100, 99, 80, true},
		{"short target above market", "open_short", 100, 105, 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProtectionPrices(tt.action, tt.market, tt.sl, tt.tp)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestEmergencyCloseUsesFreshPositionsAndVerifiesOrderCleanup(t *testing.T) {
	fake := &emergencyCloseTestTrader{}
	at := &AutoTrader{trader: fake}
	if err := at.emergencyClosePositionAndVerify("BTCUSDT", "long", 1); err != nil {
		t.Fatalf("emergency close failed: %v", err)
	}
	if fake.closeCalls != 1 || fake.invalidations != 1 || fake.cancelCalls != 1 {
		t.Fatalf("calls close=%d invalidate=%d cancel=%d, want 1 each", fake.closeCalls, fake.invalidations, fake.cancelCalls)
	}
}

func TestEmergencyCloseFailsIfProtectionOrdersRemain(t *testing.T) {
	fake := &emergencyCloseTestTrader{openOrders: []tradertypes.OpenOrder{{Symbol: "BTCUSDT"}}}
	at := &AutoTrader{trader: fake}
	if err := at.emergencyClosePositionAndVerify("BTCUSDT", "long", 1); err == nil {
		t.Fatal("expected remaining order to fail closed")
	}
}

func TestEmergencyCloseFailsIfOrderCleanupFails(t *testing.T) {
	fake := &emergencyCloseTestTrader{cancelErr: errors.New("cancel rejected")}
	at := &AutoTrader{trader: fake}
	if err := at.emergencyClosePositionAndVerify("BTCUSDT", "long", 1); err == nil {
		t.Fatal("expected cleanup error to fail closed")
	}
}
