package market

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"vergex/internal/exchange/binance"
	"vergex/internal/exchange/okx"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
)

// LiveSource supplies candles and prices from the live streams and local history.
type LiveSource interface {
	// Recent returns Binance's latest candles, oldest first; the last may still be open.
	Recent(ctx context.Context, symbol, interval string, limit int) ([]Kline, error)
	// WatchMark keeps a symbol's mark price coming for a few minutes.
	WatchMark(exchange, symbol string)
	// Mark returns a symbol's mark price if a stream delivered it just now.
	Mark(exchange, symbol string) (float64, bool)
}

const klineReadWait = 20 * time.Second

var (
	liveSource atomic.Pointer[LiveSource]
	// restKlines answers until a LiveSource is set, or if there is none.
	restKlines = binance.NewDataClient(binance.NewLimiter())
)

// SetLiveSource says where candles and live prices come from from now on.
func SetLiveSource(source LiveSource) { liveSource.Store(&source) }

// WatchMark keeps a symbol's live mark price coming for a few minutes.
func WatchMark(exchange, symbol string) {
	if source := liveSource.Load(); source != nil {
		(*source).WatchMark(exchange, symbol)
	}
}

// MarkPrice returns a symbol's live mark price, if there is a current one.
func MarkPrice(exchange, symbol string) (float64, bool) {
	if source := liveSource.Load(); source != nil {
		return (*source).Mark(exchange, symbol)
	}
	return 0, false
}

// getKlines returns a symbol's latest candles, the last possibly still open. OKX
// traders see OKX's candles, read directly since they are cheap and always current,
// and Binance's if OKX has none.
func getKlines(symbol, interval, exchange string, limit int) ([]Kline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), klineReadWait)
	defer cancel()
	if strings.EqualFold(exchange, "okx") {
		candles, err := okx.Klines(ctx, symbol, interval, limit)
		if err == nil && len(candles) > 0 {
			return fromOKX(candles, interval), nil
		}
		logger.Warnf("⚠️ OKX %s candles for %s unavailable, falling back to Binance: %v", interval, symbol, err)
	}
	if source := liveSource.Load(); source != nil {
		return (*source).Recent(ctx, symbol, interval, limit)
	}
	candles, err := restKlines.TradingKlines(ctx, symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	return FromBinance(candles, interval), nil
}

// FromBinance converts candles read from the exchange.
func FromBinance(candles []binance.Candle, interval string) []Kline {
	step := int64(parseTimeframeToMinutes(interval)) * 60 * 1000
	klines := make([]Kline, len(candles))
	for i, c := range candles {
		klines[i] = Kline{
			OpenTime: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
			CloseTime: c.OpenTime + step - 1, QuoteVolume: c.QuoteVolume, Trades: int(c.Trades),
			TakerBuyBaseVolume: c.TakerBuyVolume, TakerBuyQuoteVolume: c.TakerBuyQuote,
		}
	}
	return klines
}

func fromOKX(candles []okx.Candle, interval string) []Kline {
	step := int64(parseTimeframeToMinutes(interval)) * 60 * 1000
	klines := make([]Kline, len(candles))
	for i, c := range candles {
		klines[i] = Kline{
			OpenTime: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
			CloseTime: c.OpenTime + step - 1, QuoteVolume: c.QuoteVolume,
		}
	}
	return klines
}

