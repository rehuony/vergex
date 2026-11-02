package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// streamIdleLimit is how long a stream may go without a byte before it counts as dead. A
// stream can rightly run for minutes, so the time it takes as a whole is left to the context.
var streamIdleLimit = 90 * time.Second

// Streamer is a client that can show a reply as it is written.
type Streamer interface {
	StreamWithRequest(req *Request, onText func(string)) (*Response, error)
}

// Stream makes a call, handing onText the reply's text as it arrives. A client that cannot
// stream answers all at once, and onText simply never hears from it.
func Stream(client Client, req *Request, onText func(string)) (*Response, error) {
	if streamer, ok := client.(Streamer); ok {
		return streamer.StreamWithRequest(req, onText)
	}
	return client.CallWithRequestFull(req)
}

// streamBroken is a stream that failed after the user began to read it. Starting it again
// would show them the same words twice, so it is not tried again.
type streamBroken struct{ error }

func (e streamBroken) Unwrap() error { return e.error }

// errNotAStream says a provider answered a streaming request all at once.
var errNotAStream = errors.New("the provider did not stream")

// StreamWithRequest streams a call. A provider that will not stream, as custom endpoints may
// not, is asked again the ordinary way, with nothing to show for it but the wait.
func (client *BaseClient) StreamWithRequest(req *Request, onText func(string)) (*Response, error) {
	if client.APIKey == "" {
		return nil, fmt.Errorf("model API key not set, please call SetAPIKey first")
	}
	if req.Model == "" {
		req.Model = client.Model
	}
	ctx := contextFromRequest(req)
	response, err := retrying(client, ctx, func() (*Response, error) {
		started := time.Now()
		spoke := false
		var usage *TokenUsage
		result, err := client.exchangeStream(ctx, req, func(delta StreamDelta) {
			if delta.Text != "" {
				spoke = true
				if onText != nil {
					onText(delta.Text)
				}
			}
			if delta.Usage != nil {
				usage = delta.Usage
			}
		})
		if errors.Is(err, errNotAStream) || streamRefused(err) {
			return nil, err
		}
		finish := ""
		if result != nil {
			finish = result.FinishReason
			if result.Usage != nil {
				usage = result.Usage
			}
		}
		client.report(ctx, started, usage, finish, err)
		if err != nil && spoke {
			return nil, streamBroken{err}
		}
		return result, err
	})
	if errors.Is(err, errNotAStream) || streamRefused(err) {
		client.Log.Infof("[%s] streaming refused, asking without it", client.String())
		return client.CallWithRequestFull(req)
	}
	return response, err
}

// streamRefused says a provider turned down the request for being a streaming one, as far as
// that can be told: these statuses say nothing of the key or the provider's health.
func streamRefused(err error) bool {
	var refused *StatusError
	if !errors.As(err, &refused) {
		return false
	}
	switch refused.Status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// exchangeStream makes one streaming request.
func (client *BaseClient) exchangeStream(ctx context.Context, req *Request, onDelta func(StreamDelta)) (*Response, error) {
	body := client.Hooks.StreamBody(client.Hooks.BuildRequestBodyFromRequest(req))
	jsonData, err := client.Hooks.MarshalRequestBody(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReq, err := client.buildHTTPRequestWithContext(ctx, client.Hooks.BuildUrl(), jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	// The client's timeout counts reading the body too, which a long reply can rightly outlast.
	unbounded := *client.HTTPClient
	unbounded.Timeout = 0
	idle := newIdleWatch(streamIdleLimit, cancel)
	defer idle.stop()
	resp, err := unbounded.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", idle.explain(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response: %w", idle.explain(readErr))
		}
		return nil, newStatusError(resp, body)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return nil, errNotAStream
	}
	response, err := client.Hooks.ParseStream(idle.reader(resp.Body), onDelta)
	if err != nil {
		return nil, idle.explain(err)
	}
	return response, nil
}

// StreamDelta is what one piece of a stream added.
type StreamDelta struct {
	Text string
	// Usage is what is known of the tokens so far, where the provider says as it goes.
	Usage *TokenUsage
}

