package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// streamingServer answers each request from a script of raw bodies, as server-sent events.
type streamReply struct {
	status  int
	body    string
	json    bool
	waitFor <-chan struct{}
}

func streamingServer(t *testing.T, replies ...streamReply) (*BaseClient, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		at := int(calls.Add(1)) - 1
		reply := replies[min(at, len(replies)-1)]
		if reply.status != 0 {
			w.WriteHeader(reply.status)
			_, _ = io.WriteString(w, reply.body)
			return
		}
		if reply.json {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, reply.body)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, reply.body)
		w.(http.Flusher).Flush()
		if reply.waitFor != nil {
			select {
			case <-reply.waitFor:
			case <-r.Context().Done():
			}
		}
	}))
	t.Cleanup(srv.Close)
	client := NewClient(WithHTTPClient(srv.Client()), WithLogger(NewMockLogger()), WithAPIKey("key"), WithRetryWaitBase(time.Millisecond)).(*BaseClient)
	client.BaseURL, client.Model = srv.URL, "m"
	return client, calls
}

func chunk(content string) string {
	return fmt.Sprintf(`data: {"choices":[{"delta":{"content":%q},"finish_reason":null}]}`+"\n\n", content)
}

const streamEnd = `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
	`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_cache_hit_tokens":4}}` + "\n\n" +
	"data: [DONE]\n\n"

func streamRequest() *Request {
	return &Request{Messages: []Message{NewUserMessage("hi")}}
}

func collect(client *BaseClient, req *Request) (string, *Response, error) {
	var seen strings.Builder
	response, err := client.StreamWithRequest(req, func(text string) { seen.WriteString(text) })
	return seen.String(), response, err
}

func TestAStreamReadsLikeAnOrdinaryReply(t *testing.T) {
	client, _ := streamingServer(t, streamReply{body: chunk("Hel") + `: keep-alive` + "\n\n" + chunk("lo") + streamEnd})
	seen, response, err := collect(client, streamRequest())
	if err != nil {
		t.Fatal(err)
	}
	if seen != "Hello" || response.Content != "Hello" || response.FinishReason != "stop" {
		t.Fatalf("seen=%q response=%+v", seen, response)
	}
	if u := response.Usage; u == nil || u.PromptTokens != 10 || u.CompletionTokens != 2 || u.CachedPromptTokens != 4 {
		t.Fatalf("usage = %+v", response.Usage)
	}
}

func TestToolCallsAreAssembledFromTheirPieces(t *testing.T) {
	body := `data: {"choices":[{"delta":{"reasoning_content":"think "}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"reasoning_content":"more"}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"first","arguments":"{\"x\""}}]}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","type":"function","function":{"name":"second","arguments":"{}"}}]}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":1}"}}]}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n" + "data: [DONE]\n\n"
	client, _ := streamingServer(t, streamReply{body: body})
	seen, response, err := collect(client, streamRequest())
	if err != nil {
		t.Fatal(err)
	}
	if seen != "" || response.ReasoningContent != "think more" || response.FinishReason != "tool_calls" {
		t.Fatalf("seen=%q response=%+v", seen, response)
	}
	if len(response.ToolCalls) != 2 || response.ToolCalls[0].Function.Arguments != `{"x":1}` || response.ToolCalls[1].ID != "b" {
		t.Fatalf("tool calls = %+v", response.ToolCalls)
	}
}

func TestAStreamAsksForItsUsage(t *testing.T) {
	body := NewClient().(*BaseClient).StreamBody(map[string]any{})
	if body["stream"] != true || body["stream_options"] == nil {
		t.Fatalf("body = %v", body)
	}
}

func TestAProviderThatWillNotStreamIsAskedTheOrdinaryWay(t *testing.T) {
	plain := `{"choices":[{"message":{"content":"all at once"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	for name, refusal := range map[string]streamReply{
		"refused":     {status: http.StatusBadRequest, body: `{"error":"stream_options not supported"}`},
		"unsupported": {status: http.StatusUnprocessableEntity, body: `nope`},
		"plain JSON":  {json: true, body: plain},
	} {
		client, calls := streamingServer(t, refusal, streamReply{json: true, body: plain})
		seen, response, err := collect(client, streamRequest())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if seen != "" || response.Content != "all at once" || calls.Load() != 2 {
			t.Fatalf("%s: seen=%q response=%+v calls=%d", name, seen, response, calls.Load())
		}
	}
}

func TestABadKeyIsNotAskedAgain(t *testing.T) {
	client, calls := streamingServer(t, streamReply{status: http.StatusUnauthorized, body: `{"error":"bad key"}`})
	if _, _, err := collect(client, streamRequest()); err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestAStreamThatFailsBeforeAnyTextIsTriedAgain(t *testing.T) {
	client, calls := streamingServer(t,
		streamReply{status: http.StatusServiceUnavailable, body: `down`},
		streamReply{body: chunk("ok") + streamEnd})
	seen, _, err := collect(client, streamRequest())
	if err != nil || seen != "ok" || calls.Load() != 2 {
		t.Fatalf("err=%v seen=%q calls=%d", err, seen, calls.Load())
	}
}

// Starting over after the user began to read would show them the same words twice.
func TestAStreamThatBreaksAfterTextIsNotTriedAgain(t *testing.T) {
	client, calls := streamingServer(t, streamReply{body: chunk("half of it")}, streamReply{body: chunk("again") + streamEnd})
	seen, _, err := collect(client, streamRequest())
	if err == nil || calls.Load() != 1 || seen != "half of it" {
		t.Fatalf("err=%v seen=%q calls=%d", err, seen, calls.Load())
	}
}

func TestStoppingAStreamEndsItAtOnce(t *testing.T) {
	never := make(chan struct{})
	defer close(never)
	client, _ := streamingServer(t, streamReply{body: chunk("started"), waitFor: never})
	ctx, cancel := context.WithCancel(context.Background())
	req := streamRequest()
	req.Ctx = ctx
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	started := time.Now()
	_, err := client.StreamWithRequest(req, nil)
	if !errors.Is(err, context.Canceled) || time.Since(started) > 5*time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(started))
	}
}

func TestAQuietStreamCountsAsTimedOut(t *testing.T) {
	orig := streamIdleLimit
	streamIdleLimit = 30 * time.Millisecond
	t.Cleanup(func() { streamIdleLimit = orig })
	never := make(chan struct{})
	defer close(never)
	client, _ := streamingServer(t, streamReply{body: chunk("started"), waitFor: never})
	_, _, err := collect(client, streamRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if kind := Classify(client, err).Kind; kind != FailureTimeout {
		t.Fatalf("kind = %s", kind)
	}
}

func TestAStreamIsReportedOnceLikeAnyCall(t *testing.T) {
	orig := CallCallback
	var reports []CallReport
	CallCallback = func(report CallReport) { reports = append(reports, report) }
	t.Cleanup(func() { CallCallback = orig })
	client, _ := streamingServer(t, streamReply{body: chunk("hi") + streamEnd})
	if _, _, err := collect(client, streamRequest()); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].PromptTokens != 10 || reports[0].FinishReason != "stop" || reports[0].Error != "" {
		t.Fatalf("reports = %+v", reports)
	}
}

// A client that cannot stream still answers.
func TestStreamFallsBackOnAClientThatCannot(t *testing.T) {
	response, err := Stream(plainClient{}, streamRequest(), func(string) { t.Fatal("no text was expected") })
	if err != nil || response.Content != "plain" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

type plainClient struct{ Client }

func (plainClient) CallWithRequestFull(*Request) (*Response, error) {
	return &Response{Content: "plain"}, nil
}