// calculateTimeframeSeries calculates series data for a single timeframe
func calculateTimeframeSeries(klines []Kline, timeframe string, count int) *TimeframeSeriesData {
	if count <= 0 {
		count = 10 // default
	}

	data := &TimeframeSeriesData{
		Timeframe:   timeframe,
		Klines:      make([]KlineBar, 0, count),
		MidPrices:   make([]float64, 0, count),
		EMA20Values: make([]float64, 0, count),
		EMA50Values: make([]float64, 0, count),
		MACDValues:  make([]float64, 0, count),
		RSI7Values:  make([]float64, 0, count),
		RSI14Values: make([]float64, 0, count),
		Volume:      make([]float64, 0, count),
		BOLLUpper:   make([]float64, 0, count),
		BOLLMiddle:  make([]float64, 0, count),
		BOLLLower:   make([]float64, 0, count),
	}

	// Get latest N data points based on count from config
	start := len(klines) - count
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		// Store full OHLCV kline data
		data.Klines = append(data.Klines, KlineBar{
			Time:   klines[i].OpenTime,
			Open:   klines[i].Open,
			High:   klines[i].High,
			Low:    klines[i].Low,
			Close:  klines[i].Close,
			Volume: klines[i].Volume,
		})

		// Keep MidPrices and Volume for backward compatibility
		data.MidPrices = append(data.MidPrices, klines[i].Close)
		data.Volume = append(data.Volume, klines[i].Volume)

		// Calculate EMA20 for each point
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// Calculate EMA50 for each point
		if i >= 49 {
			ema50 := calculateEMA(klines[:i+1], 50)
			data.EMA50Values = append(data.EMA50Values, ema50)
		}

		// Calculate MACD for each point
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// Calculate RSI for each point
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}

		// Calculate Bollinger Bands (period 20, std dev multiplier 2)
		if i >= 19 {
			upper, middle, lower := calculateBOLL(klines[:i+1], 20, 2.0)
			data.BOLLUpper = append(data.BOLLUpper, upper)
			data.BOLLMiddle = append(data.BOLLMiddle, middle)
			data.BOLLLower = append(data.BOLLLower, lower)
		}
	}

	// Calculate ATR14
	data.ATR14 = calculateATR(klines, 14)

	return data
}

// calculatePriceChangeByBars calculates how many K-lines to look back for price change based on timeframe
func calculatePriceChangeByBars(klines []Kline, timeframe string, targetMinutes int) float64 {
	if len(klines) < 2 {
		return 0
	}

	// Parse timeframe to minutes
	tfMinutes := parseTimeframeToMinutes(timeframe)
	if tfMinutes <= 0 {
		return 0
	}

	// Calculate how many K-lines to look back
	barsBack := targetMinutes / tfMinutes
	if barsBack < 1 {
		barsBack = 1
	}

	currentPrice := klines[len(klines)-1].Close
	idx := len(klines) - 1 - barsBack
	if idx < 0 {
		idx = 0
	}

	oldPrice := klines[idx].Close
	if oldPrice > 0 {
		return ((currentPrice - oldPrice) / oldPrice) * 100
	}
	return 0
}

// parseTimeframeToMinutes parses timeframe string to minutes
func parseTimeframeToMinutes(tf string) int {
	switch tf {
	case "1m":
		return 1
	case "3m":
		return 3
	case "5m":
		return 5
	case "15m":
		return 15
	case "30m":
		return 30
	case "1h":
		return 60
	case "2h":
		return 120
	case "4h":
		return 240
	case "6h":
		return 360
	case "8h":
		return 480
	case "12h":
		return 720
	case "1d":
		return 1440
	case "3d":
		return 4320
	case "1w":
		return 10080
	default:
		return 0
	}
}

// calculateIntradaySeries calculates intraday series data
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 10),
		EMA20Values: make([]float64, 0, 10),
		MACDValues:  make([]float64, 0, 10),
		RSI7Values:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
		Volume:      make([]float64, 0, 10),
	}

	// Get latest 10 data points
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)
		data.Volume = append(data.Volume, klines[i].Volume)

		// Calculate EMA20 for each point
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// Calculate MACD for each point
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// Calculate RSI for each point
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	// Calculate 3m ATR14
	data.ATR14 = calculateATR(klines, 14)

	return data
}

// calculateLongerTermData calculates longer-term data
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// Calculate EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// Calculate ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// Calculate volume
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// Calculate average volume
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// Calculate MACD and RSI series
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// GetBoxData fetches 1h klines and calculates box data for a symbol
func GetBoxData(symbol string) (*BoxData, error) {
	symbol = instrument.Normalize(symbol)

	// Fetch 500 1h klines
	var klines []Kline
	var err error

	klines, err = getKlines(symbol, "1h", "binance", LongBoxPeriod)

	if err != nil {
		return nil, fmt.Errorf("failed to get 1h klines: %w", err)
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("no kline data available")
	}

	currentPrice := klines[len(klines)-1].Close

	return calculateBoxData(klines, currentPrice), nil
}