// StreamBody asks for a stream, and for its usage at the end, which OpenAI-format providers
// leave out of one otherwise.
func (client *BaseClient) StreamBody(body map[string]any) map[string]any {
	body["stream"] = true
	body["stream_options"] = map[string]any{"include_usage": true}
	return body
}

// EachEvent reads a server-sent event stream, handing over each event's name and data. It
// stops at the end of the stream or the first error from handle.
func EachEvent(stream io.Reader, handle func(event, data string) error) error {
	reader := bufio.NewReaderSize(stream, 64<<10)
	var event string
	var data strings.Builder
	flush := func() error {
		if data.Len() == 0 {
			event = ""
			return nil
		}
		text := data.String()
		name := event
		data.Reset()
		event = ""
		return handle(name, text)
	}
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
		case strings.HasPrefix(line, ":"):
			// A comment, which is how some providers keep a quiet stream alive.
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		if err != nil {
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("failed to read the stream: %w", err)
		}
	}
}

// ParseStream reads an OpenAI-format stream into the same Response an ordinary call gets.
func (client *BaseClient) ParseStream(stream io.Reader, onDelta func(StreamDelta)) (*Response, error) {
	response := &Response{}
	var text, reasoning strings.Builder
	calls := map[int]*ToolCall{}
	done := false
	err := EachEvent(stream, func(_, data string) error {
		if data == "[DONE]" {
			done = true
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *openAIUsage    `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("failed to parse the stream: %w", err)
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return &StatusError{Status: http.StatusBadGateway, Body: string(chunk.Error)}
		}
		if chunk.Usage != nil {
			if usage := chunk.Usage.tokenUsage(); usage != nil {
				response.Usage = usage
			}
		}
		if len(chunk.Choices) == 0 {
			return nil
		}
		choice := chunk.Choices[0]
		if choice.FinishReason != nil {
			response.FinishReason = *choice.FinishReason
		}
		if choice.Delta.Content != "" {
			text.WriteString(choice.Delta.Content)
			onDelta(StreamDelta{Text: choice.Delta.Content})
		}
		reasoning.WriteString(choice.Delta.ReasoningContent)
		for _, piece := range choice.Delta.ToolCalls {
			call := calls[piece.Index]
			if call == nil {
				call = &ToolCall{Type: "function"}
				calls[piece.Index] = call
			}
			if piece.ID != "" {
				call.ID = piece.ID
			}
			if piece.Type != "" {
				call.Type = piece.Type
			}
			if piece.Function.Name != "" {
				call.Function.Name = piece.Function.Name
			}
			call.Function.Arguments += piece.Function.Arguments
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !done && response.FinishReason == "" {
		return nil, errors.New("the stream ended before the reply did: EOF")
	}
	response.Content, response.ReasoningContent = text.String(), reasoning.String()
	for index := 0; index < len(calls); index++ {
		if call := calls[index]; call != nil {
			response.ToolCalls = append(response.ToolCalls, *call)
		}
	}
	return response, nil
}

// idleWatch cancels a stream that has gone quiet, and says so when its reader fails.
type idleWatch struct {
	limit  time.Duration
	timer  *time.Timer
	mu     sync.Mutex
	fired  bool
	cancel context.CancelFunc
}

func newIdleWatch(limit time.Duration, cancel context.CancelFunc) *idleWatch {
	watch := &idleWatch{limit: limit, cancel: cancel}
	watch.timer = time.AfterFunc(limit, watch.fire)
	return watch
}

func (w *idleWatch) fire() {
	w.mu.Lock()
	w.fired = true
	w.mu.Unlock()
	w.cancel()
}

func (w *idleWatch) stop() { w.timer.Stop() }

// explain turns the cancellation of a quiet stream into the timeout it really is.
func (w *idleWatch) explain(err error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fired {
		return fmt.Errorf("the stream went quiet for %v: %w", w.limit, context.DeadlineExceeded)
	}
	return err
}

func (w *idleWatch) reader(body io.Reader) io.Reader { return &watchedReader{body: body, watch: w} }

// watchedReader counts every read as a sign of life.
type watchedReader struct {
	body  io.Reader
	watch *idleWatch
}

func (r *watchedReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.watch.timer.Reset(r.watch.limit)
	}
	return n, err
}
