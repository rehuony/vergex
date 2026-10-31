package okx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestUSDTSwapsMapsLiveLinearSwapsAndCaches(t *testing.T) {
	var calls atomic.Int32
	fail := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"code":"0","data":[
			{"instId":"BTC-USDT-SWAP","ctType":"linear","settleCcy":"USDT","state":"live"},
			{"instId":"ETH-USD-SWAP","ctType":"inverse","settleCcy":"ETH","state":"live"},
			{"instId":"OLD-USDT-SWAP","ctType":"linear","settleCcy":"USDT","state":"suspend"},
			{"instId":"PEPE-USDT-SWAP","ctType":"linear","settleCcy":"USDT","state":"live"}
		]}`)
	}))
	defer srv.Close()
	clock := time.Unix(0, 0)
	l := &Listing{baseURL: srv.URL, http: srv.Client(), now: func() time.Time { return clock }}

	got, err := l.USDTSwaps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["BTCUSDT"] || !got["PEPEUSDT"] {
		t.Fatalf("listing = %v", got)
	}
	if _, err := l.USDTSwaps(context.Background()); err != nil || calls.Load() != 1 {
		t.Fatalf("expected cached listing, calls=%d err=%v", calls.Load(), err)
	}

	clock = clock.Add(2 * time.Hour)
	fail.Store(true)
	stale, err := l.USDTSwaps(context.Background())
	if err != nil || !stale["BTCUSDT"] {
		t.Fatalf("expected previous listing on refresh failure, got %v %v", stale, err)
	}
}
