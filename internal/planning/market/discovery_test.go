package market

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestBinanceDiscoveryRanksNotionalAndKeepsOnlyCryptoPerpetuals(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Path == "/fapi/v1/exchangeInfo" {
			fmt.Fprint(w, `{"symbols":[{"symbol":"BTCUSDT","baseAsset":"BTC","quoteAsset":"USDT","marginAsset":"USDT","status":"TRADING","contractType":"PERPETUAL","underlyingType":"COIN"},{"symbol":"TSLAUSDT","baseAsset":"TSLA","quoteAsset":"USDT","marginAsset":"USDT","status":"TRADING","contractType":"TRADIFI_PERPETUAL","underlyingType":"EQUITY"},{"symbol":"OLDUSDT","baseAsset":"OLD","quoteAsset":"USDT","marginAsset":"USDT","status":"SETTLING","contractType":"PERPETUAL"},{"symbol":"GHOSTUSDT","baseAsset":"GHOST","quoteAsset":"USDT","marginAsset":"USDT","status":"TRADING","contractType":"PERPETUAL"}]}`)
			return
		}
		fmt.Fprint(w, `[{"symbol":"BTCUSDT","quoteVolume":"1000000000","lastPrice":"80000"},{"symbol":"TSLAUSDT","quoteVolume":"2000000000","lastPrice":"350"},{"symbol":"OLDUSDT","quoteVolume":"9999999999"},{"symbol":"GHOSTUSDT","quoteVolume":"NaN"}]`)
	}))
	defer server.Close()
	c := New()
	c.BinanceFutures = server.URL
	c.HTTP = server.Client()
	for i := 0; i < 2; i++ {
		list, err := c.Markets(context.Background(), "binance")
		if err != nil || len(list.Markets) != 1 || list.Markets[0].Symbol != "BTCUSDT" || list.Markets[0].Category != "crypto" || !list.Markets[0].ExecutionSupported {
			t.Fatalf("invalid market ranking: %+v %v", list, err)
		}
	}
	if reads.Load() != 2 {
		t.Fatal("cache did not prevent repeated exchange reads", reads.Load())
	}
}

func TestPublicDiscoveryLiveSmoke(t *testing.T) {
	if os.Getenv("VERGEX_PUBLIC_MARKET_SMOKE") != "1" {
		t.Skip("set VERGEX_PUBLIC_MARKET_SMOKE=1 for public venue reads")
	}
	c := New()
	for _, venue := range []string{"binance"} {
		list, err := c.Markets(context.Background(), venue)
		if err != nil {
			t.Fatal(venue, err)
		}
		if len(list.Markets) == 0 {
			t.Fatalf("missing market discovery for %s", venue)
		}
		for _, m := range list.Markets {
			if m.Category != "crypto" {
				t.Fatalf("non-crypto market %s (%s) on %s", m.Symbol, m.Category, venue)
			}
		}
		t.Logf("%s: %d markets, %d coverage warnings; top %s", venue, len(list.Markets), len(list.Warnings), list.Markets[0].Symbol)
		if len(list.Warnings) > 0 {
			t.Log(list.Warnings)
		}
	}
}
