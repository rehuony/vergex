package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"vergex/internal/exchange/binance"
	"vergex/internal/store"
)

type fakeRanking struct {
	tickers []binance.Ticker
	oi      []binance.OIChange
	window  string
	pool    int
}

func (f *fakeRanking) TopVolume(context.Context) ([]binance.Ticker, error) { return f.tickers, nil }

func (f *fakeRanking) PriceMovers(_ context.Context, gainers bool, _ float64) ([]binance.Ticker, error) {
	out := append([]binance.Ticker(nil), f.tickers...)
	if !gainers {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}

func (f *fakeRanking) OIChanges(_ context.Context, window string, pool int) ([]binance.OIChange, error) {
	f.window, f.pool = window, pool
	return f.oi, nil
}

func oiChange(symbol string, pct, volume float64) binance.OIChange {
	return binance.OIChange{
		Ticker:        binance.Ticker{Symbol: symbol, QuoteVolume: volume, PriceChangePercent: 1},
		Window:        "4h",
		OIChangePct:   pct,
		OIChangeValue: pct * 1_000_000,
	}
}

func engineWith(source store.CoinSourceConfig, ranking RankingSource) *StrategyEngine {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource = source
	cfg.NormalizeProductSchema()
	e := NewStrategyEngine(&cfg)
	e.ranking = ranking
	return e
}

func symbols(coins []CandidateCoin) string {
	out := make([]string, 0, len(coins))
	for _, c := range coins {
		out = append(out, c.Symbol)
	}
	return strings.Join(out, ",")
}

func TestOITopAndLowUseDirectionAndExclusions(t *testing.T) {
	ranking := &fakeRanking{oi: []binance.OIChange{
		oiChange("AUSDT", 30, 9e8),
		oiChange("BUSDT", 20, 1e3), // below the volume floor
		oiChange("CUSDT", 10, 9e8),
		oiChange("DUSDT", 5, 9e8),
		oiChange("EUSDT", -8, 9e8),
		oiChange("FUSDT", -15, 9e8),
	}}

	top := engineWith(store.CoinSourceConfig{
		SourceType: "oi_top", OITopLimit: 2, ExcludedCoins: []string{"CUSDT"},
		MinQuoteVolume: 1e6, OIWindow: "24h", OIPoolSize: 30,
	}, ranking)
	coins, err := top.GetCandidateCoins()
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(coins); got != "AUSDT,DUSDT" {
		t.Fatalf("oi_top = %s, want AUSDT,DUSDT", got)
	}
	if ranking.window != "24h" || ranking.pool != 30 {
		t.Fatalf("window/pool not passed through: %s/%d", ranking.window, ranking.pool)
	}
	if data := top.RankingContext()["DUSDT"]; data == nil || data.Rank != 2 || data.OIDeltaPercent != 5 {
		t.Fatalf("ranking context = %+v", data)
	}

	low := engineWith(store.CoinSourceConfig{SourceType: "oi_low", OILowLimit: 5}, ranking)
	coins, _ = low.GetCandidateCoins()
	if got := symbols(coins); got != "FUSDT,EUSDT" {
		t.Fatalf("oi_low must only take falling OI, got %s", got)
	}
}

func TestPriceMoverSourcesStopAtTheSignChange(t *testing.T) {
	ranking := &fakeRanking{tickers: []binance.Ticker{
		{Symbol: "UPUSDT", PriceChangePercent: 9, QuoteVolume: 9e8},
		{Symbol: "FLATUSDT", PriceChangePercent: 0, QuoteVolume: 9e8},
		{Symbol: "DOWNUSDT", PriceChangePercent: -7, QuoteVolume: 9e8},
	}}
	gainers, _ := engineWith(store.CoinSourceConfig{SourceType: "gainers", GainersLimit: 3}, ranking).GetCandidateCoins()
	if got := symbols(gainers); got != "UPUSDT" {
		t.Fatalf("gainers = %s", got)
	}
	losers, _ := engineWith(store.CoinSourceConfig{SourceType: "losers", LosersLimit: 3}, ranking).GetCandidateCoins()
	if got := symbols(losers); got != "DOWNUSDT" {
		t.Fatalf("losers = %s", got)
	}
	volume, _ := engineWith(store.CoinSourceConfig{SourceType: "volume_top", VolumeTopLimit: 2}, ranking).GetCandidateCoins()
	if got := symbols(volume); got != "UPUSDT,FLATUSDT" {
		t.Fatalf("volume_top = %s", got)
	}
}

func TestMixedSourcesMergeInOrderWithStatic(t *testing.T) {
	ranking := &fakeRanking{
		tickers: []binance.Ticker{{Symbol: "BTCUSDT", PriceChangePercent: 2, QuoteVolume: 9e9}},
		oi:      []binance.OIChange{oiChange("SOLUSDT", 12, 9e8), oiChange("BTCUSDT", 3, 9e9)},
	}
	e := engineWith(store.CoinSourceConfig{
		SourceType: "mixed", UseOITop: true, OITopLimit: 2, UseVolumeTop: true, VolumeTopLimit: 1,
		StaticCoins: []string{"ETHUSDT", "BTCUSDT"},
	}, ranking)
	coins, err := e.GetCandidateCoins()
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(coins); got != "SOLUSDT,BTCUSDT,ETHUSDT" {
		t.Fatalf("mixed = %s", got)
	}
	if got := strings.Join(coins[1].Sources, "+"); got != "oi_top+volume_top+static" {
		t.Fatalf("BTC sources = %s", got)
	}
}

func TestRankingContextIsRenderedInUserPrompt(t *testing.T) {
	text := formatRankingContext(&OITopData{Rank: 1, OIDeltaPercent: 12.5, OIDeltaValue: 3_000_000, PriceDeltaPercent: -2, QuoteVolume24h: 450_000_000, Window: "4h"})
	for _, want := range []string{"Ranking #1", "OI change (4h): +12.50%", "24h volume: 450.0M USDT", "24h price change: -2.00%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ranking context %q missing %q", text, want)
		}
	}
}

func TestOKXVenueSkipsCoinsNotListedOnOKX(t *testing.T) {
	ranking := &fakeRanking{tickers: []binance.Ticker{
		{Symbol: "BTCUSDT", QuoteVolume: 9e9},
		{Symbol: "1000PEPEUSDT", QuoteVolume: 8e9},
		{Symbol: "ETHUSDT", QuoteVolume: 7e9},
		{Symbol: "SOLUSDT", QuoteVolume: 6e9},
	}}
	e := engineWith(store.CoinSourceConfig{SourceType: "volume_top", VolumeTopLimit: 2}, ranking).WithVenue("okx")
	e.listing = func(context.Context) (map[string]bool, error) {
		return map[string]bool{"BTCUSDT": true, "ETHUSDT": true, "SOLUSDT": true}, nil
	}
	coins, err := e.GetCandidateCoins()
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(coins); got != "BTCUSDT,ETHUSDT" {
		t.Fatalf("okx volume_top = %s, want BTCUSDT,ETHUSDT", got)
	}

	e.listing = func(context.Context) (map[string]bool, error) { return nil, fmt.Errorf("okx down") }
	if _, err := e.GetCandidateCoins(); err == nil {
		t.Fatal("expected an error when the OKX listing is unavailable")
	}

	binanceEngine := engineWith(store.CoinSourceConfig{SourceType: "volume_top", VolumeTopLimit: 2}, ranking).WithVenue("binance")
	coins, _ = binanceEngine.GetCandidateCoins()
	if got := symbols(coins); got != "BTCUSDT,1000PEPEUSDT" {
		t.Fatalf("binance volume_top = %s", got)
	}
}
