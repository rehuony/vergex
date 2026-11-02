package market

import (
	"context"
	"testing"
	"vergex/internal/venue/binance"
)

type fakeLiveSource struct {
	symbol, interval string
	limit            int
}

func (f *fakeLiveSource) Recent(_ context.Context, symbol, interval string, limit int) ([]Kline, error) {
	f.symbol, f.interval, f.limit = symbol, interval, limit
	return generateTestKlines(limit), nil
}

func (f *fakeLiveSource) WatchMark(string, string) {}

func (f *fakeLiveSource) OpenInterest(context.Context, string) (float64, error) { return 0, nil }

func (f *fakeLiveSource) FundingRate(context.Context, string) (float64, error) { return 0, nil }

func (f *fakeLiveSource) Mark(string, string) (float64, bool) { return 0, false }

func TestGetKlinesReadsTheLiveSource(t *testing.T) {
	defer liveSource.Store(liveSource.Load())
	source := &fakeLiveSource{}
	SetLiveSource(source)

	klines, err := Klines("BTCUSDT", "15m", "binance", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(klines) != 7 || source.symbol != "BTCUSDT" || source.interval != "15m" || source.limit != 7 {
		t.Fatalf("klines = %d, source = %+v", len(klines), source)
	}
}

func TestFromBinanceSetsCloseTimeAndFlows(t *testing.T) {
	klines := FromBinance([]binance.Candle{{OpenTime: 180000, Close: 2, Volume: 3, QuoteVolume: 4, Trades: 5, TakerBuyVolume: 6, TakerBuyQuote: 7}}, "3m")
	k := klines[0]
	if k.CloseTime != 359999 || k.Close != 2 || k.QuoteVolume != 4 || k.Trades != 5 || k.TakerBuyBaseVolume != 6 || k.TakerBuyQuoteVolume != 7 {
		t.Fatalf("kline = %+v", k)
	}
}
