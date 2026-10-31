package market

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVenueAdapters(t *testing.T) {
	for _, venue := range []string{"binance", "okx"} {
		t.Run(venue, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				switch venue {
				case "binance":
					if strings.Contains(r.URL.Path, "exchangeInfo") {
						fmt.Fprint(w, `{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","contractType":"PERPETUAL","marginAsset":"USDT","quoteAsset":"USDT","baseAsset":"BTC","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.01"},{"filterType":"LOT_SIZE","stepSize":"0.00001","minQty":"0.00001"},{"filterType":"NOTIONAL","minNotional":"10"}]}]}`)
					} else {
						if r.URL.Query().Get("limit") != "100" {
							t.Error("missing multi-level depth request")
						}
						fmt.Fprintf(w, `{"T":%d,"bids":[["99999","2"],["99998","4"]],"asks":[["100000","3"],["100001","5"]]}`, time.Now().UnixMilli())
					}
				case "okx":
					if strings.Contains(r.URL.Path, "instruments") {
						fmt.Fprint(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","state":"live","instType":"SWAP","ctType":"linear","ctVal":"1","ctValCcy":"BTC","settleCcy":"USDT","ctMult":"1","tickSz":"0.1","lotSz":"0.00001","minSz":"0.00001"}]}`)
					} else {
						fmt.Fprintf(w, `{"code":"0","data":[{"bids":[["99999","2","0","1"]],"asks":[["100000","3","0","1"]],"ts":"%d"}]}`, time.Now().UnixMilli())
					}
				}
			}))
			defer server.Close()
			c := New()
			c.Binance = server.URL
			c.BinanceFutures = server.URL
			c.OKX = server.URL
			symbol := map[string]string{"binance": "BTCUSDT", "okx": "BTC-USDT-SWAP"}[venue]
			s, e := c.Snapshot(context.Background(), venue, symbol)
			if e != nil {
				t.Fatal(e)
			}
			if s.Ask != "100000" || (s.BidSize != "2" && s.BidSize != "2.000000000000") || requests != 2 {
				t.Fatalf("bad snapshot %+v requests=%d", s, requests)
			}
			if venue != "okx" && (len(s.Bids) != 2 || len(s.Asks) != 2 || s.Bids[1].Quantity != "4" || s.Asks[1].Price != "100001") {
				t.Fatalf("discarded executable depth: %+v", s)
			}
		})
	}
}
func TestHanNativeSymbolMetadataAndEscapedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "exchangeInfo") {
			fmt.Fprint(w, `{"symbols":[{"symbol":"\u725b\u6765USDT","status":"TRADING","contractType":"PERPETUAL","marginAsset":"USDT","quoteAsset":"USDT","baseAsset":"\u725b\u6765","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.00001"},{"filterType":"LOT_SIZE","stepSize":"1","minQty":"1"},{"filterType":"MIN_NOTIONAL","notional":"5"}]}]}`)
			return
		}
		if r.URL.Query().Get("symbol") != "\u725b\u6765USDT" || !strings.Contains(r.RequestURI, "%E7%89%9B") {
			t.Error("native symbol was changed or not URL-encoded", r.RequestURI)
		}
		if strings.Contains(r.URL.Path, "klines") {
			fmt.Fprintf(w, `[[%d,"0.10","0.12","0.09","0.11","50"]]`, time.Now().Unix()/60*60*1000)
			return
		}
		fmt.Fprintf(w, `{"T":%d,"bids":[["0.10","500"]],"asks":[["0.11","500"]]}`, time.Now().UnixMilli())
	}))
	defer server.Close()
	c := New()
	c.BinanceFutures = server.URL
	s, err := c.Snapshot(context.Background(), "binance", "\u725b\u6765USDT")
	if err != nil || s.Symbol != "\u725b\u6765USDT" || s.BaseAsset != "\u725b\u6765" {
		t.Fatal(s, err)
	}
	if _, err = c.Candles(context.Background(), CandleQuery{Venue: "binance", Symbol: "\u725b\u6765USDT", Interval: "1m", Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Snapshot(context.Background(), "binance", "\u725b\u6765\u4e0d\u5b58\u5728USDT"); err == nil {
		t.Fatal("unlisted native symbol accepted")
	}
}
func TestRejectMarketResponses(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"http error", "{}", 429}, {"invalid json", "no", 200}, {"inactive", `{"symbols":[{"symbol":"BTCUSDT","status":"BREAK","isSpotTradingAllowed":true}]}`, 200}, {"missing rules", `{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","isSpotTradingAllowed":true}]}`, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer srv.Close()
			c := New()
			c.Binance = srv.URL
			c.BinanceFutures = srv.URL
			if _, e := c.Snapshot(context.Background(), "binance", "BTCUSDT"); e == nil {
				t.Fatal("accepted bad response")
			}
		})
	}
	c := New()
	if _, e := c.Snapshot(context.Background(), "binance", "BTCUSDT&symbol=ETHUSDT"); e == nil {
		t.Fatal("symbol injection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Snapshot(ctx, "okx", "BTC-USDT-SWAP"); e == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestStaleVenueTimestamp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "instruments") {
			fmt.Fprint(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","state":"live","instType":"SWAP","ctType":"linear","ctVal":"1","ctValCcy":"BTC","settleCcy":"USDT","ctMult":"1","tickSz":"0.1","lotSz":"0.00001","minSz":"0.00001"}]}`)
		} else {
			fmt.Fprint(w, `{"code":"0","data":[{"bids":[["99","2"]],"asks":[["100","3"]],"ts":"1"}]}`)
		}
	}))
	defer srv.Close()
	c := New()
	c.OKX = srv.URL
	if _, e := c.Snapshot(context.Background(), "okx", "BTC-USDT-SWAP"); e == nil {
		t.Fatal("stale venue data accepted")
	}
}

func TestOKXContractUnitsAreConvertedToBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "instruments") {
			if r.URL.Query().Get("instType") != "SWAP" {
				t.Error("not swap metadata")
			}
			fmt.Fprint(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","instType":"SWAP","state":"live","ctType":"linear","ctVal":"0.01","ctValCcy":"BTC","ctMult":"1","settleCcy":"USDT","tickSz":"0.1","lotSz":"0.01","minSz":"0.01"}]}`)
		} else {
			fmt.Fprintf(w, `{"code":"0","data":[{"bids":[["99999","200"]],"asks":[["100000","300"]],"ts":"%d"}]}`, time.Now().UnixMilli())
		}
	}))
	defer srv.Close()
	c := New()
	c.OKX = srv.URL
	b, err := c.Snapshot(context.Background(), "okx", "BTC-USDT-SWAP")
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind != "perpetual" || b.BidSize != "2.000000000000" || b.Step != "0.000100000000" || b.ContractValue != "0.01" {
		t.Fatalf("unit conversion wrong: %+v", b)
	}
}
func TestBinancePerpetualNeverFallsBackToSpot(t *testing.T) {
	spotCalls := 0
	spot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { spotCalls++; w.WriteHeader(500) }))
	defer spot.Close()
	perp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fapi/") {
			t.Error("wrong perpetual API")
		}
		fmt.Fprint(w, `{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","contractType":"CURRENT_QUARTER","marginAsset":"USDT","quoteAsset":"USDT"}]}`)
	}))
	defer perp.Close()
	c := New()
	c.Binance = spot.URL
	c.BinanceFutures = perp.URL
	if _, err := c.Snapshot(context.Background(), "binance", "BTCUSDT"); err == nil {
		t.Fatal("quarter accepted")
	}
	if spotCalls != 0 {
		t.Fatal("silent spot fallback")
	}
}

func TestBinanceTradFiPerpetualIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"symbols":[{"symbol":"SKHYUSDT","status":"TRADING","contractType":"TRADIFI_PERPETUAL","marginAsset":"USDT","quoteAsset":"USDT","baseAsset":"SKHY","filters":[{"filterType":"PRICE_FILTER","tickSize":"0.01000"}]}]}`)
	}))
	defer server.Close()
	c := New()
	c.BinanceFutures = server.URL
	if _, err := c.Snapshot(context.Background(), "binance", "SKHYUSDT"); err == nil {
		t.Fatal("TradFi perpetual must not be accepted as a crypto market")
	}
}
