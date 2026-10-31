package market

import (
	"context"
	"errors"
	"strings"
)

func (c *Client) okxMarkets(ctx context.Context, out *MarketList) error {
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
	if e := c.request(ctx, "GET", c.OKX+p1, nil, &info); e != nil {
		return e
	}
	if e := c.request(ctx, "GET", c.OKX+p2, nil, &tickers); e != nil {
		return e
	}
	if info.Code != "0" || tickers.Code != "0" {
		return errors.New("OKX market discovery unavailable")
	}
	active := map[string]string{}
	for _, r := range info.Data {
		if r.InstType == "SWAP" && r.CtType == "linear" && r.SettleCcy == "USDT" && r.State == "live" && strings.HasSuffix(r.InstId, "-USDT-SWAP") && (r.CtMult == "" || r.CtMult == "1" || r.CtMult == "1.0") && symbolOK(r.InstId) {
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
		out.Markets = append(out.Markets, MarketSymbol{Symbol: r.InstId, Base: base, Category: "crypto", Volume24H: v * price, Price: r.Last, ExecutionSupported: true})
	}
	out.Sources = []string{c.OKX + p1, c.OKX + p2}
	out.Warnings = []string{"OKX 24h notional volume is estimated from base volume and the latest price."}
	return nil
}
