package binance

// Rankings are built from Binance USDT-M futures public endpoints: 24h tickers (volume,
// price change) and open interest history. Results are cached and shared across traders
// so that several strategies do not multiply the request weight against Binance.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"vergex/internal/logger"

	"golang.org/x/sync/singleflight"
)

const (
	defaultBaseURL = "https://fapi.binance.com"

	// DefaultOIPoolSize is the number of highest-volume symbols evaluated for OI change.
	DefaultOIPoolSize = 50
	// DefaultOIWindow is the OI change window when the strategy does not set one.
	DefaultOIWindow = "4h"

	oiCacheTTL     = 15 * time.Minute
	tickerCacheTTL = time.Minute
	// listingCacheTTL bounds how long the crypto perpetual listing is reused.
	listingCacheTTL = time.Hour
	// maxConcurrentOIRequests bounds openInterestHist calls in flight to stay
	// well under Binance's per-IP request weight limit.
	maxConcurrentOIRequests = 4
	oiRequestSpacing        = 100 * time.Millisecond
)

// usdtPerpetual matches USDT-margined perpetual symbols and excludes dated
// delivery contracts such as BTCUSDT_250328.
var usdtPerpetual = regexp.MustCompile(`^[A-Z0-9]+USDT$`)

// Ticker is the 24h summary of one USDT perpetual.
type Ticker struct {
	Symbol             string
	LastPrice          float64
	PriceChangePercent float64
	QuoteVolume        float64 // 24h volume in USDT
}

// OIChange is the open interest change of one symbol over a window.
type OIChange struct {
	Ticker
	Window        string
	OIValue       float64 // latest open interest value in USDT
	OIChangePct   float64 // OI value change over the window, in percent
	OIChangeValue float64 // OI value change over the window, in USDT
}

// Client calls the Binance futures public API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a client for the Binance futures public API.
func NewClient() *Client {
	return &Client{baseURL: defaultBaseURL, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("binance %s: status %d: %s", path, resp.StatusCode, truncate(string(body), 200))
	}
	return json.Unmarshal(body, out)
}

// Tickers24h returns 24h tickers of all USDT perpetuals with non-zero volume.
func (c *Client) Tickers24h(ctx context.Context) ([]Ticker, error) {
	var raw []struct {
		Symbol             string `json:"symbol"`
		LastPrice          string `json:"lastPrice"`
		PriceChangePercent string `json:"priceChangePercent"`
		QuoteVolume        string `json:"quoteVolume"`
	}
	if err := c.getJSON(ctx, "/fapi/v1/ticker/24hr", nil, &raw); err != nil {
		return nil, err
	}
	tickers := make([]Ticker, 0, len(raw))
	for _, r := range raw {
		if !usdtPerpetual.MatchString(r.Symbol) {
			continue
		}
		volume, _ := strconv.ParseFloat(r.QuoteVolume, 64)
		if volume <= 0 {
			continue
		}
		price, _ := strconv.ParseFloat(r.LastPrice, 64)
		change, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
		tickers = append(tickers, Ticker{Symbol: r.Symbol, LastPrice: price, PriceChangePercent: change, QuoteVolume: volume})
	}
	return tickers, nil
}

// CryptoPerpetuals returns the symbols of actively trading USDT-margined
// crypto perpetuals. TradFi perpetuals (stocks, commodities, FX) use contract
// type TRADIFI_PERPETUAL and are excluded: this deployment trades crypto only.
func (c *Client) CryptoPerpetuals(ctx context.Context) (map[string]bool, error) {
	var info struct {
		Symbols []struct {
			Symbol       string `json:"symbol"`
			Status       string `json:"status"`
			ContractType string `json:"contractType"`
			QuoteAsset   string `json:"quoteAsset"`
			MarginAsset  string `json:"marginAsset"`
		} `json:"symbols"`
	}
	if err := c.getJSON(ctx, "/fapi/v1/exchangeInfo", nil, &info); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(info.Symbols))
	for _, s := range info.Symbols {
		if s.Status == "TRADING" && s.ContractType == "PERPETUAL" && s.QuoteAsset == "USDT" && s.MarginAsset == "USDT" {
			out[s.Symbol] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("binance exchangeInfo listed no crypto perpetuals")
	}
	return out, nil
}

// OpenInterestValues returns hourly open interest values (USDT), oldest first.
func (c *Client) OpenInterestValues(ctx context.Context, symbol string, points int) ([]float64, error) {
	var raw []struct {
		SumOpenInterestValue string `json:"sumOpenInterestValue"`
	}
	query := url.Values{"symbol": {symbol}, "period": {"1h"}, "limit": {strconv.Itoa(points)}}
	if err := c.getJSON(ctx, "/futures/data/openInterestHist", query, &raw); err != nil {
		return nil, err
	}
	values := make([]float64, 0, len(raw))
	for _, r := range raw {
		v, err := strconv.ParseFloat(r.SumOpenInterestValue, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid open interest value for %s: %w", symbol, err)
		}
		values = append(values, v)
	}
	return values, nil
}

// WindowHours converts "1h" | "4h" | "24h" into hours; anything else uses the default window.
func WindowHours(window string) (string, int) {
	switch window {
	case "1h":
		return "1h", 1
	case "24h":
		return "24h", 24
	default:
		return DefaultOIWindow, 4
	}
}

// Ranking computes and caches rankings shared by every trader in the process.
type Ranking struct {
	client *Client
	group  singleflight.Group

	mu        sync.Mutex
	tickers   []Ticker
	tickerAt  time.Time
	listing   map[string]bool
	listingAt time.Time
	oi        map[string]oiEntry
	now       func() time.Time
}

type oiEntry struct {
	at      time.Time
	changes []OIChange
}

