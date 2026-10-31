package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Candle is one kline as Binance reports it.
type Candle struct {
	OpenTime       int64 // Unix milliseconds
	Open           float64
	High           float64
	Low            float64
	Close          float64
	Volume         float64
	QuoteVolume    float64
	Trades         int64
	TakerBuyVolume float64
	TakerBuyQuote  float64
}

// StatusError is a response Binance answered with an error status.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("binance status %d: %s", e.Code, e.Message)
}

// RateLimited reports an answer that paused every request, not just this one.
func RateLimited(err error) bool {
	var status *StatusError
	if !errors.As(err, &status) {
		return false
	}
	return status.Code == http.StatusTooManyRequests || status.Code == http.StatusForbidden || status.Code == http.StatusTeapot
}

// DataClient reads public market data under a Limiter. A failed request is not
// retried here: the caller queues it again, so nothing piles up during an outage.
type DataClient struct {
	baseURL string
	http    *http.Client
	limiter *Limiter
}

// NewDataClient creates a client that spends the given limiter's budgets.
func NewDataClient(limiter *Limiter) *DataClient {
	return &DataClient{baseURL: defaultBaseURL, http: &http.Client{Timeout: 20 * time.Second}, limiter: limiter}
}

// WithBaseURL points the client somewhere else, such as a test server.
func (c *DataClient) WithBaseURL(base string) *DataClient {
	c.baseURL = base
	return c
}

// Limiter returns the limiter the client spends.
func (c *DataClient) Limiter() *Limiter { return c.limiter }

func (c *DataClient) get(ctx context.Context, budget Budget, cost int, path string, query url.Values, out any) error {
	if err := c.limiter.Acquire(ctx, budget, cost); err != nil {
		return err
	}
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.limiter.Observe(resp.StatusCode, resp.Header)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &StatusError{Code: resp.StatusCode, Message: truncate(string(body), 200)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// klineWeight is the request weight Binance charges for a kline limit.
func klineWeight(limit int) int {
	switch {
	case limit < 100:
		return 1
	case limit < 500:
		return 2
	case limit <= 1000:
		return 5
	default:
		return 10
	}
}

// Klines returns candles from startMs (or the latest ones when startMs is 0),
// oldest first. The last one may not have closed yet.
func (c *DataClient) Klines(ctx context.Context, symbol, interval string, startMs int64, limit int) ([]Candle, error) {
	query := url.Values{"symbol": {symbol}, "interval": {interval}, "limit": {strconv.Itoa(limit)}}
	if startMs > 0 {
		query.Set("startTime", strconv.FormatInt(startMs, 10))
	}
	var rows [][]any
	if err := c.get(ctx, BudgetWeight, klineWeight(limit), "/fapi/v1/klines", query, &rows); err != nil {
		return nil, err
	}
	candles := make([]Candle, 0, len(rows))
	for _, row := range rows {
		candle, err := parseKline(row)
		if err != nil {
			return nil, fmt.Errorf("kline %s %s: %w", symbol, interval, err)
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

func parseKline(row []any) (Candle, error) {
	if len(row) < 11 {
		return Candle{}, fmt.Errorf("%d fields", len(row))
	}
	openTime, ok := row[0].(float64)
	if !ok {
		return Candle{}, errors.New("open time is not a number")
	}
	trades, ok := row[8].(float64)
	if !ok {
		return Candle{}, errors.New("trade count is not a number")
	}
	candle := Candle{OpenTime: int64(openTime), Trades: int64(trades)}
	for _, field := range []struct {
		index int
		dest  *float64
	}{
		{1, &candle.Open}, {2, &candle.High}, {3, &candle.Low}, {4, &candle.Close},
		{5, &candle.Volume}, {7, &candle.QuoteVolume}, {9, &candle.TakerBuyVolume}, {10, &candle.TakerBuyQuote},
	} {
		text, ok := row[field.index].(string)
		if !ok {
			return Candle{}, fmt.Errorf("field %d is not a string", field.index)
		}
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Candle{}, fmt.Errorf("field %d: %w", field.index, err)
		}
		*field.dest = value
	}
	return candle, nil
}
