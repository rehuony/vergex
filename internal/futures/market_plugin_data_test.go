package futures

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"vergex/internal/futures/plugins"
)

func TestPublicPluginNativeUnitsAndTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("private header on public data")
		}
		switch r.URL.Path {
		case "/fapi/v1/premiumIndex":
			fmt.Fprintf(w, `{"symbol":"BTCUSDT","markPrice":"83000","lastFundingRate":"-0.0001","time":%d}`, time.Now().UnixMilli())
		case "/fapi/v1/openInterest":
			fmt.Fprintf(w, `{"symbol":"BTCUSDT","openInterest":"12000","time":%d}`, time.Now().UnixMilli())
		case "/api/v5/public/funding-rate":
			fmt.Fprintf(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","fundingRate":"0.0002","ts":"%d"}]}`, time.Now().UnixMilli())
		case "/api/v5/public/open-interest":
			fmt.Fprintf(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","oi":"45000","ts":"%d"}]}`, time.Now().UnixMilli())
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			http.Error(w, "unexpected", 400)
		}
	}))
	defer server.Close()
	c := NewMarket()
	c.Public.Binance.Futures = server.URL
	c.Public.OKX.Base = server.URL
	registry, _ := plugins.Open(plugins.Builtins(c)...)
	for _, q := range []plugins.Request{{Venue: "binance", Symbol: "BTCUSDT"}, {Venue: "okx", Symbol: "BTC-USDT-SWAP"}} {
		for _, id := range []string{"funding", "open_interest"} {
			r, err := registry.Read(context.Background(), id, q)
			if err != nil {
				t.Fatal(q, id, err)
			}
			if q.Venue == "okx" && id == "open_interest" && r.Metrics[0].Unit != "contracts" {
				t.Fatal("contracts confused with base")
			}
			if r.SourceTime.IsZero() {
				t.Fatal("missing source timestamp")
			}
		}
	}
}
