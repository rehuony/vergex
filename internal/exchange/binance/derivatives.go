package binance

import (
	"context"
	"net/url"
	"strconv"
)

// Point is one timed value of a derivatives series.
type Point struct {
	Time  int64 // Unix milliseconds
	Value float64
}

func timedQuery(symbol, period string, startMs int64, limit int) url.Values {
	query := url.Values{"symbol": {symbol}, "limit": {strconv.Itoa(limit)}}
	if period != "" {
		query.Set("period", period)
	}
	if startMs > 0 {
		query.Set("startTime", strconv.FormatInt(startMs, 10))
	}
	return query
}

// OpenInterestHistory returns open interest value in USDT, oldest first. Binance
// keeps about 30 days. Period is one of 5m, 15m, 30m, 1h, 2h, 4h, 6h, 12h, 1d.
func (c *DataClient) OpenInterestHistory(ctx context.Context, symbol, period string, startMs int64, limit int) ([]Point, error) {
	var rows []struct {
		Value     string `json:"sumOpenInterestValue"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := c.get(ctx, BudgetData, 1, "/futures/data/openInterestHist", timedQuery(symbol, period, startMs, limit), &rows); err != nil {
		return nil, err
	}
	points := make([]Point, 0, len(rows))
	for _, row := range rows {
		if value, err := strconv.ParseFloat(row.Value, 64); err == nil {
			points = append(points, Point{Time: row.Timestamp, Value: value})
		}
	}
	return points, nil
}

// LongShortRatio returns the global long/short account ratio, oldest first.
func (c *DataClient) LongShortRatio(ctx context.Context, symbol, period string, startMs int64, limit int) ([]Point, error) {
	var rows []struct {
		Ratio     string `json:"longShortRatio"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := c.get(ctx, BudgetData, 1, "/futures/data/globalLongShortAccountRatio", timedQuery(symbol, period, startMs, limit), &rows); err != nil {
		return nil, err
	}
	points := make([]Point, 0, len(rows))
	for _, row := range rows {
		if value, err := strconv.ParseFloat(row.Ratio, 64); err == nil {
			points = append(points, Point{Time: row.Timestamp, Value: value})
		}
	}
	return points, nil
}

// FundingHistory returns settled funding rates, oldest first.
func (c *DataClient) FundingHistory(ctx context.Context, symbol string, startMs int64, limit int) ([]Point, error) {
	var rows []struct {
		Rate string `json:"fundingRate"`
		Time int64  `json:"fundingTime"`
	}
	if err := c.get(ctx, BudgetFunding, 1, "/fapi/v1/fundingRate", timedQuery(symbol, "", startMs, limit), &rows); err != nil {
		return nil, err
	}
	points := make([]Point, 0, len(rows))
	for _, row := range rows {
		if value, err := strconv.ParseFloat(row.Rate, 64); err == nil {
			points = append(points, Point{Time: row.Time, Value: value})
		}
	}
	return points, nil
}

// TickerPrices returns the latest price of every symbol in one request.
func (c *DataClient) TickerPrices(ctx context.Context) (map[string]float64, error) {
	var rows []struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := c.get(ctx, BudgetWeight, 2, "/fapi/v1/ticker/price", nil, &rows); err != nil {
		return nil, err
	}
	prices := make(map[string]float64, len(rows))
	for _, row := range rows {
		if value, err := strconv.ParseFloat(row.Price, 64); err == nil {
			prices[row.Symbol] = value
		}
	}
	return prices, nil
}
