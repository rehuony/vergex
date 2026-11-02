package okx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vergex/internal/account"
	"vergex/internal/market/instrument"
)

// PublicClient reads OKX's public REST data with exact numbers: books, listings, candles
// and single derivatives figures, for one request at a time from a person looking at a
// chart. It holds no keys.
type PublicClient struct {
	HTTP *http.Client
	Base string
	// Testnet reads demo trading's data, which OKX serves from the same host.
	Testnet bool
}

// NewPublicClient creates a client for OKX's own host.
func NewPublicClient(client *http.Client) *PublicClient {
	return &PublicClient{HTTP: client, Base: okxBaseURL}
}

// NewTestnetPublicClient creates a client for OKX's demo trading.
func NewTestnetPublicClient(client *http.Client) *PublicClient {
	return &PublicClient{HTTP: client, Base: okxBaseURL, Testnet: true}
}

func (c *PublicClient) request(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	if c.Testnet {
		req.Header.Set(simulatedTradingHeader, simulatedTradingValue(true))
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

// productDecimal rejects silently rounded contract multipliers/quantity steps.
func productDecimal(a, b string) (string, error) {
	x, ok := new(big.Rat).SetString(a)
	if !ok || x.Sign() <= 0 {
		return "", errors.New("invalid contract quantity")
	}
	y, ok := new(big.Rat).SetString(b)
	if !ok || y.Sign() <= 0 {
		return "", errors.New("invalid contract value")
	}
	z := new(big.Rat).Mul(x, y)
	v := z.FloatString(12)
	r, _ := new(big.Rat).SetString(v)
	if !account.Positive(account.Decimal(v)) || r.Cmp(z) != 0 {
		return "", errors.New("unsupported contract quantity precision")
	}
	return v, nil
}

// PerpetualBook reads a USDT linear perpetual's rules and order book, with quantities
// converted from contracts to the base asset.
func (c *PublicClient) PerpetualBook(ctx context.Context, symbol string) (instrument.Book, error) {
	var s instrument.Book
	if !strings.HasSuffix(symbol, "-USDT-SWAP") {
		return s, errors.New("Use an OKX USDT linear perpetual symbol such as BTC-USDT-SWAP")
	}
	var info struct {
		Code string
		Data []struct{ InstId, State, InstType, CtType, CtVal, CtValCcy, CtMult, SettleCcy, TickSz, LotSz, MinSz string }
	}
	if err := c.request(ctx, c.Base+"/api/v5/public/instruments?instType=SWAP&instId="+url.QueryEscape(symbol), &info); err != nil {
		return s, err
	}
	if info.Code != "0" || len(info.Data) != 1 {
		return s, errors.New("invalid OKX perpetual instrument metadata")
	}
	r := info.Data[0]
	if r.InstId != symbol || r.State != "live" || r.InstType != "SWAP" || r.CtType != "linear" || r.SettleCcy != "USDT" || r.CtValCcy != strings.TrimSuffix(symbol, "-USDT-SWAP") {
		return s, errors.New("unsupported OKX contract; inverse/non-USDT/inactive contracts rejected")
	}
	// ctVal defines base units per contract. ctMult is not silently ignored.
	if r.CtMult != "" && r.CtMult != "1" && r.CtMult != "1.0" {
		return s, errors.New("non-unit OKX ctMult unsupported")
	}
	var err error
	s.ContractValue = r.CtVal
	s.BaseAsset = r.CtValCcy
	s.QuoteAsset = r.SettleCcy
	s.Tick = r.TickSz
	if s.Step, err = productDecimal(r.LotSz, r.CtVal); err != nil {
		return s, err
	}
	if s.MinQty, err = productDecimal(r.MinSz, r.CtVal); err != nil {
		return s, err
	}
	s.MinNotional = "0.000000000001"
	var book struct {
		Code string
		Data []struct {
			Bids, Asks [][]string
			Ts         string
		}
	}
	if err = c.request(ctx, c.Base+"/api/v5/market/books?instId="+url.QueryEscape(symbol)+"&sz=100", &book); err != nil {
		return s, err
	}
	if book.Code != "0" || len(book.Data) != 1 || len(book.Data[0].Bids) == 0 || len(book.Data[0].Asks) == 0 || len(book.Data[0].Bids[0]) < 2 || len(book.Data[0].Asks[0]) < 2 {
		return s, errors.New("empty OKX perpetual book")
	}
	b := book.Data[0]
	s.Bid = b.Bids[0][0]
	s.Ask = b.Asks[0][0]
	if s.BidSize, err = productDecimal(b.Bids[0][1], r.CtVal); err != nil {
		return s, err
	}
	if s.AskSize, err = productDecimal(b.Asks[0][1], r.CtVal); err != nil {
		return s, err
	}
	for _, side := range []struct {
		raw [][]string
		dst *[]instrument.BookLevel
	}{{b.Bids, &s.Bids}, {b.Asks, &s.Asks}} {
		for _, level := range side.raw {
			if len(level) < 2 {
				return s, errors.New("invalid OKX book level")
			}
			qty, e := productDecimal(level[1], r.CtVal)
			if e != nil {
				return s, e
			}
			*side.dst = append(*side.dst, instrument.BookLevel{Price: level[0], Quantity: qty})
		}
	}
	var ms int64
	if _, err = fmt.Sscan(b.Ts, &ms); err != nil || ms <= 0 {
		return s, errors.New("invalid OKX book timestamp")
	}
	s.SourceTime = time.UnixMilli(ms).UTC()
	return s, nil
}

// MarkPrice reads a USDT perpetual's mark price.
func (c *PublicClient) MarkPrice(ctx context.Context, symbol string) (string, error) {
	var r struct {
		Code string `json:"code"`
		Data []struct {
			Mark string `json:"markPx"`
		} `json:"data"`
	}
	path := "/api/v5/public/mark-price?instType=SWAP&instId=" + url.QueryEscape(InstrumentID(symbol))
	if err := c.request(ctx, c.Base+path, &r); err != nil {
		return "", err
	}
	if r.Code != "0" || len(r.Data) != 1 {
		return "", errors.New("invalid OKX mark price response")
	}
	return r.Data[0].Mark, nil
}
