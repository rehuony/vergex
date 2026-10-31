package forecast

import "vergex/internal/market"

// IndicatorPoint is one indicator's value at a candle, for the chart.
type IndicatorPoint struct {
	Time  int64   `json:"time"` // Unix seconds.
	Value float64 `json:"value"`
}

// Indicators are the series behind the factors, for the chart to draw.
type Indicators struct {
	EMA20      []IndicatorPoint `json:"ema20"`
	EMA50      []IndicatorPoint `json:"ema50"`
	BollUpper  []IndicatorPoint `json:"boll_upper"`
	BollMiddle []IndicatorPoint `json:"boll_middle"`
	BollLower  []IndicatorPoint `json:"boll_lower"`
	VWAP       []IndicatorPoint `json:"vwap"`
	RSI        []IndicatorPoint `json:"rsi"`
}

// The most warm-up an indicator needs before it means anything.
const indicatorWarmup = 50

// IndicatorSeries works out the indicators for the last shown candles. Each point
// reads the same window, with the same functions and parameters, as a forecast made
// at that candle, so what is drawn is what the factors saw.
func IndicatorSeries(klines []market.Kline, shown int) Indicators {
	indicators := Indicators{}
	first := max(len(klines)-shown, 0)
	for i := first; i < len(klines); i++ {
		if i+1 < indicatorWarmup {
			continue
		}
		window := klines[max(0, i+1-WindowBars) : i+1]
		time := klines[i].OpenTime / 1000
		add := func(series *[]IndicatorPoint, value float64) {
			*series = append(*series, IndicatorPoint{Time: time, Value: value})
		}
		add(&indicators.EMA20, market.EMA(window, 20))
		add(&indicators.EMA50, market.EMA(window, 50))
		upper, middle, lower := market.Bollinger(window, 20, 2)
		add(&indicators.BollUpper, upper)
		add(&indicators.BollMiddle, middle)
		add(&indicators.BollLower, lower)
		if vwap, ok := rollingVWAP(window, 24); ok {
			add(&indicators.VWAP, vwap)
		}
		add(&indicators.RSI, market.RSI(window, 14))
	}
	return indicators
}
