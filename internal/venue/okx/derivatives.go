package okx

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"vergex/internal/market/instrument"
)

func (c *PublicClient) reading(ctx context.Context, path, symbol string, value func(funding, oi string) string) (instrument.Reading, error) {
	var r struct {
		Code string `json:"code"`
		Data []struct {
			Instrument string `json:"instId"`
			Funding    string `json:"fundingRate"`
			OI         string `json:"oi"`
			Time       string `json:"ts"`
		} `json:"data"`
	}
	reading := instrument.Reading{Source: "https://www.okx.com" + strings.Split(path, "?")[0]}
	if err := c.request(ctx, c.Base+path+url.QueryEscape(symbol), &r); err != nil {
		return reading, err
	}
	if r.Code != "0" || len(r.Data) != 1 || r.Data[0].Instrument != symbol {
		return reading, errors.New("invalid OKX plugin response")
	}
	row := r.Data[0]
	ms, err := strconv.ParseInt(row.Time, 10, 64)
	if err != nil || ms <= 0 {
		return reading, errors.New("invalid OKX plugin timestamp")
	}
	reading.Value, reading.At = value(row.Funding, row.OI), time.UnixMilli(ms).UTC()
	return reading, nil
}

// Funding reads a USDT perpetual's current funding rate.
func (c *PublicClient) Funding(ctx context.Context, symbol string) (instrument.Reading, error) {
	return c.reading(ctx, "/api/v5/public/funding-rate?instId=", symbol, func(funding, _ string) string { return funding })
}

// OpenInterest reads a USDT perpetual's open contracts.
func (c *PublicClient) OpenInterest(ctx context.Context, symbol string) (instrument.Reading, error) {
	return c.reading(ctx, "/api/v5/public/open-interest?instType=SWAP&instId=", symbol, func(_, oi string) string { return oi })
}