// NewRanking creates a ranking service backed by client.
func NewRanking(client *Client) *Ranking {
	return &Ranking{client: client, oi: make(map[string]oiEntry), now: time.Now}
}

var (
	defaultRanking     *Ranking
	defaultRankingOnce sync.Once
)

// Default returns the process-wide ranking service.
func Default() *Ranking {
	defaultRankingOnce.Do(func() { defaultRanking = NewRanking(NewClient()) })
	return defaultRanking
}

// Tickers returns cached 24h tickers of crypto perpetuals (refreshed every minute).
func (r *Ranking) Tickers(ctx context.Context) ([]Ticker, error) {
	r.mu.Lock()
	if r.tickers != nil && r.now().Sub(r.tickerAt) < tickerCacheTTL {
		cached := r.tickers
		r.mu.Unlock()
		return cached, nil
	}
	r.mu.Unlock()

	v, err, _ := r.group.Do("tickers", func() (any, error) {
		all, err := r.client.Tickers24h(ctx)
		if err != nil {
			return nil, err
		}
		listing, err := r.cryptoListing(ctx)
		if err != nil {
			return nil, err
		}
		tickers := make([]Ticker, 0, len(all))
		for _, t := range all {
			if listing[t.Symbol] {
				tickers = append(tickers, t)
			}
		}
		r.mu.Lock()
		r.tickers, r.tickerAt = tickers, r.now()
		r.mu.Unlock()
		return tickers, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]Ticker), nil
}

// cryptoListing returns the cached crypto perpetual listing, refreshing it
// hourly. A failed refresh keeps using the previous listing when there is one.
func (r *Ranking) cryptoListing(ctx context.Context) (map[string]bool, error) {
	r.mu.Lock()
	listing, at := r.listing, r.listingAt
	r.mu.Unlock()
	if listing != nil && r.now().Sub(at) < listingCacheTTL {
		return listing, nil
	}
	fresh, err := r.client.CryptoPerpetuals(ctx)
	if err != nil {
		if listing != nil {
			logger.Warnf("⚠️ Binance listing refresh failed, using cached listing: %v", err)
			return listing, nil
		}
		return nil, err
	}
	r.mu.Lock()
	r.listing, r.listingAt = fresh, r.now()
	r.mu.Unlock()
	return fresh, nil
}

// TopVolume returns tickers sorted by 24h quote volume, highest first.
func (r *Ranking) TopVolume(ctx context.Context) ([]Ticker, error) {
	tickers, err := r.Tickers(ctx)
	if err != nil {
		return nil, err
	}
	sorted := append([]Ticker(nil), tickers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].QuoteVolume > sorted[j].QuoteVolume })
	return sorted, nil
}

// PriceMovers returns tickers sorted by 24h price change: gainers first when
// gainers is true, otherwise losers first. Only symbols with at least
// minQuoteVolume USDT of 24h volume are included.
func (r *Ranking) PriceMovers(ctx context.Context, gainers bool, minQuoteVolume float64) ([]Ticker, error) {
	tickers, err := r.Tickers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Ticker, 0, len(tickers))
	for _, t := range tickers {
		if t.QuoteVolume >= minQuoteVolume {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if gainers {
			return out[i].PriceChangePercent > out[j].PriceChangePercent
		}
		return out[i].PriceChangePercent < out[j].PriceChangePercent
	})
	return out, nil
}

// OIChanges returns the OI change of the poolSize highest-volume symbols over
// window, sorted by change percent (highest first). Results are cached for 15
// minutes per (window, poolSize).
func (r *Ranking) OIChanges(ctx context.Context, window string, poolSize int) ([]OIChange, error) {
	window, hours := WindowHours(window)
	if poolSize <= 0 {
		poolSize = DefaultOIPoolSize
	}
	key := fmt.Sprintf("%s/%d", window, poolSize)

	r.mu.Lock()
	if entry, ok := r.oi[key]; ok && r.now().Sub(entry.at) < oiCacheTTL {
		r.mu.Unlock()
		return entry.changes, nil
	}
	r.mu.Unlock()

	v, err, _ := r.group.Do("oi/"+key, func() (any, error) {
		pool, err := r.TopVolume(ctx)
		if err != nil {
			return nil, err
		}
		if len(pool) > poolSize {
			pool = pool[:poolSize]
		}
		changes := r.computeOIChanges(ctx, pool, window, hours)
		if len(changes) == 0 {
			return nil, fmt.Errorf("no open interest history available")
		}
		r.mu.Lock()
		r.oi[key] = oiEntry{at: r.now(), changes: changes}
		r.mu.Unlock()
		return changes, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]OIChange), nil
}

func (r *Ranking) computeOIChanges(ctx context.Context, pool []Ticker, window string, hours int) []OIChange {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		changes = make([]OIChange, 0, len(pool))
		sem     = make(chan struct{}, maxConcurrentOIRequests)
		failed  int
	)
	for _, ticker := range pool {
		select {
		case <-ctx.Done():
			wg.Wait()
			return changes
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(t Ticker) {
			defer wg.Done()
			defer func() { <-sem }()
			values, err := r.client.OpenInterestValues(ctx, t.Symbol, hours+1)
			time.Sleep(oiRequestSpacing)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || len(values) < 2 || values[0] <= 0 {
				failed++
				return
			}
			first, last := values[0], values[len(values)-1]
			changes = append(changes, OIChange{
				Ticker:        t,
				Window:        window,
				OIValue:       last,
				OIChangePct:   (last - first) / first * 100,
				OIChangeValue: last - first,
			})
		}(ticker)
	}
	wg.Wait()
	if failed > 0 {
		logger.Warnf("⚠️ Open interest history unavailable for %d of %d symbols", failed, len(pool))
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].OIChangePct > changes[j].OIChangePct })
	return changes
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
