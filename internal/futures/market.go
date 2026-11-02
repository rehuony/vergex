package futures

import (
	"context"
	"errors"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"vergex/internal/futures/plan"
	"vergex/internal/futures/plugins"
	"vergex/internal/market"
	"vergex/internal/market/instrument"

	"golang.org/x/sync/singleflight"
)

// Market is what Futures reads from a venue's public data, in its own terms: books as
// plan snapshots, checked for planning, and exact candles for its chart.
type Market struct {
	Public          *market.Public
	discoveryMu     sync.Mutex
	discoveryCache  map[string]MarketList
	discoveryFlight singleflight.Group
}

// NewMarket reads from the venues' own hosts.
func NewMarket() *Market { return &Market{Public: market.NewPublic()} }

func snapshotOf(venue, symbol, kind string, book instrument.Book) plan.Snapshot {
	levels := func(rows []instrument.BookLevel) []plan.BookLevel {
		var out []plan.BookLevel
		for _, row := range rows {
			out = append(out, plan.BookLevel{Price: plan.Decimal(row.Price), Quantity: plan.Decimal(row.Quantity)})
		}
		return out
	}
	return plan.Snapshot{
		Venue: venue, Symbol: symbol, Kind: kind,
		BaseAsset: book.BaseAsset, QuoteAsset: book.QuoteAsset, ContractValue: plan.Decimal(book.ContractValue),
		Bid: plan.Decimal(book.Bid), Ask: plan.Decimal(book.Ask), BidSize: plan.Decimal(book.BidSize), AskSize: plan.Decimal(book.AskSize),
		Bids: levels(book.Bids), Asks: levels(book.Asks),
		Tick: plan.Decimal(book.Tick), Step: plan.Decimal(book.Step), MinQty: plan.Decimal(book.MinQty), MinNotional: plan.Decimal(book.MinNotional),
		SourceTime: book.SourceTime,
	}
}

// Snapshot reads a perpetual's book, checked for planning.
func (m *Market) Snapshot(ctx context.Context, venue, symbol string) (plan.Snapshot, error) {
	if !instrument.ValidSymbol(symbol) {
		return plan.Snapshot{}, errors.New("invalid native symbol")
	}
	if venue != "binance" && venue != "okx" {
		return plan.Snapshot{}, errors.New("unsupported venue")
	}
	book, err := m.Public.PerpetualBook(ctx, venue, symbol)
	s := snapshotOf(venue, symbol, "perpetual", book)
	if err != nil {
		return s, err
	}
	s.Observed = time.Now().UTC()
	return s, s.Validate(time.Now().UTC())
}

// Candles are chart observations, never executable quotes or backtest results.
type Candle struct {
	Time   int64        `json:"time"`
	Open   plan.Decimal `json:"open"`
	High   plan.Decimal `json:"high"`
	Low    plan.Decimal `json:"low"`
	Close  plan.Decimal `json:"close"`
	Volume plan.Decimal `json:"volume"`
}

// CandleQuery says which candles a chart wants.
type CandleQuery struct {
	Venue    string `json:"venue"`
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	From     int64  `json:"from,omitempty"`
	To       int64  `json:"to,omitempty"`
	Limit    int    `json:"limit"`
}

// ChartSource supplies candles for a chart.
type ChartSource interface {
	Candles(context.Context, CandleQuery) ([]Candle, error)
}

// IntervalSeconds is how long a chart interval is; 0 for one a chart cannot show.
func IntervalSeconds(interval string) int64 {
	return map[string]int64{"1m": 60, "5m": 300, "15m": 900, "1h": 3600, "4h": 14400, "1d": 86400}[interval]
}

// Validate refuses a query outside what a chart can ask for.
func (q CandleQuery) Validate() error {
	if (q.Venue != "binance" && q.Venue != "okx") || !instrument.ValidSymbol(q.Symbol) || IntervalSeconds(q.Interval) == 0 {
		return errors.New("invalid chart venue, symbol or interval")
	}
	if q.Limit < 1 || q.Limit > 300 || q.From < 0 || q.To < 0 || q.To > time.Now().Add(time.Minute).Unix() || (q.From > 0 && q.To > 0 && q.From >= q.To) {
		return errors.New("invalid candle time range or limit (1–300)")
	}
	return nil
}

