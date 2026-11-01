package okx

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vergex/internal/market/wsfeed"
)

// Venue names this exchange in live data.
const Venue = "okx"

const (
	// Candles and mark prices are served from different addresses.
	candleStreamURL = "wss://ws.okx.com/ws/v5/business"
	markStreamURL   = "wss://ws.okx.com/ws/v5/public"

	// OKX allows 480 subscribe messages per connection per hour, and closes a
	// connection that sends nothing for 30 seconds.
	maxStreamTopics = 400
	streamBatch     = 50
	streamControl   = time.Second
	streamPingEvery = 20 * time.Second
	streamSilence   = 45 * time.Second
)

// CandleTopic is an instrument's candle channel, or "" for an interval OKX lacks.
func CandleTopic(symbol, interval string) string {
	bar, ok := barNames[interval]
	if !ok {
		return ""
	}
	return "candle" + bar + ":" + InstrumentID(symbol)
}

// MarkTopic is an instrument's mark price channel.
func MarkTopic(symbol string) string { return "mark-price:" + InstrumentID(symbol) }

// NewCandleStream creates a feed of OKX candles. An empty url means the real one.
func NewCandleStream(url string) *wsfeed.Feed {
	if url == "" {
		url = candleStreamURL
	}
	return newStream(url)
}

// NewMarkStream creates a feed of OKX mark prices. An empty url means the real one.
func NewMarkStream(url string) *wsfeed.Feed {
	if url == "" {
		url = markStreamURL
	}
	return newStream(url)
}

func newStream(url string) *wsfeed.Feed {
	return wsfeed.New(wsfeed.Config{
		Venue: Venue, URL: url, Protocol: streamProtocol{}, MaxTopics: maxStreamTopics, BatchTopics: streamBatch,
		ControlEvery: streamControl, Silence: streamSilence, Ping: []byte("ping"), PingEvery: streamPingEvery,
	})
}

type streamProtocol struct{}

type streamArg struct {
	Channel string `json:"channel"`
	InstID  string `json:"instId"`
}

func (streamProtocol) Control(topics []string, subscribe bool) []byte {
	op := "unsubscribe"
	if subscribe {
		op = "subscribe"
	}
	args := make([]streamArg, 0, len(topics))
	for _, topic := range topics {
		channel, inst, _ := strings.Cut(topic, ":")
		args = append(args, streamArg{channel, inst})
	}
	raw, _ := json.Marshal(struct {
		Op   string      `json:"op"`
		Args []streamArg `json:"args"`
	}{op, args})
	return raw
}

func (streamProtocol) Parse(raw []byte) ([]wsfeed.Event, error) {
	if string(raw) == "pong" {
		return nil, nil
	}
	var message struct {
		Event string          `json:"event"`
		Msg   string          `json:"msg"`
		Arg   streamArg       `json:"arg"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil, err
	}
	if message.Event == "error" {
		return nil, errors.New("okx stream error: " + message.Msg)
	}
	if message.Event != "" || len(message.Data) == 0 {
		return nil, nil // An acknowledgement or a notice.
	}
	if bar, isCandle := strings.CutPrefix(message.Arg.Channel, "candle"); isCandle {
		return parseCandles(message.Arg.InstID, bar, message.Data)
	}
	if message.Arg.Channel == "mark-price" {
		return parseMarks(message.Data)
	}
	return nil, nil
}

func parseCandles(inst, bar string, data json.RawMessage) ([]wsfeed.Event, error) {
	interval := ""
	for name, okxName := range barNames {
		if okxName == bar {
			interval = name
		}
	}
	if interval == "" {
		return nil, fmt.Errorf("okx stream: unknown bar %q", bar)
	}
	var rows [][]string
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	events := make([]wsfeed.Event, 0, len(rows))
	for _, row := range rows {
		candle, err := ParseCandle(row)
		if err != nil {
			return nil, err
		}
		events = append(events, wsfeed.Event{Candle: &wsfeed.Candle{
			Symbol: inst, Interval: interval, OpenTime: candle.OpenTime, Open: candle.Open, High: candle.High, Low: candle.Low,
			Close: candle.Close, Volume: candle.Volume, QuoteVolume: candle.QuoteVolume, Closed: candle.Closed,
		}})
	}
	return events, nil
}

func parseMarks(data json.RawMessage) ([]wsfeed.Event, error) {
	var rows []struct {
		InstID string `json:"instId"`
		Mark   string `json:"markPx"`
		Time   string `json:"ts"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	ticks := make([]wsfeed.Tick, 0, len(rows))
	for _, row := range rows {
		price, err := strconv.ParseFloat(row.Mark, 64)
		if err != nil || price <= 0 {
			return nil, errors.New("okx stream: invalid mark price")
		}
		at, _ := strconv.ParseInt(row.Time, 10, 64)
		ticks = append(ticks, wsfeed.Tick{Symbol: row.InstID, Price: price, Time: at})
	}
	return []wsfeed.Event{{Marks: ticks}}, nil
}
