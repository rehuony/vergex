package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// bookServer answers the public requests a book makes, with fixed bodies and the given
// timestamp in milliseconds.
func bookServer(t *testing.T, ms int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/fapi/v1/exchangeInfo"):
			fmt.Fprint(w, `{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","contractType":"PERPETUAL","marginAsset":"USDT","quoteAsset":"USDT","baseAsset":"BTC","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"},{"filterType":"LOT_SIZE","stepSize":"0.001","minQty":"0.002"},{"filterType":"MIN_NOTIONAL","notional":"100"}]}]}`)
		case strings.Contains(r.URL.Path, "/fapi/v1/depth"):
			fmt.Fprintf(w, `{"T":%d,"E":%d,"bids":[["99999.9","2.5"],["99999.8","4"],["99990","7.125"]],"asks":[["100000.1","3"],["100000.2","5.5"]]}`, ms, ms)
		case strings.Contains(r.URL.Path, "/api/v5/public/instruments"):
			fmt.Fprint(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","state":"live","instType":"SWAP","ctType":"linear","ctVal":"0.01","ctValCcy":"BTC","settleCcy":"USDT","ctMult":"1","tickSz":"0.1","lotSz":"0.1","minSz":"0.1"}]}`)
		case strings.Contains(r.URL.Path, "/api/v5/market/books"):
			fmt.Fprintf(w, `{"code":"0","data":[{"bids":[["99999.9","250","0","1"],["99999.8","400","0","2"]],"asks":[["100000.1","300","0","1"],["100000.2","550","0","3"]],"ts":"%d"}]}`, ms)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestABookReadsTheSameAsBeforeItMoved holds what the Futures liquidity check reads to the
// values recorded from the code before the HTTP calls moved into venue/.
func TestABookReadsTheSameAsBeforeItMoved(t *testing.T) {
	ms := time.Now().UnixMilli()
	server := bookServer(t, ms)
	c := NewMarket()
	c.Public.Binance.Futures, c.Public.OKX.Base = server.URL, server.URL

	for _, tt := range []struct{ venue, symbol, want string }{
		{"binance", "BTCUSDT", goldenBinanceBook},
		{"okx", "BTC-USDT-SWAP", goldenOKXBook},
	} {
		snapshot, err := c.Snapshot(context.Background(), tt.venue, tt.symbol)
		if err != nil {
			t.Fatalf("%s: %v", tt.venue, err)
		}
		if !snapshot.SourceTime.Equal(time.UnixMilli(ms)) || snapshot.SourceTime.Location() != time.UTC {
			t.Errorf("%s source time = %v", tt.venue, snapshot.SourceTime)
		}
		if snapshot.Observed.IsZero() {
			t.Errorf("%s has no observed time", tt.venue)
		}
		snapshot.Observed, snapshot.SourceTime = time.Time{}, time.Time{}
		got, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tt.want {
			t.Errorf("%s book changed:\n got %s\nwant %s", tt.venue, got, tt.want)
		}
	}
}

const (
	goldenBinanceBook = `{"venue":"binance","symbol":"BTCUSDT","kind":"perpetual","base_asset":"BTC","quote_asset":"USDT","bid":"99999.9","ask":"100000.1","bid_size":"2.5","ask_size":"3","bids":[{"price":"99999.9","quantity":"2.5"},{"price":"99999.8","quantity":"4"},{"price":"99990","quantity":"7.125"}],"asks":[{"price":"100000.1","quantity":"3"},{"price":"100000.2","quantity":"5.5"}],"tick":"0.10","step":"0.001","min_qty":"0.002","min_notional":"100","observed":"0001-01-01T00:00:00Z","source_time":"0001-01-01T00:00:00Z"}`
	goldenOKXBook     = `{"venue":"okx","symbol":"BTC-USDT-SWAP","kind":"perpetual","base_asset":"BTC","quote_asset":"USDT","contract_value":"0.01","bid":"99999.9","ask":"100000.1","bid_size":"2.500000000000","ask_size":"3.000000000000","bids":[{"price":"99999.9","quantity":"2.500000000000"},{"price":"99999.8","quantity":"4.000000000000"}],"asks":[{"price":"100000.1","quantity":"3.000000000000"},{"price":"100000.2","quantity":"5.500000000000"}],"tick":"0.1","step":"0.001000000000","min_qty":"0.001000000000","min_notional":"0.000000000001","observed":"0001-01-01T00:00:00Z","source_time":"0001-01-01T00:00:00Z"}`
)
