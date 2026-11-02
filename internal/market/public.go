package market

import (
	"context"
	"errors"
	"net/http"
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

// NewPublic reads from the venues' own hosts.
func NewPublic() *Public {
	client := &http.Client{Timeout: 10 * time.Second, Transport: service.VenueTransport}
	return &Public{Binance: binance.NewPublicClient(client), OKX: okx.NewPublicClient(client)}
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
