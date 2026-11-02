package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	spot "github.com/adshao/go-binance/v2"
)

func TestKeyPermissionsReadWhatBinanceReports(t *testing.T) {
	answer := `{"ipRestrict":true,"enableReading":true,"enableFutures":false,"enableWithdrawals":true,"enableSpotAndMarginTrading":true}`
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sapi/v1/account/apiRestrictions" || r.Method != http.MethodGet {
			t.Errorf("asked %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer server.Close()
	client := spot.NewClient("key", "secret")
	client.BaseURL = server.URL

	got, err := keyPermissions(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	// Spot trading is not what trading means here: only futures counts.
	if !got.Read || got.Trade || !got.Withdraw || !got.IPRestricted {
		t.Errorf("permissions = %+v", got)
	}

	answer, status = `{"code":-2015,"msg":"Invalid API-key, IP, or permissions for action."}`, http.StatusUnauthorized
	_, err = keyPermissions(context.Background(), client)
	if err == nil || strings.Contains(err.Error(), server.URL) {
		t.Errorf("error = %v", err)
	}
}

func TestBinanceTestEnvironmentDoesNotSayWhatAKeyMayDo(t *testing.T) {
	got, err := NewExecutionTrader("key", "secret").onEnvironment(true).Permissions(context.Background())
	if got != nil || err != nil {
		t.Errorf("permissions = %+v, err %v", got, err)
	}
}
