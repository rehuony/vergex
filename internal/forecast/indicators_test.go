package forecast

import (
	"math"
	"testing"

	"vergex/internal/market"
)

func TestIndicatorsMatchWhatTheFactorsReadAtEachCandle(t *testing.T) {
	klines := walk(WindowBars+100, 0.3)
	indicators := IndicatorSeries(klines, 100)
	if len(indicators.EMA20) != 100 || len(indicators.RSI) != 100 || len(indicators.VWAP) != 100 {
		t.Fatalf("points = %d, %d, %d", len(indicators.EMA20), len(indicators.RSI), len(indicators.VWAP))
	}
	window := klines[len(klines)-WindowBars:]
	last := len(indicators.EMA20) - 1
	upper, middle, lower := market.Bollinger(window, 20, 2)
	vwap, _ := rollingVWAP(window, 24)
	for name, pair := range map[string][2]float64{
		"ema20":  {indicators.EMA20[last].Value, market.EMA(window, 20)},
		"ema50":  {indicators.EMA50[last].Value, market.EMA(window, 50)},
		"upper":  {indicators.BollUpper[last].Value, upper},
		"middle": {indicators.BollMiddle[last].Value, middle},
		"lower":  {indicators.BollLower[last].Value, lower},
		"vwap":   {indicators.VWAP[last].Value, vwap},
		"rsi":    {indicators.RSI[last].Value, market.RSI(window, 14)},
	} {
		if math.Abs(pair[0]-pair[1]) > 1e-9 {
			t.Fatalf("%s = %v, the factor read %v", name, pair[0], pair[1])
		}
	}
	if indicators.EMA20[last].Time != klines[len(klines)-1].OpenTime/1000 {
		t.Fatal("points should carry their candle's time in seconds")
	}
}

func TestIndicatorsSkipCandlesWithoutEnoughHistory(t *testing.T) {
	indicators := IndicatorSeries(walk(60, 0.3), 60)
	if len(indicators.EMA50) != 60-indicatorWarmup+1 {
		t.Fatalf("points = %d", len(indicators.EMA50))
	}
	if empty := IndicatorSeries(walk(10, 0.3), 10); len(empty.EMA20) != 0 {
		t.Fatalf("points = %d from 10 candles", len(empty.EMA20))
	}
}