// NormalizeCandles sorts candles and refuses any that could not be real.
func NormalizeCandles(rows []Candle, now time.Time) ([]Candle, error) {
	if len(rows) > 1000 {
		return nil, errors.New("too many venue candles")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	for i, row := range rows {
		if row.Time <= 0 || row.Time > now.Add(30*time.Second).Unix() || (i > 0 && rows[i-1].Time == row.Time) {
			return nil, errors.New("invalid or duplicate candle timestamp")
		}
		for _, v := range []plan.Decimal{row.Open, row.High, row.Low, row.Close} {
			if !plan.Positive(v) {
				return nil, errors.New("invalid candle price")
			}
		}
		o, _ := new(big.Rat).SetString(string(row.Open))
		h, _ := new(big.Rat).SetString(string(row.High))
		l, _ := new(big.Rat).SetString(string(row.Low))
		c, _ := new(big.Rat).SetString(string(row.Close))
		if h.Cmp(l) < 0 || h.Cmp(o) < 0 || h.Cmp(c) < 0 || l.Cmp(o) > 0 || l.Cmp(c) > 0 {
			return nil, errors.New("invalid candle OHLC bounds")
		}
		if len(row.Volume) > 40 || (!plan.Positive(row.Volume) && !zeroVolume.MatchString(string(row.Volume))) {
			return nil, errors.New("invalid candle volume")
		}
	}
	return rows, nil
}

var zeroVolume = regexp.MustCompile(`^0+(\.0{1,12})?$`)

// Candles reads a chart's candles from the venue.
func (m *Market) Candles(ctx context.Context, q CandleQuery) ([]Candle, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	exact, err := m.Public.ExactCandles(ctx, q.Venue, q.Symbol, q.Interval, IntervalSeconds(q.Interval), q.To, q.Limit)
	if err != nil {
		return nil, err
	}
	var rows []Candle
	for _, r := range exact {
		rows = append(rows, Candle{r.Time, plan.Decimal(r.Open), plan.Decimal(r.High), plan.Decimal(r.Low), plan.Decimal(r.Close), plan.Decimal(r.Volume)})
	}
	rows, err = NormalizeCandles(rows, time.Now())
	if err != nil {
		return nil, err
	}
	var out []Candle
	for _, r := range rows {
		if q.To == 0 || r.Time < q.To {
			out = append(out, r)
		}
	}
	if len(out) > q.Limit {
		out = out[len(out)-q.Limit:]
	}
	// From is advisory: older bars can satisfy TradingView's countBack request.
	return out, nil
}

// MarketSymbol is a contract someone can plan on.
type MarketSymbol struct {
	Symbol             string  `json:"symbol"`
	Base               string  `json:"base"`
	Category           string  `json:"category"`
	Dex                string  `json:"dex,omitempty"`
	Volume24H          float64 `json:"volume_24h"`
	Price              string  `json:"price"`
	ExecutionSupported bool    `json:"execution_supported"`
}

// MarketList is what a venue offers, highest volume first.
type MarketList struct {
	Venue     string         `json:"venue"`
	Markets   []MarketSymbol `json:"markets"`
	FetchedAt time.Time      `json:"fetched_at"`
	Sources   []string       `json:"sources"`
	Warnings  []string       `json:"warnings,omitempty"`
}

// Markets only reads public exchange metadata and 24h notional volume. It cannot use fee
// wallets, submit orders, or substitute one venue for another.
func (m *Market) Markets(ctx context.Context, venue string) (MarketList, error) {
	if venue != "binance" && venue != "okx" {
		return MarketList{}, errors.New("Unsupported discovery venue")
	}
	result, err, _ := m.discoveryFlight.Do(venue, func() (any, error) {
		m.discoveryMu.Lock()
		cached, exists := m.discoveryCache[venue]
		m.discoveryMu.Unlock()
		if exists && time.Since(cached.FetchedAt) < time.Minute {
			return cached, nil
		}
		deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out := MarketList{Venue: venue, Markets: []MarketSymbol{}, FetchedAt: time.Now().UTC()}
		listings, err := m.Public.Listings(deadline, venue)
		if err != nil {
			return MarketList{}, err
		}
		for _, l := range listings.Markets {
			out.Markets = append(out.Markets, MarketSymbol{Symbol: l.Symbol, Base: l.Base, Category: "crypto", Volume24H: l.Volume24H, Price: l.Price, ExecutionSupported: true})
		}
		out.Sources, out.Warnings = listings.Sources, listings.Warnings
		sort.Slice(out.Markets, func(i, j int) bool {
			if out.Markets[i].Volume24H == out.Markets[j].Volume24H {
				return out.Markets[i].Symbol < out.Markets[j].Symbol
			}
			return out.Markets[i].Volume24H > out.Markets[j].Volume24H
		})
		m.discoveryMu.Lock()
		if m.discoveryCache == nil {
			m.discoveryCache = map[string]MarketList{}
		}
		m.discoveryCache[venue] = out
		m.discoveryMu.Unlock()
		return out, nil
	})
	if err != nil {
		return MarketList{}, err
	}
	return result.(MarketList), nil
}

// PluginData reads a public data plugin's figures from the venue.
func (m *Market) PluginData(ctx context.Context, id string, q plugins.Request) (plugins.Result, error) {
	out := plugins.Result{Venue: q.Venue, Symbol: q.Symbol, Observed: time.Now().UTC(), Mode: "public", Notice: "Read-only snapshot, not a direction forecast or fill evidence. Funding is excluded from the current risk estimate. One OI snapshot does not measure OI change."}
	if id != "funding" && id != "open_interest" {
		return out, errors.New("unknown public data plugin")
	}
	var reading instrument.Reading
	var err error
	switch q.Venue {
	case "binance":
		if !strings.HasSuffix(q.Symbol, "USDT") {
			return out, errors.New("This plugin supports Binance USDT futures only")
		}
		if id == "funding" {
			reading, err = m.Public.Binance.Funding(ctx, q.Symbol)
		} else {
			reading, err = m.Public.Binance.OpenInterest(ctx, q.Symbol)
		}
		out.Source = reading.Source
		if err != nil {
			return out, err
		}
		if id == "funding" {
			out.Metrics = []plugins.Metric{{Key: "funding_rate", Label: "Latest funding rate", Value: reading.Value, Unit: "fraction"}, {Key: "mark_price", Label: "Mark price", Value: reading.Mark, Unit: "USDT"}}
		} else {
			out.Metrics = []plugins.Metric{{Key: "open_interest", Label: "Open interest", Value: reading.Value, Unit: strings.TrimSuffix(q.Symbol, "USDT")}}
		}
	case "okx":
		if !strings.HasSuffix(q.Symbol, "-USDT-SWAP") {
			return out, errors.New("This plugin supports OKX USDT perpetuals only")
		}
		if id == "funding" {
			reading, err = m.Public.OKX.Funding(ctx, q.Symbol)
		} else {
			reading, err = m.Public.OKX.OpenInterest(ctx, q.Symbol)
		}
		out.Source = reading.Source
		if err != nil {
			return out, err
		}
		if id == "funding" {
			out.Metrics = []plugins.Metric{{Key: "funding_rate", Label: "Current funding rate", Value: reading.Value, Unit: "fraction"}}
		} else {
			out.Metrics = []plugins.Metric{{Key: "open_interest", Label: "Open contracts", Value: reading.Value, Unit: "contracts"}}
		}
	default:
		return out, errors.New("unsupported plugin venue")
	}
	out.SourceTime = reading.At
	out.Observed = time.Now().UTC()
	return out, nil
}
