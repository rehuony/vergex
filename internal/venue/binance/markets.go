package binance

import (
	"context"
	"math"
	"strconv"

	"vergex/internal/market/instrument"
)

func validVolume(value string) (float64, bool) {
	v, e := strconv.ParseFloat(value, 64)
	return v, e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

// Listings reads the USDT crypto perpetuals on offer and their 24h notional volume.
func (c *PublicClient) Listings(ctx context.Context) (instrument.Listings, error) {
	var out instrument.Listings
	var info struct {
		Symbols []struct{ Symbol, BaseAsset, QuoteAsset, MarginAsset, Status, ContractType string }
	}
	var tickers []struct{ Symbol, QuoteVolume, LastPrice string }
	if err := c.request(ctx, c.Futures+"/fapi/v1/exchangeInfo", &info); err != nil {
		return out, err
	}
	if err := c.request(ctx, c.Futures+"/fapi/v1/ticker/24hr", &tickers); err != nil {
		return out, err
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
		if m.Status != "TRADING" || m.QuoteAsset != "USDT" || m.MarginAsset != "USDT" || m.ContractType != "PERPETUAL" || !instrument.ValidSymbol(m.Symbol) {
			continue
		}
		q, ok := quotes[m.Symbol]
		if !ok {
			continue
		}
		out.Markets = append(out.Markets, instrument.Listing{Symbol: m.Symbol, Base: m.BaseAsset, Volume24H: q.volume, Price: q.price})
	}
	out.Sources = []string{c.Futures + "/fapi/v1/exchangeInfo", c.Futures + "/fapi/v1/ticker/24hr"}
	return out, nil
}
