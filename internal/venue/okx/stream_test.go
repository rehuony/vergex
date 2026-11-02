package okx

import (
	"encoding/json"
	"testing"
)

// These messages were captured from the live stream.
const (
	candleMessage = `{"arg":{"channel":"candle1Dutc","instId":"BTC-USDT-SWAP"},"data":[["1793432280000","77314.2","77315.2","77307.2","77307.3","176.80","1.7680","136681.49555000","1"]]}`
	markMessage   = `{"arg":{"channel":"mark-price","instId":"BTC-USDT-SWAP"},"data":[{"instId":"BTC-USDT-SWAP","instType":"SWAP","markPx":"77307.1","ts":"1793432317054"}]}`
)

func TestParsesACandle(t *testing.T) {
	events, err := streamProtocol{}.Parse([]byte(candleMessage))
	if err != nil || len(events) != 1 || events[0].Candle == nil {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
	c := events[0].Candle
	if c.Symbol != "BTC-USDT-SWAP" || c.Interval != "1d" || c.OpenTime != 1793432280000 || !c.Closed {
		t.Fatalf("candle = %+v", c)
	}
	if c.Open != 77314.2 || c.High != 77315.2 || c.Low != 77307.2 || c.Close != 77307.3 || c.Volume != 1.768 {
		t.Fatalf("values = %+v", c)
	}
}

func TestParsesAMarkPrice(t *testing.T) {
	events, err := streamProtocol{}.Parse([]byte(markMessage))
	if err != nil || len(events) != 1 || len(events[0].Marks) != 1 {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
	if tick := events[0].Marks[0]; tick.Symbol != "BTC-USDT-SWAP" || tick.Price != 77307.1 || tick.Time != 1793432317054 {
		t.Fatalf("tick = %+v", tick)
	}
}

func TestIgnoresPongsAndAcknowledgementsAndReportsErrors(t *testing.T) {
	protocol := streamProtocol{}
	for _, raw := range []string{"pong", `{"event":"subscribe","arg":{"channel":"candle1m","instId":"BTC-USDT-SWAP"},"connId":"x"}`, `{"event":"notice","msg":"m","code":"64010"}`} {
		if events, err := protocol.Parse([]byte(raw)); err != nil || len(events) != 0 {
			t.Fatalf("%s: events = %+v, err = %v", raw, events, err)
		}
	}
	if _, err := protocol.Parse([]byte(`{"event":"error","msg":"nope","code":"60018"}`)); err == nil {
		t.Fatal("an error message should say so")
	}
}

func TestBuildsSubscribeMessages(t *testing.T) {
	var message struct {
		Op   string      `json:"op"`
		Args []streamArg `json:"args"`
	}
	topics := []string{CandleTopic("BTCUSDT", "1h"), MarkTopic("ETHUSDT")}
	if err := json.Unmarshal(streamProtocol{}.Control(topics, true), &message); err != nil {
		t.Fatal(err)
	}
	if message.Op != "subscribe" || message.Args[0] != (streamArg{"candle1H", "BTC-USDT-SWAP"}) || message.Args[1] != (streamArg{"mark-price", "ETH-USDT-SWAP"}) {
		t.Fatalf("message = %+v", message)
	}
	if CandleTopic("BTCUSDT", "1w") != "" {
		t.Fatal("an interval OKX is not set up for has no topic")
	}
}
