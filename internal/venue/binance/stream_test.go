package binance

import (
	"encoding/json"
	"testing"
)

// These messages were captured from the live stream.
const (
	klineMessage = `{"stream":"btcusdt@kline_1m","data":{"e":"kline","E":1793431047285,"s":"BTCUSDT","k":{"t":1793431020000, "T":1793431079999, "s":"BTCUSDT", "i":"1m", "f":8223260264, "L":8223260998, "o":"77413.70", "c":"77425.00", "h":"77426.00", "l":"77405.90", "v":"21.078", "n":731, "x":false, "q":"1631744.22360", "V":"17.622", "Q":"1364200.38920", "B":"0"}}}`
	miniMessage  = `{"stream":"!miniTicker@arr","data":[{"e":"24hrMiniTicker","E":1793431047512,"s":"AINUSDT","ps":"AINUSDT","c":"0.2650100","o":"0.1680900","h":"0.2927000","l":"0.1406000","v":"1651779768","q":"376384410.9527400","st":1}]}`
	markMessage  = `{"stream":"btcusdt@markPrice@1s","data":{"e":"markPriceUpdate","E":1793431048000,"s":"BTCUSDT","p":"77425.00000000","ap":"77425.00000000","P":"77392.35003452","i":"77437.19847826","r":"0.00010000","T":1793433600000,"st":1}}`
)

func TestParsesAKlineWithoutMixingUpFieldCases(t *testing.T) {
	events, err := (&streamProtocol{}).Parse([]byte(klineMessage))
	if err != nil || len(events) != 1 || events[0].Candle == nil {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
	c := events[0].Candle
	if c.Symbol != "BTCUSDT" || c.Interval != "1m" || c.OpenTime != 1793431020000 || c.Closed || c.Trades != 731 {
		t.Fatalf("candle = %+v", c)
	}
	if c.Open != 77413.70 || c.High != 77426 || c.Low != 77405.90 || c.Close != 77425 {
		t.Fatalf("prices = %+v", c)
	}
	if c.Volume != 21.078 || c.TakerBuyVolume != 17.622 || c.QuoteVolume != 1631744.22360 || c.TakerBuyQuote != 1364200.38920 {
		t.Fatalf("volumes = %+v", c)
	}
}

func TestParsesPrices(t *testing.T) {
	events, err := (&streamProtocol{}).Parse([]byte(miniMessage))
	if err != nil || len(events) != 1 || len(events[0].Prices) != 1 {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
	if tick := events[0].Prices[0]; tick.Symbol != "AINUSDT" || tick.Price != 0.26501 || tick.Open24h != 0.16809 || tick.Time != 1793431047512 {
		t.Fatalf("tick = %+v", tick)
	}
}

func TestParsesTheMarkNotTheIndex(t *testing.T) {
	events, err := (&streamProtocol{}).Parse([]byte(markMessage))
	if err != nil || len(events) != 1 || len(events[0].Marks) != 1 {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
	if tick := events[0].Marks[0]; tick.Symbol != "BTCUSDT" || tick.Price != 77425 {
		t.Fatalf("tick = %+v", tick)
	}
}

func TestIgnoresAcknowledgementsAndReportsErrors(t *testing.T) {
	protocol := &streamProtocol{}
	if events, err := protocol.Parse([]byte(`{"result":null,"id":1}`)); err != nil || len(events) != 0 {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
	if _, err := protocol.Parse([]byte(`{"error":{"code":2,"msg":"bad"},"id":1}`)); err == nil {
		t.Fatal("an error message should say so")
	}
	if _, err := protocol.Parse([]byte(`not json`)); err == nil {
		t.Fatal("bad json should say so")
	}
}

func TestBuildsSubscribeMessages(t *testing.T) {
	var message struct {
		Method string   `json:"method"`
		Params []string `json:"params"`
		ID     int64    `json:"id"`
	}
	protocol := &streamProtocol{}
	if err := json.Unmarshal(protocol.Control([]string{KlineTopic("BTCUSDT", "1m")}, true), &message); err != nil {
		t.Fatal(err)
	}
	if message.Method != "SUBSCRIBE" || message.Params[0] != "btcusdt@kline_1m" || message.ID != 1 {
		t.Fatalf("message = %+v", message)
	}
	if err := json.Unmarshal(protocol.Control([]string{MarkTopic("ETHUSDT")}, false), &message); err != nil {
		t.Fatal(err)
	}
	if message.Method != "UNSUBSCRIBE" || message.Params[0] != "ethusdt@markPrice@1s" || message.ID != 2 {
		t.Fatalf("message = %+v", message)
	}
}
