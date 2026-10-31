// Package market provides read-only public REST book and instrument adapters.
package market

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"vergex/internal/planning/core"
	"vergex/internal/planning/instrument"
)

type Client struct {
	discoveryMu                  sync.Mutex
	discoveryCache               map[string]MarketList
	discoveryFlight              singleflight.Group
	HTTP                         *http.Client
	Binance, BinanceFutures, OKX string
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Second}, Binance: "https://data-api.binance.vision", BinanceFutures: "https://fapi.binance.com", OKX: "https://www.okx.com"}
}
func (c *Client) request(ctx context.Context, method, u string, body any, out any) error {
	var b []byte
	var err error
	if body != nil {
		b, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("market HTTP %d", resp.StatusCode)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	return dec.Decode(out)
}
func symbolOK(s string) bool { return instrument.ValidSymbol(s) }

func (c *Client) Snapshot(ctx context.Context, venue, symbol string) (core.Snapshot, error) {
	return c.SnapshotKind(ctx, venue, symbol, "perpetual")
}
func (c *Client) spotSnapshot(ctx context.Context, venue, symbol string) (core.Snapshot, error) {
	s := core.Snapshot{Venue: venue, Symbol: symbol, Kind: "spot"}
	if !symbolOK(symbol) {
		return s, errors.New("use uppercase venue-native symbol (BTCUSDT, BTC-USDT, BTC)")
	}
	switch venue {
	case "binance":
		var info struct {
			Symbols []struct {
				Symbol, Status       string
				IsSpotTradingAllowed bool
				Filters              []struct{ FilterType, TickSize, StepSize, MinQty, MinNotional string }
			}
		}
		if err := c.request(ctx, "GET", c.Binance+"/api/v3/exchangeInfo?symbol="+url.QueryEscape(symbol), nil, &info); err != nil {
			return s, err
		}
		if len(info.Symbols) != 1 || info.Symbols[0].Symbol != symbol || info.Symbols[0].Status != "TRADING" || !info.Symbols[0].IsSpotTradingAllowed {
			return s, errors.New("unsupported/inactive Binance spot symbol")
		}
		for _, f := range info.Symbols[0].Filters {
			switch f.FilterType {
			case "PRICE_FILTER":
				s.Tick = core.Decimal(f.TickSize)
			case "LOT_SIZE":
				s.Step = core.Decimal(f.StepSize)
				s.MinQty = core.Decimal(f.MinQty)
			case "MIN_NOTIONAL", "NOTIONAL":
				s.MinNotional = core.Decimal(f.MinNotional)
			}
		}
		var book struct{ Bids, Asks [][]string }
		if err := c.request(ctx, "GET", c.Binance+"/api/v3/depth?symbol="+url.QueryEscape(symbol)+"&limit=5", nil, &book); err != nil {
			return s, err
		}
		if len(book.Bids) == 0 || len(book.Asks) == 0 || len(book.Bids[0]) < 2 || len(book.Asks[0]) < 2 {
			return s, errors.New("empty Binance book")
		}
		s.Bid = core.Decimal(book.Bids[0][0])
		s.BidSize = core.Decimal(book.Bids[0][1])
		s.Ask = core.Decimal(book.Asks[0][0])
		s.AskSize = core.Decimal(book.Asks[0][1])
	case "okx":
		var info struct {
			Code string
			Data []struct{ InstId, State, TickSz, LotSz, MinSz string }
		}
		if err := c.request(ctx, "GET", c.OKX+"/api/v5/public/instruments?instType=SPOT&instId="+url.QueryEscape(symbol), nil, &info); err != nil {
			return s, err
		}
		if info.Code != "0" || len(info.Data) != 1 || info.Data[0].InstId != symbol || info.Data[0].State != "live" {
			return s, errors.New("unsupported/inactive OKX spot symbol")
		}
		r := info.Data[0]
		s.Tick = core.Decimal(r.TickSz)
		s.Step = core.Decimal(r.LotSz)
		s.MinQty = core.Decimal(r.MinSz)
		// OKX spot metadata specifies minimum size, not a universal quote minimum.
		s.MinNotional = "0.000000000001"
		var book struct {
			Code string
			Data []struct {
				Bids, Asks [][]string
				Ts         string
			}
		}
		if err := c.request(ctx, "GET", c.OKX+"/api/v5/market/books?instId="+url.QueryEscape(symbol)+"&sz=5", nil, &book); err != nil {
			return s, err
		}
		if book.Code != "0" || len(book.Data) != 1 || len(book.Data[0].Bids) == 0 || len(book.Data[0].Asks) == 0 || len(book.Data[0].Bids[0]) < 2 || len(book.Data[0].Asks[0]) < 2 {
			return s, errors.New("empty OKX book")
		}
		b := book.Data[0]
		s.Bid = core.Decimal(b.Bids[0][0])
		s.BidSize = core.Decimal(b.Bids[0][1])
		s.Ask = core.Decimal(b.Asks[0][0])
		s.AskSize = core.Decimal(b.Asks[0][1])
		ms, e := strconv.ParseInt(b.Ts, 10, 64)
		if e != nil {
			return s, e
		}
		s.SourceTime = time.UnixMilli(ms)
	default:
		return s, errors.New("unknown venue")
	}
	s.Observed = time.Now().UTC()
	return s, s.Validate(time.Now().UTC())
}
