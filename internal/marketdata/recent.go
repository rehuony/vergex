package marketdata

import (
	"context"
	"errors"

	"vergex/internal/logger"
	"vergex/internal/market"
	"vergex/internal/provider/binance"
)

// Recent returns a symbol's latest candles for trading, oldest first, the last one
// still open. It answers from local history and the stream only when it can vouch
// for them: the stream is up and delivering the forming candle, and history joins
// up with it. Anything else gets the exchange's own answer, so a read is never
// silently stale. Reading also keeps the series subscribed and filling for next time.
func (s *Service) Recent(ctx context.Context, symbol, interval string, limit int) ([]market.Kline, error) {
	if series, err := s.Open(symbol, interval); err == nil {
		if klines, ok := s.localRecent(series, limit); ok {
			return klines, nil
		}
	} else if IntervalMillis(interval) != 0 {
		logger.Warnf("Market data: open %s %s: %v", symbol, interval, err)
	}
	candles, err := s.client.TradingKlines(ctx, symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	return market.FromBinance(candles, interval), nil
}

func (s *Service) localRecent(series *Series, limit int) ([]market.Kline, bool) {
	if !s.stream.Healthy() {
		return nil, false
	}
	step := IntervalMillis(series.Interval)
	current := s.now().UnixMilli() / step * step
	live, ok := s.hub.Latest(binance.Venue, series.Symbol, series.Interval)
	if !ok || live.OpenTime != current || series.LastClosedTime != current-step {
		return nil, false
	}
	candles, err := s.Store.Window(series, series.LastClosedTime, limit-1)
	if err != nil {
		if !errors.Is(err, ErrNotEnoughHistory) && !errors.Is(err, ErrIncomplete) {
			logger.Warnf("Market data: read %s %s: %v", series.Symbol, series.Interval, err)
		}
		return nil, false
	}
	forming := market.FromBinance([]binance.Candle{toBinanceCandle(live.Candle)}, series.Interval)
	return append(ToKlines(candles, series.Interval), forming...), true
}

// ToKlines converts stored candles for calculations.
func ToKlines(candles []Candle, interval string) []market.Kline {
	rows := make([]binance.Candle, len(candles))
	for i, c := range candles {
		rows[i] = binance.Candle{
			OpenTime: c.OpenTime, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
			QuoteVolume: c.QuoteVolume, Trades: c.Trades, TakerBuyVolume: c.TakerBuyVolume, TakerBuyQuote: c.TakerBuyQuote,
		}
	}
	return market.FromBinance(rows, interval)
}
