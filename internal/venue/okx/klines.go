package okx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"vergex/internal/market/instrument"
	"vergex/internal/service"
)

// maxCandles is the most candles /market/candles returns.
const maxCandles = 300

// barNames maps an interval to OKX's name for it. Long bars use the UTC variants,
// to open when Binance's do.
var barNames = map[string]string{
	"1m": "1m", "3m": "3m", "5m": "5m", "15m": "15m", "30m": "30m", "1h": "1H", "2h": "2H",
	"4h": "4H", "6h": "6Hutc", "12h": "12Hutc", "1d": "1Dutc",
}

// Candle is one OKX candle, with volume in the base asset rather than contracts.
type Candle struct {
	OpenTime    int64 // Unix milliseconds
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	QuoteVolume float64
	Closed      bool
}

// InstrumentID returns the swap a generic symbol names: BTCUSDT -> BTC-USDT-SWAP.
func InstrumentID(symbol string) string {
	if strings.Contains(symbol, "-") {
		return symbol
	}
	return strings.TrimSuffix(symbol, "USDT") + "-USDT-SWAP"
}

var klineClient = &http.Client{Timeout: 15 * time.Second, Transport: service.VenueTransport}

// klineBaseURL is a variable so tests can point it at a fake server.
var klineBaseURL = defaultBaseURL

// Klines returns a symbol's latest candles, oldest first. The last may still be open.
func Klines(ctx context.Context, symbol, interval string, limit int) ([]Candle, error) {
	bar, ok := barNames[interval]
	if !ok {
		return nil, fmt.Errorf("unsupported interval %q", interval)
	}
	query := url.Values{"instId": {InstrumentID(symbol)}, "bar": {bar}, "limit": {strconv.Itoa(min(limit, maxCandles))}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, klineBaseURL+"/api/v5/market/candles?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := klineClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okx status %d", resp.StatusCode)
	}
	var result struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Code != "0" {
		return nil, fmt.Errorf("okx error %s: %s", result.Code, result.Msg)
	}
	// OKX lists the newest first.
	candles := make([]Candle, 0, len(result.Data))
	for i := len(result.Data) - 1; i >= 0; i-- {
		candle, err := ParseCandle(result.Data[i])
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

// ParseCandle reads OKX's candle row: time, open, high, low, close, volume in
// contracts, in the base asset, in the quote asset, and whether it has closed.
func ParseCandle(row []string) (Candle, error) {
	if len(row) < 9 {
		return Candle{}, errors.New("okx candle: too few fields")
	}
	openTime, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return Candle{}, errors.New("okx candle: invalid time")
	}
	candle := Candle{OpenTime: openTime, Closed: row[8] == "1"}
	for _, field := range []struct {
		index int
		dest  *float64
	}{
		{1, &candle.Open}, {2, &candle.High}, {3, &candle.Low}, {4, &candle.Close}, {6, &candle.Volume}, {7, &candle.QuoteVolume},
	} {
		value, err := strconv.ParseFloat(row[field.index], 64)
		if err != nil {
			return Candle{}, errors.New("okx candle: invalid number")
		}
		*field.dest = value
	}
	return candle, nil
}

// ExactCandles reads a perpetual's candles with their numbers as OKX wrote them, the
// newest before toSeconds when it is set. intervalSeconds says how long a candle is, so a
// request for recent candles can go to the faster endpoint.
func (c *PublicClient) ExactCandles(ctx context.Context, symbol, interval string, intervalSeconds, toSeconds int64, limit int) ([]instrument.ExactCandle, error) {
	bar := map[string]string{"1m": "1m", "5m": "5m", "15m": "15m", "1h": "1H", "4h": "4H", "1d": "1Dutc"}[interval]
	values := url.Values{"instId": {symbol}, "bar": {bar}, "limit": {strconv.Itoa(limit)}}
	path := "/api/v5/market/candles"
	if toSeconds > 0 && toSeconds < time.Now().Unix()-intervalSeconds {
		path = "/api/v5/market/history-candles"
		values.Set("after", strconv.FormatInt(toSeconds*1000, 10))
	}
	var raw struct {
		Code string
		Data [][]string
	}
	if err := c.request(ctx, c.Base+path+"?"+values.Encode(), &raw); err != nil {
		return nil, err
	}
	if raw.Code != "0" {
		return nil, errors.New("OKX rejected candle request")
	}
	var rows []instrument.ExactCandle
	for _, r := range raw.Data {
		if len(r) < 7 {
			return nil, errors.New("invalid OKX candle")
		}
		ms, err := strconv.ParseInt(r[0], 10, 64)
		if err != nil {
			return nil, errors.New("invalid OKX candle time")
		}
		rows = append(rows, instrument.ExactCandle{Time: ms / 1000, Open: r[1], High: r[2], Low: r[3], Close: r[4], Volume: r[6]})
	}
	return rows, nil
}
