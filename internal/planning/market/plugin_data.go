package market

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"vergex/internal/planning/plugins"
)

func (c *Client) PluginData(ctx context.Context, id string, q plugins.Request) (plugins.Result, error) {
	out := plugins.Result{Venue: q.Venue, Symbol: q.Symbol, Observed: time.Now().UTC(), Mode: "public", Notice: "Read-only snapshot, not a direction forecast or fill evidence. Funding is excluded from the current risk estimate. One OI snapshot does not measure OI change."}
	if id != "funding" && id != "open_interest" {
		return out, errors.New("unknown public data plugin")
	}
	switch q.Venue {
	case "binance":
		if !strings.HasSuffix(q.Symbol, "USDT") {
			return out, errors.New("This plugin supports Binance USDT futures only")
		}
		if id == "funding" {
			var r struct {
				Symbol  string `json:"symbol"`
				Funding string `json:"lastFundingRate"`
				Mark    string `json:"markPrice"`
				Time    int64  `json:"time"`
			}
			out.Source = "https://fapi.binance.com/fapi/v1/premiumIndex"
			if err := c.request(ctx, http.MethodGet, c.BinanceFutures+"/fapi/v1/premiumIndex?symbol="+url.QueryEscape(q.Symbol), nil, &r); err != nil {
				return out, err
			}
			if r.Symbol != q.Symbol || r.Time <= 0 {
				return out, errors.New("invalid funding instrument or timestamp")
			}
			out.SourceTime = time.UnixMilli(r.Time).UTC()
			out.Metrics = []plugins.Metric{{Key: "funding_rate", Label: "Latest funding rate", Value: r.Funding, Unit: "fraction"}, {Key: "mark_price", Label: "Mark price", Value: r.Mark, Unit: "USDT"}}
		} else {
			var r struct {
				Symbol string `json:"symbol"`
				OI     string `json:"openInterest"`
				Time   int64  `json:"time"`
			}
			out.Source = "https://fapi.binance.com/fapi/v1/openInterest"
			if err := c.request(ctx, http.MethodGet, c.BinanceFutures+"/fapi/v1/openInterest?symbol="+url.QueryEscape(q.Symbol), nil, &r); err != nil {
				return out, err
			}
			if r.Symbol != q.Symbol || r.Time <= 0 {
				return out, errors.New("invalid OI instrument or timestamp")
			}
			out.SourceTime = time.UnixMilli(r.Time).UTC()
			out.Metrics = []plugins.Metric{{Key: "open_interest", Label: "Open interest", Value: r.OI, Unit: strings.TrimSuffix(q.Symbol, "USDT")}}
		}
	case "okx":
		if !strings.HasSuffix(q.Symbol, "-USDT-SWAP") {
			return out, errors.New("This plugin supports OKX USDT perpetuals only")
		}
		path := "/api/v5/public/open-interest?instType=SWAP&instId="
		if id == "funding" {
			path = "/api/v5/public/funding-rate?instId="
		}
		var r struct {
			Code string `json:"code"`
			Data []struct {
				Instrument string `json:"instId"`
				Funding    string `json:"fundingRate"`
				OI         string `json:"oi"`
				Time       string `json:"ts"`
			} `json:"data"`
		}
		out.Source = "https://www.okx.com" + strings.Split(path, "?")[0]
		if err := c.request(ctx, http.MethodGet, c.OKX+path+url.QueryEscape(q.Symbol), nil, &r); err != nil {
			return out, err
		}
		if r.Code != "0" || len(r.Data) != 1 || r.Data[0].Instrument != q.Symbol {
			return out, errors.New("invalid OKX plugin response")
		}
		row := r.Data[0]
		ms, err := strconv.ParseInt(row.Time, 10, 64)
		if err != nil || ms <= 0 {
			return out, errors.New("invalid OKX plugin timestamp")
		}
		out.SourceTime = time.UnixMilli(ms).UTC()
		if id == "funding" {
			out.Metrics = []plugins.Metric{{Key: "funding_rate", Label: "Current funding rate", Value: row.Funding, Unit: "fraction"}}
		} else {
			out.Metrics = []plugins.Metric{{Key: "open_interest", Label: "Open contracts", Value: row.OI, Unit: "contracts"}}
		}
	default:
		return out, errors.New("unsupported plugin venue")
	}
	out.Observed = time.Now().UTC()
	return out, nil
}
