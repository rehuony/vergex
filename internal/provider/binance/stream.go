package binance

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"vergex/internal/provider/wsfeed"
)

// Venue names this exchange in live data.
const Venue = "binance"

const (
	// marketStreamURL is the route that serves candles and prices. A connection
	// without a route receives nothing from it.
	marketStreamURL = "wss://fstream.binance.com/market/stream"

	// AllPricesTopic is every symbol's last price once a second. It costs less than
	// one busy symbol's candles, and its steady beat shows the connection is alive.
	AllPricesTopic = "!miniTicker@arr"

	// Binance allows 1024 streams on a connection and 10 incoming messages a second,
	// and closes a connection after 24 hours.
	maxStreamTopics = 800
	streamBatch     = 200
	streamControl   = 250 * time.Millisecond
	streamMaxAge    = 23 * time.Hour
	streamSilence   = 15 * time.Second
)

// KlineTopic is a symbol's candle stream.
func KlineTopic(symbol, interval string) string {
	return strings.ToLower(symbol) + "@kline_" + interval
}

// MarkTopic is a symbol's mark price, once a second.
func MarkTopic(symbol string) string {
	return strings.ToLower(symbol) + "@markPrice@1s"
}

// NewStream creates a feed of Binance futures market data. An empty url means the
// real one.
func NewStream(url string) *wsfeed.Feed {
	if url == "" {
		url = marketStreamURL
	}
	return wsfeed.New(wsfeed.Config{
		Venue: Venue, URL: url, Protocol: &streamProtocol{}, MaxTopics: maxStreamTopics, BatchTopics: streamBatch,
		ControlEvery: streamControl, Silence: streamSilence, MaxAge: streamMaxAge,
	})
}

type streamProtocol struct {
	nextID atomic.Int64
}

func (p *streamProtocol) Control(topics []string, subscribe bool) []byte {
	method := "UNSUBSCRIBE"
	if subscribe {
		method = "SUBSCRIBE"
	}
	raw, _ := json.Marshal(struct {
		Method string   `json:"method"`
		Params []string `json:"params"`
		ID     int64    `json:"id"`
	}{method, topics, p.nextID.Add(1)})
	return raw
}

// Field names below repeat in both cases on purpose: encoding/json falls back to a
// case-insensitive match, so a lone "q" would also take the value of "Q".
type streamKline struct {
	OpenTime       int64  `json:"t"`
	CloseTime      int64  `json:"T"`
	Symbol         string `json:"s"`
	Interval       string `json:"i"`
	Open           string `json:"o"`
	Close          string `json:"c"`
	High           string `json:"h"`
	Low            string `json:"l"`
	LastTrade      int64  `json:"L"`
	Volume         string `json:"v"`
	TakerBuyVolume string `json:"V"`
	QuoteVolume    string `json:"q"`
	TakerBuyQuote  string `json:"Q"`
	Trades         int64  `json:"n"`
	Closed         bool   `json:"x"`
}

type streamTick struct {
	Type       string `json:"e"`
	Time       int64  `json:"E"`
	Symbol     string `json:"s"`
	Last       string `json:"c"` // Mini ticker.
	Mark       string `json:"p"` // Mark price.
	IndexPrice string `json:"P"`
}

func (p *streamProtocol) Parse(raw []byte) ([]wsfeed.Event, error) {
	var message struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil, err
	}
	if len(message.Error) > 0 {
		return nil, errors.New("binance stream error: " + string(message.Error))
	}
	switch {
	case message.Stream == "":
		return nil, nil // An acknowledgement.
	case message.Stream == AllPricesTopic:
		var rows []streamTick
		if err := json.Unmarshal(message.Data, &rows); err != nil {
			return nil, err
		}
		ticks := make([]wsfeed.Tick, 0, len(rows))
		for _, row := range rows {
			if price, err := strconv.ParseFloat(row.Last, 64); err == nil && price > 0 {
				ticks = append(ticks, wsfeed.Tick{Symbol: row.Symbol, Price: price, Time: row.Time})
			}
		}
		return []wsfeed.Event{{Prices: ticks}}, nil
	case strings.Contains(message.Stream, "@kline_"):
		candle, err := parseStreamKline(message.Data)
		if err != nil {
			return nil, err
		}
		return []wsfeed.Event{{Candle: candle}}, nil
	case strings.Contains(message.Stream, "@markPrice"):
		var row streamTick
		if err := json.Unmarshal(message.Data, &row); err != nil {
			return nil, err
		}
		price, err := strconv.ParseFloat(row.Mark, 64)
		if err != nil || price <= 0 {
			return nil, errors.New("binance stream: invalid mark price")
		}
		return []wsfeed.Event{{Marks: []wsfeed.Tick{{Symbol: row.Symbol, Price: price, Time: row.Time}}}}, nil
	}
	return nil, nil
}

func parseStreamKline(data json.RawMessage) (*wsfeed.Candle, error) {
	var message struct {
		Kline streamKline `json:"k"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, err
	}
	k := message.Kline
	candle := &wsfeed.Candle{Symbol: k.Symbol, Interval: k.Interval, OpenTime: k.OpenTime, Trades: k.Trades, Closed: k.Closed}
	for _, field := range []struct {
		text string
		dest *float64
	}{
		{k.Open, &candle.Open}, {k.High, &candle.High}, {k.Low, &candle.Low}, {k.Close, &candle.Close},
		{k.Volume, &candle.Volume}, {k.QuoteVolume, &candle.QuoteVolume},
		{k.TakerBuyVolume, &candle.TakerBuyVolume}, {k.TakerBuyQuote, &candle.TakerBuyQuote},
	} {
		value, err := strconv.ParseFloat(field.text, 64)
		if err != nil {
			return nil, errors.New("binance stream: invalid kline")
		}
		*field.dest = value
	}
	return candle, nil
}
