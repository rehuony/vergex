package okx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKlinesReturnsOldestFirstWithBaseVolume(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
			["2000","77378.8","77378.9","77378.2","77378.2","216.89","2.1689","167826.85","0"],
			["1000","1","3","0.5","2","100","1","2","1"]]}`))
	}))
	defer server.Close()
	klineBaseURL = server.URL
	defer func() { klineBaseURL = defaultBaseURL }()

	candles, err := Klines(context.Background(), "BTCUSDT", "1d", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if query != "bar=1Dutc&instId=BTC-USDT-SWAP&limit=300" {
		t.Fatalf("query = %s", query)
	}
	if len(candles) != 2 || candles[0].OpenTime != 1000 || !candles[0].Closed || candles[1].Closed {
		t.Fatalf("candles = %+v", candles)
	}
	if last := candles[1]; last.Close != 77378.2 || last.Volume != 2.1689 || last.QuoteVolume != 167826.85 {
		t.Fatalf("last = %+v", last)
	}
}

func TestKlinesReportsWhatOKXRejects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"51001","msg":"Instrument ID does not exist","data":[]}`))
	}))
	defer server.Close()
	klineBaseURL = server.URL
	defer func() { klineBaseURL = defaultBaseURL }()

	if _, err := Klines(context.Background(), "NOPEUSDT", "1h", 10); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := Klines(context.Background(), "BTCUSDT", "1w", 10); err == nil {
		t.Fatal("an unknown interval should fail before any request")
	}
}

func TestInstrumentID(t *testing.T) {
	for input, want := range map[string]string{"BTCUSDT": "BTC-USDT-SWAP", "BTC-USDT-SWAP": "BTC-USDT-SWAP"} {
		if got := InstrumentID(input); got != want {
			t.Fatalf("%s -> %s, want %s", input, got, want)
		}
	}
}
