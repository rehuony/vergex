package market

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/service"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
)

// Public reads what a venue publishes with its numbers exact, one request at a time for
// someone planning a trade. Candles for calculations come from the streams and local
// history instead: see LiveSource.
type Public struct {
	Binance *binance.PublicClient
	OKX     *okx.PublicClient
}

// publicObserver hears how Binance answers public reads, which share the IP's allowance.
var publicObserver atomic.Pointer[func(status int, header http.Header)]

// ObservePublicResponses tells a rate limiter about Binance's answers to public reads. It
// only listens: someone planning a trade waits for no budget, while background requests
// get to back off when these use up the allowance or Binance objects.
func ObservePublicResponses(observe func(status int, header http.Header)) {
	publicObserver.Store(&observe)
}

// observedTransport passes each response to the observer on its way back.
type observedTransport struct{ next http.RoundTripper }

func (t observedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(request)
	if err == nil {
		if observe := publicObserver.Load(); observe != nil {
			(*observe)(response.StatusCode, response.Header)
		}
	}
	return response, err
}

// NewPublic reads from the venues' own hosts.
func NewPublic() *Public {
	binanceClient := &http.Client{Timeout: 10 * time.Second, Transport: observedTransport{service.VenueTransport}}
	client := &http.Client{Timeout: 10 * time.Second, Transport: service.VenueTransport}
	return &Public{Binance: binance.NewPublicClient(binanceClient), OKX: okx.NewPublicClient(client)}
}

// NewTestnetPublic reads from the venues' test environments.
func NewTestnetPublic() *Public {
	client := &http.Client{Timeout: 10 * time.Second, Transport: service.VenueTransport}
	return &Public{Binance: binance.NewTestnetPublicClient(client), OKX: okx.NewTestnetPublicClient(client)}
}

// PerpetualBook reads a perpetual's book on a venue.
func (p *Public) PerpetualBook(ctx context.Context, venue, symbol string) (instrument.Book, error) {
	switch venue {
	case "binance":
		return p.Binance.PerpetualBook(ctx, symbol)
	case "okx":
		return p.OKX.PerpetualBook(ctx, symbol)
	}
	return instrument.Book{}, errors.New("unsupported market kind")
}

// MarkPrice reads a perpetual's mark price on a venue.
func (p *Public) MarkPrice(ctx context.Context, venue, symbol string) (string, error) {
	switch venue {
	case "binance":
		return p.Binance.MarkPrice(ctx, symbol)
	case "okx":
		return p.OKX.MarkPrice(ctx, symbol)
	}
	return "", errors.New("unsupported market kind")
}

// ExactCandles reads a perpetual's candles on a venue, the newest before toSeconds when
// it is set. intervalSeconds is how long interval is.
func (p *Public) ExactCandles(ctx context.Context, venue, symbol, interval string, intervalSeconds, toSeconds int64, limit int) ([]instrument.ExactCandle, error) {
	switch venue {
	case "binance":
		return p.Binance.ExactCandles(ctx, symbol, interval, toSeconds, limit)
	case "okx":
		return p.OKX.ExactCandles(ctx, symbol, interval, intervalSeconds, toSeconds, limit)
	}
	return nil, nil
}

// Listings reads what a venue offers.
func (p *Public) Listings(ctx context.Context, venue string) (instrument.Listings, error) {
	if venue == "binance" {
		return p.Binance.Listings(ctx)
	}
	return p.OKX.Listings(ctx)
}
