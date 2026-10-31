package market

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"
)

type MarketSymbol struct {
	Symbol             string  `json:"symbol"`
	Base               string  `json:"base"`
	Category           string  `json:"category"`
	Dex                string  `json:"dex,omitempty"`
	Volume24H          float64 `json:"volume_24h"`
	Price              string  `json:"price"`
	ExecutionSupported bool    `json:"execution_supported"`
}
type MarketList struct {
	Venue     string         `json:"venue"`
	Markets   []MarketSymbol `json:"markets"`
	FetchedAt time.Time      `json:"fetched_at"`
	Sources   []string       `json:"sources"`
	Warnings  []string       `json:"warnings,omitempty"`
}

// Discovery only reads public exchange metadata and 24h notional volume. It
// cannot use fee wallets, submit orders, or substitute one venue for another.
func (c *Client) Markets(ctx context.Context, venue string) (MarketList, error) {
	if venue != "binance" && venue != "okx" {
		return MarketList{}, errors.New("Unsupported discovery venue")
	}
	result, err, _ := c.discoveryFlight.Do(venue, func() (any, error) {
		c.discoveryMu.Lock()
		cached, exists := c.discoveryCache[venue]
		c.discoveryMu.Unlock()
		if exists && time.Since(cached.FetchedAt) < time.Minute {
			return cached, nil
		}
		deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out := MarketList{Venue: venue, Markets: []MarketSymbol{}, FetchedAt: time.Now().UTC()}
		var err error
		if venue == "binance" {
			err = c.binanceMarkets(deadline, &out)
		} else {
			err = c.okxMarkets(deadline, &out)
		}
		if err != nil {
			return MarketList{}, err
		}
		sort.Slice(out.Markets, func(i, j int) bool {
			if out.Markets[i].Volume24H == out.Markets[j].Volume24H {
				return out.Markets[i].Symbol < out.Markets[j].Symbol
			}
			return out.Markets[i].Volume24H > out.Markets[j].Volume24H
		})
		c.discoveryMu.Lock()
		if c.discoveryCache == nil {
			c.discoveryCache = map[string]MarketList{}
		}
		c.discoveryCache[venue] = out
		c.discoveryMu.Unlock()
		return out, nil
	})
	if err != nil {
		return MarketList{}, err
	}
	return result.(MarketList), nil
}
func validVolume(value string) (float64, bool) {
	v, e := strconv.ParseFloat(value, 64)
	return v, e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}
func (c *Client) binanceMarkets(ctx context.Context, out *MarketList) error {
	var info struct {
		Symbols []struct{ Symbol, BaseAsset, QuoteAsset, MarginAsset, Status, ContractType string }
	}
	var tickers []struct{ Symbol, QuoteVolume, LastPrice string }
	if err := c.request(ctx, "GET", c.BinanceFutures+"/fapi/v1/exchangeInfo", nil, &info); err != nil {
		return err
	}
	if err := c.request(ctx, "GET", c.BinanceFutures+"/fapi/v1/ticker/24hr", nil, &tickers); err != nil {
		return err
	}
	quotes := map[string]struct {
		volume float64
		price  string
	}{}
	for _, t := range tickers {
		if v, ok := validVolume(t.QuoteVolume); ok {
			quotes[t.Symbol] = struct {
				volume float64
				price  string
			}{v, t.LastPrice}
		}
	}
	for _, m := range info.Symbols {
		// Crypto only: TRADIFI_PERPETUAL (stocks, commodities, FX) is excluded.
		if m.Status != "TRADING" || m.QuoteAsset != "USDT" || m.MarginAsset != "USDT" || m.ContractType != "PERPETUAL" || !symbolOK(m.Symbol) {
			continue
		}
		q, ok := quotes[m.Symbol]
		if !ok {
			continue
		}
		out.Markets = append(out.Markets, MarketSymbol{Symbol: m.Symbol, Base: m.BaseAsset, Category: "crypto", Volume24H: q.volume, Price: q.price, ExecutionSupported: true})
	}
	out.Sources = []string{c.BinanceFutures + "/fapi/v1/exchangeInfo", c.BinanceFutures + "/fapi/v1/ticker/24hr"}
	return nil
}
