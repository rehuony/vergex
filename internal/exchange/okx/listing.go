package okx

// The listing is OKX public market metadata. It is used to keep coin rankings built from
// Binance data to symbols that an OKX trader can trade.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"vergex/internal/logger"
)

const (
	defaultBaseURL  = "https://www.okx.com"
	listingCacheTTL = time.Hour
)

// Listing caches the live USDT-margined linear swaps on OKX, keyed by the
// generic symbol the traders use (BTC-USDT-SWAP -> BTCUSDT).
type Listing struct {
	baseURL string
	http    *http.Client
	now     func() time.Time

	mu      sync.Mutex
	symbols map[string]bool
	at      time.Time
}

// NewListing creates a listing reader for the OKX public API.
func NewListing() *Listing {
	return &Listing{baseURL: defaultBaseURL, http: &http.Client{Timeout: 20 * time.Second}, now: time.Now}
}

var (
	defaultListing     *Listing
	defaultListingOnce sync.Once
)

// Default returns the process-wide OKX listing.
func Default() *Listing {
	defaultListingOnce.Do(func() { defaultListing = NewListing() })
	return defaultListing
}

// USDTSwaps returns the generic symbols of live OKX USDT linear swaps. The
// result is cached for an hour; when a refresh fails the previous listing is
// reused.
func (l *Listing) USDTSwaps(ctx context.Context) (map[string]bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.symbols != nil && l.now().Sub(l.at) < listingCacheTTL {
		return l.symbols, nil
	}
	fresh, err := l.fetch(ctx)
	if err != nil {
		if l.symbols != nil {
			logger.Warnf("⚠️ OKX listing refresh failed, using cached listing: %v", err)
			return l.symbols, nil
		}
		return nil, err
	}
	l.symbols, l.at = fresh, l.now()
	return fresh, nil
}

func (l *Listing) fetch(ctx context.Context) (map[string]bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.baseURL+"/api/v5/public/instruments?instType=SWAP", nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okx instruments: status %d", resp.StatusCode)
	}
	var info struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID    string `json:"instId"`
			CtType    string `json:"ctType"`
			SettleCcy string `json:"settleCcy"`
			State     string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	if info.Code != "0" {
		return nil, fmt.Errorf("okx instruments: code %s: %s", info.Code, info.Msg)
	}
	out := make(map[string]bool, len(info.Data))
	for _, d := range info.Data {
		base, ok := strings.CutSuffix(d.InstID, "-USDT-SWAP")
		if !ok || d.CtType != "linear" || d.SettleCcy != "USDT" || d.State != "live" || base == "" {
			continue
		}
		out[base+"USDT"] = true
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("okx instruments listed no live USDT swaps")
	}
	return out, nil
}
