package binance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"
	"vergex/internal/market/instrument"
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

// LatestOpenInterest returns a symbol's open interest now. A value Binance does not give
// is an error, not a measured zero.
func (c *DataClient) LatestOpenInterest(ctx context.Context, symbol string) (float64, error) {
	var row struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := c.get(ctx, BudgetWeight, 1, "/fapi/v1/openInterest", url.Values{"symbol": {symbol}}, &row); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(row.OpenInterest, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, fmt.Errorf("open interest unavailable: invalid or missing value")
	}
	return value, nil
}

// LatestFundingRate returns a symbol's latest funding rate.
func (c *DataClient) LatestFundingRate(ctx context.Context, symbol string) (float64, error) {
	var row struct {
		Rate string `json:"lastFundingRate"`
	}
	if err := c.get(ctx, BudgetWeight, 1, "/fapi/v1/premiumIndex", url.Values{"symbol": {symbol}}, &row); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(row.Rate, 64)
	if err != nil {
		return 0, fmt.Errorf("funding rate unavailable: %w", err)
	}
	return value, nil
}

// Funding reads a USDT perpetual's latest funding rate and mark price.
func (c *PublicClient) Funding(ctx context.Context, symbol string) (instrument.Reading, error) {
	var r struct {
		Symbol  string `json:"symbol"`
		Funding string `json:"lastFundingRate"`
		Mark    string `json:"markPrice"`
		Time    int64  `json:"time"`
	}
	reading := instrument.Reading{Source: "https://fapi.binance.com/fapi/v1/premiumIndex"}
	if err := c.request(ctx, c.Futures+"/fapi/v1/premiumIndex?symbol="+url.QueryEscape(symbol), &r); err != nil {
		return reading, err
	}
	if r.Symbol != symbol || r.Time <= 0 {
		return reading, errors.New("invalid funding instrument or timestamp")
	}
	reading.Value, reading.Mark, reading.At = r.Funding, r.Mark, time.UnixMilli(r.Time).UTC()
	return reading, nil
}

// OpenInterest reads a USDT perpetual's open interest in its base asset.
func (c *PublicClient) OpenInterest(ctx context.Context, symbol string) (instrument.Reading, error) {
	var r struct {
		Symbol string `json:"symbol"`
		OI     string `json:"openInterest"`
		Time   int64  `json:"time"`
	}
	reading := instrument.Reading{Source: "https://fapi.binance.com/fapi/v1/openInterest"}
	if err := c.request(ctx, c.Futures+"/fapi/v1/openInterest?symbol="+url.QueryEscape(symbol), &r); err != nil {
		return reading, err
	}
	if r.Symbol != symbol || r.Time <= 0 {
		return reading, errors.New("invalid OI instrument or timestamp")
	}
	reading.Value, reading.At = r.OI, time.UnixMilli(r.Time).UTC()
	return reading, nil
}
