package okx

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"vergex/internal/market/instrument"
)

func validVolume(value string) (float64, bool) {
	v, e := strconv.ParseFloat(value, 64)
	return v, e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

// Listings reads the USDT linear perpetuals on offer and their estimated 24h notional volume.
func (c *PublicClient) Listings(ctx context.Context) (instrument.Listings, error) {
	var out instrument.Listings
	var info struct {
		Code string
		Data []struct{ InstId, InstType, CtType, SettleCcy, State, CtValCcy, CtMult string }
	}
	var tickers struct {
		Code string
		Data []struct{ InstId, Last, VolCcy24h string }
	}
	p1 := "/api/v5/public/instruments?instType=SWAP"
	p2 := "/api/v5/market/tickers?instType=SWAP"
	if e := c.request(ctx, c.Base+p1, &info); e != nil {
		return out, e
	}
	if e := c.request(ctx, c.Base+p2, &tickers); e != nil {
		return out, e
	}
	if info.Code != "0" || tickers.Code != "0" {
		return out, errors.New("OKX market discovery unavailable")
	}
	active := map[string]string{}
	for _, r := range info.Data {
		if r.InstType == "SWAP" && r.CtType == "linear" && r.SettleCcy == "USDT" && r.State == "live" && strings.HasSuffix(r.InstId, "-USDT-SWAP") && (r.CtMult == "" || r.CtMult == "1" || r.CtMult == "1.0") && instrument.ValidSymbol(r.InstId) {
			active[r.InstId] = r.CtValCcy
		}
	}
	for _, r := range tickers.Data {
		base, ok := active[r.InstId]
		v, valid := validVolume(r.VolCcy24h)
		price, pok := validVolume(r.Last)
		if !ok || !valid || !pok {
			continue
		}
		out.Markets = append(out.Markets, instrument.Listing{Symbol: r.InstId, Base: base, Volume24H: v * price, Price: r.Last})
	}
	out.Sources = []string{c.Base + p1, c.Base + p2}
	out.Warnings = []string{"OKX 24h notional volume is estimated from base volume and the latest price."}
	return out, nil
}
