package futures

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOKXDepthConvertsEveryContractLevel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v5/public/instruments" {
			fmt.Fprint(w, `{"code":"0","data":[{"instId":"BTC-USDT-SWAP","state":"live","instType":"SWAP","ctType":"linear","ctVal":"0.01","ctValCcy":"BTC","ctMult":"1","settleCcy":"USDT","tickSz":"0.1","lotSz":"0.01","minSz":"0.01"}]}`)
			return
		}
		if r.URL.Query().Get("sz") != "100" {
			t.Fatal("insufficient depth")
		}
		fmt.Fprintf(w, `{"code":"0","data":[{"bids":[["80000","100","0","1"],["79999","200","0","1"]],"asks":[["80000.1","100","0","1"],["80001","200","0","1"]],"ts":"%d"}]}`, time.Now().UnixMilli())
	}))
	defer server.Close()
	c := NewMarket()
	c.Public.OKX.Base = server.URL
	s, e := c.Snapshot(context.Background(), "okx", "BTC-USDT-SWAP")
	if e != nil || len(s.Bids) != 2 || s.Bids[1].Quantity != "2.000000000000" {
		t.Fatalf("wrong base book %+v %v", s, e)
	}
}
