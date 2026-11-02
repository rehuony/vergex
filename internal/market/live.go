package market

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
	"vergex/internal/logger"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
)

// LiveSource supplies candles and prices from the live streams and local history.
type LiveSource interface {
	// Recent returns Binance's latest candles, oldest first; the last may still be open.
	Recent(ctx context.Context, symbol, interval string, limit int) ([]Kline, error)
	// WatchMark keeps a symbol's mark price coming for a few minutes.
	WatchMark(exchange, symbol string)
	// Mark returns a symbol's mark price if a stream delivered it just now.
	Mark(exchange, symbol string) (float64, bool)
	// OpenInterest returns a symbol's open interest now, under the request budget.
	OpenInterest(ctx context.Context, symbol string) (float64, error)
	// FundingRate returns a symbol's latest funding rate, under the request budget.
	FundingRate(ctx context.Context, symbol string) (float64, error)
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

// Klines returns a symbol's latest candles, the last possibly still open. OKX
// traders see OKX's candles, read directly since they are cheap and always current,
// and Binance's if OKX has none.
func Klines(symbol, interval, exchange string, limit int) ([]Kline, error) {
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
	step := int64(TimeframeMinutes(interval)) * 60 * 1000
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
	step := int64(TimeframeMinutes(interval)) * 60 * 1000
	klines := make([]Kline, len(candles))
	for i, c := range candles {
		klines[i] = Kline{
			OpenTime: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
			CloseTime: c.OpenTime + step - 1, QuoteVolume: c.QuoteVolume,
		}
	}
	return klines
}

// TimeframeMinutes is how many minutes a timeframe is; 0 for one it does not know
func TimeframeMinutes(tf string) int {
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

// OpenInterest returns a symbol's open interest now.
func OpenInterest(ctx context.Context, symbol string) (float64, error) {
	if source := liveSource.Load(); source != nil {
		return (*source).OpenInterest(ctx, symbol)
	}
	return restKlines.LatestOpenInterest(ctx, symbol)
}

// FundingRate returns a symbol's latest funding rate.
func FundingRate(ctx context.Context, symbol string) (float64, error) {
	if source := liveSource.Load(); source != nil {
		return (*source).FundingRate(ctx, symbol)
	}
	return restKlines.LatestFundingRate(ctx, symbol)
}
