package wsfeed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// echoProtocol treats every text message as one price event named after its text.
type echoProtocol struct{}

func (echoProtocol) Control(topics []string, subscribe bool) []byte {
	raw, _ := json.Marshal(map[string]any{"topics": topics, "subscribe": subscribe})
	return raw
}

func (echoProtocol) Parse(raw []byte) ([]Event, error) {
	return []Event{{Prices: []Tick{{Symbol: string(raw)}}}}, nil
}

// fakeServer is an exchange: it records control messages and hands each connection
// to a test through conns.
type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	controls []map[string]any
	conns    chan *websocket.Conn
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	server := &fakeServer{conns: make(chan *websocket.Conn, 8)}
	upgrader := websocket.Upgrader{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		server.conns <- conn
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var control map[string]any
			if json.Unmarshal(raw, &control) == nil {
				server.mu.Lock()
				server.controls = append(server.controls, control)
				server.mu.Unlock()
			}
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (s *fakeServer) url() string { return "ws" + strings.TrimPrefix(s.URL, "http") }

func (s *fakeServer) subscribedTopics() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := map[string]bool{}
	for _, control := range s.controls {
		for _, topic := range control["topics"].([]any) {
			if control["subscribe"].(bool) {
				topics[topic.(string)] = true
			} else {
				delete(topics, topic.(string))
			}
		}
	}
	return topics
}

func (s *fakeServer) resetControls() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controls = nil
}

func testConfig(url string) Config {
	return Config{
		URL: url, Protocol: echoProtocol{}, MaxTopics: 3, BatchTopics: 2,
		ControlEvery: 5 * time.Millisecond, Silence: 200 * time.Millisecond,
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func runFeed(t *testing.T, feed *Feed, out chan Event) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		feed.Run(ctx, out)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("feed did not stop with its context")
		}
	})
}

func TestFeedConnectsOnlyWithTopicsAndFollowsThem(t *testing.T) {
	server := newFakeServer(t)
	feed := New(testConfig(server.url()))
	runFeed(t, feed, make(chan Event, 8))

	select {
	case <-server.conns:
		t.Fatal("connected with no topics")
	case <-time.After(50 * time.Millisecond):
	}

	feed.Set([]string{"a", "b", "c", "d"})
	<-server.conns
	waitFor(t, "the topics within the limit", func() bool {
		topics := server.subscribedTopics()
		return topics["a"] && topics["b"] && topics["c"]
	})
	if server.subscribedTopics()["d"] {
		t.Fatal("subscribed past the limit")
	}
	if status := feed.Status(); status.LeftOut != 1 {
		t.Fatalf("left out = %d, want 1", status.LeftOut)
	}

	feed.Set([]string{"c", "e"})
	waitFor(t, "the changed topics", func() bool {
		topics := server.subscribedTopics()
		return len(topics) == 2 && topics["c"] && topics["e"]
	})
}

func TestFeedReconnectsAndSubscribesAgainAfterSilence(t *testing.T) {
	server := newFakeServer(t)
	feed := New(testConfig(server.url()))
	runFeed(t, feed, make(chan Event, 8))

	feed.Set([]string{"a"})
	first := <-server.conns
	defer first.Close()
	waitFor(t, "the first subscription", func() bool { return server.subscribedTopics()["a"] })
	waitFor(t, "a healthy connection", feed.Healthy)
	server.resetControls()

	// The server says nothing, so the feed gives up on it and connects again.
	var second *websocket.Conn
	select {
	case second = <-server.conns:
		defer second.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("did not reconnect after silence")
	}
	waitFor(t, "the second subscription", func() bool { return server.subscribedTopics()["a"] })
	if feed.Status().Reconnects == 0 {
		t.Fatal("reconnect was not counted")
	}
}

func TestFeedReassemblesFragmentedMessages(t *testing.T) {
	server := newFakeServer(t)
	feed := New(testConfig(server.url()))
	out := make(chan Event, 8)
	runFeed(t, feed, out)
	feed.Set([]string{"a"})
	conn := <-server.conns
	defer conn.Close()

	writer, err := conn.NextWriter(websocket.TextMessage)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"hel", "lo"} {
		if _, err := writer.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-out:
		if event.Prices[0].Symbol != "hello" {
			t.Fatalf("got %q", event.Prices[0].Symbol)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
}

func TestFeedDropsRatherThanWaitsForASlowConsumer(t *testing.T) {
	server := newFakeServer(t)
	feed := New(testConfig(server.url()))
	out := make(chan Event, 1) // Nobody reads this.
	runFeed(t, feed, out)
	feed.Set([]string{"a"})
	conn := <-server.conns
	defer conn.Close()

	for range 20 {
		if err := conn.WriteMessage(websocket.TextMessage, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "all messages to be read", func() bool { return feed.Status().Messages == 20 })
	if dropped := feed.Status().Dropped; dropped != 19 {
		t.Fatalf("dropped = %d, want 19", dropped)
	}
	if !feed.Healthy() {
		t.Fatal("a slow consumer must not hurt the connection")
	}
}

func TestFeedClosesWhenNoTopicsRemain(t *testing.T) {
	server := newFakeServer(t)
	feed := New(testConfig(server.url()))
	runFeed(t, feed, make(chan Event, 8))
	feed.Set([]string{"a"})
	conn := <-server.conns
	defer conn.Close()
	waitFor(t, "the connection", func() bool { return feed.Status().Connected })

	feed.Set(nil)
	waitFor(t, "the connection to close", func() bool { return !feed.Status().Connected })
}

func TestFeedReplacesAnOldConnection(t *testing.T) {
	server := newFakeServer(t)
	config := testConfig(server.url())
	config.MaxAge = 30 * time.Millisecond
	config.Silence = time.Minute
	feed := New(config)
	runFeed(t, feed, make(chan Event, 8))
	feed.Set([]string{"a"})
	for range 2 {
		select {
		case conn := <-server.conns:
			defer conn.Close()
		case <-time.After(5 * time.Second):
			t.Fatal("connection was not replaced")
		}
	}
	if feed.Status().Reconnects != 0 {
		t.Fatal("a planned replacement is not a failure")
	}
}

func TestFeedSendsItsPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if string(raw) == "ping" {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("pong"))
			}
		}
	}))
	defer server.Close()
	config := testConfig("ws" + strings.TrimPrefix(server.URL, "http"))
	config.Ping, config.PingEvery = []byte("ping"), 10*time.Millisecond
	feed := New(config)
	out := make(chan Event, 8)
	runFeed(t, feed, out)
	feed.Set([]string{"a"})
	select {
	case event := <-out:
		if event.Prices[0].Symbol != "pong" {
			t.Fatalf("got %q", event.Prices[0].Symbol)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no answer to the ping")
	}
}
