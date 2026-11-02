package binance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"vergex/internal/market/instrument"
)

// PublicClient reads Binance's public REST data with exact numbers: books, listings,
// candles and single derivatives figures, for one request at a time from a person
// looking at a chart. It holds no keys.
type PublicClient struct {
	HTTP          *http.Client
	Spot, Futures string
}

// NewPublicClient creates a client for Binance's own hosts.
func NewPublicClient(client *http.Client) *PublicClient {
	return &PublicClient{HTTP: client, Spot: "https://data-api.binance.vision", Futures: "https://fapi.binance.com"}
}

// NewTestnetPublicClient creates a client for Binance's test environment.
func NewTestnetPublicClient(client *http.Client) *PublicClient {
	public := NewPublicClient(client)
	public.Futures = testnetBaseURL
	return public
}

func (c *PublicClient) request(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, bytes.NewReader(nil))
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
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// PerpetualBook reads a USDT perpetual's rules and order book.
func (c *PublicClient) PerpetualBook(ctx context.Context, symbol string) (instrument.Book, error) {
	var s instrument.Book
	var info struct {
		Symbols []struct {
			Symbol, Status, ContractType, MarginAsset, QuoteAsset, BaseAsset string
			Filters                                                          []struct{ FilterType, TickSize, StepSize, MinQty, MinNotional, Notional string }
		}
	}
	if err := c.request(ctx, c.Futures+"/fapi/v1/exchangeInfo", &info); err != nil {
		return s, err
	}
	found := false
	for _, r := range info.Symbols {
		if r.Symbol != symbol {
			continue
		}
		if r.Status != "TRADING" || r.ContractType != "PERPETUAL" || r.MarginAsset != "USDT" || r.QuoteAsset != "USDT" {
			return s, errors.New("Only actively trading Binance USDT crypto perpetual contracts are supported")
		}
		found = true
		s.BaseAsset = r.BaseAsset
		s.QuoteAsset = r.QuoteAsset
		for _, f := range r.Filters {
			switch f.FilterType {
			case "PRICE_FILTER":
				s.Tick = f.TickSize
			case "LOT_SIZE":
				s.Step = f.StepSize
				s.MinQty = f.MinQty
			case "MIN_NOTIONAL", "NOTIONAL":
				v := f.Notional
				if v == "" {
					v = f.MinNotional
				}
				s.MinNotional = v
			}
		}
	}
	if !found {
		return s, errors.New("Symbol is not listed as a Binance USDT perpetual; no spot fallback was used")
	}
	var b struct {
		Bids, Asks [][]string
		T, E       int64
	}
	if err := c.request(ctx, c.Futures+"/fapi/v1/depth?symbol="+url.QueryEscape(symbol)+"&limit=100", &b); err != nil {
		return s, err
	}
	if len(b.Bids) == 0 || len(b.Asks) == 0 || len(b.Bids[0]) < 2 || len(b.Asks[0]) < 2 {
		return s, errors.New("empty Binance perpetual book")
	}
	s.Bid = b.Bids[0][0]
	s.BidSize = b.Bids[0][1]
	s.Ask = b.Asks[0][0]
	s.AskSize = b.Asks[0][1]
	for _, side := range []struct {
		raw [][]string
		dst *[]instrument.BookLevel
	}{{b.Bids, &s.Bids}, {b.Asks, &s.Asks}} {
		for _, level := range side.raw {
			if len(level) < 2 {
				return s, errors.New("invalid Binance perpetual book level")
			}
			*side.dst = append(*side.dst, instrument.BookLevel{Price: level[0], Quantity: level[1]})
		}
	}
	if b.T == 0 {
		b.T = b.E
	}
	if b.T <= 0 {
		return s, errors.New("missing Binance futures source timestamp")
	}
	s.SourceTime = time.UnixMilli(b.T).UTC()
	return s, nil
}

// MarkPrice reads a USDT perpetual's mark price.
func (c *PublicClient) MarkPrice(ctx context.Context, symbol string) (string, error) {
	reading, err := c.Funding(ctx, symbol)
	return reading.Mark, err
}
