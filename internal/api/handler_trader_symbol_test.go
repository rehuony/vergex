package api

import "testing"

func TestIsSupportedTraderSymbol(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   bool
	}{
		{name: "legacy USDT perp", symbol: "BTCUSDT", want: true},
		{name: "legacy USDT perp lowercase", symbol: "ethusdt", want: true},
		{name: "removed Hyperliquid USDC pair rejected", symbol: "GOLD-USDC", want: false},
		{name: "removed xyz prefix rejected", symbol: "xyz:SMSN", want: false},
		{name: "empty slot ignored", symbol: "  ", want: true},
		{name: "bare ticker rejected", symbol: "SMSN", want: false},
		{name: "unknown non-USDT pair rejected", symbol: "BTCUSD", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedTraderSymbol(tt.symbol); got != tt.want {
				t.Fatalf("isSupportedTraderSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
		})
	}
}
