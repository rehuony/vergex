package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CallWithRequestStream streams the LLM response via SSE (Server-Sent Events).
// onChunk is called with the full accumulated text so far after each received chunk.
// Returns the complete final text when the stream ends.
//
// Idle timeout: if no chunk arrives for 30 seconds the stream is cancelled automatically.
// This prevents the scanner from blocking indefinitely on a hung or stalled connection.
func (client *Client) CallWithRequestStream(req *Request, onChunk func(string)) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("AI API key not set")
	}
	if req.Model == "" {
		req.Model = client.Model
	}
	req.Stream = true

	requestBody := client.Hooks.BuildRequestBodyFromRequest(req)
	jsonData, err := client.Hooks.MarshalRequestBody(requestBody)
	if err != nil {
		return "", err
	}

	url := client.Hooks.BuildUrl()
	httpReq, err := client.buildHTTPRequestWithContext(contextFromRequest(req), url, jsonData)
	if err != nil {
		return "", err
	}

	// Idle-timeout watchdog: cancel the request if no SSE line arrives for 60 seconds.
	// This breaks the scanner out of an indefinitely blocking Read on a hung connection.
	const idleTimeout = 60 * time.Second
	ctx, cancel := context.WithCancel(contextFromRequest(req))
	defer cancel()
	resetCh := make(chan struct{}, 1)
	go func() {
		t := time.NewTimer(idleTimeout)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cancel() // idle timeout: kill the connection
				return
			case <-resetCh:
				// received a line — reset the idle timer
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(idleTimeout)
			}
		}
	}()

	httpReq = httpReq.WithContext(ctx)
	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("streaming request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	text, usage, err := ParseSSEStream(resp.Body, onChunk, func() {
		select {
		case resetCh <- struct{}{}:
		default:
		}
	})
	client.ReportUsage(usage)
	return text, err
}

type sseStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text         string  `json:"text"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
}

// openAIUsage is the usage block of OpenAI-compatible responses. Cached
// prompt tokens are reported as prompt_tokens_details.cached_tokens (OpenAI,
// Qwen, Kimi, Grok) or prompt_cache_hit_tokens (DeepSeek).
type openAIUsage struct {
	PromptTokens         int `json:"prompt_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	TotalTokens          int `json:"total_tokens"`
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails  *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) tokenUsage() *TokenUsage {
	if u.TotalTokens <= 0 {
		return nil
	}
	cached := u.PromptCacheHitTokens
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > cached {
		cached = u.PromptTokensDetails.CachedTokens
	}
	return &TokenUsage{
		PromptTokens:       u.PromptTokens,
		CachedPromptTokens: cached,
		CompletionTokens:   u.CompletionTokens,
		TotalTokens:        u.TotalTokens,
	}
}

func sseData(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	// The optional space after ':' is not required by the SSE specification.
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

// ParseSSEStream reads an SSE response body, accumulates text deltas,
// and calls onChunk with the full accumulated text after each chunk.
// It accepts OpenAI delta chunks, final message chunks, and legacy text chunks.
// If onLine is non-nil, it is called after each raw SSE line is scanned
// (useful for resetting idle-timeout watchdogs).
// Returns the complete accumulated text and any parsed token usage (nil if absent).
func ParseSSEStream(body io.Reader, onChunk func(string), onLine func()) (string, *TokenUsage, error) {
	var accumulated strings.Builder
	var usage *TokenUsage
	scanner := bufio.NewScanner(body)
	// Scanner defaults to 64 KiB. Some gateways put a complete final response
	// on one SSE line, so allow a bounded large event instead of dropping it.
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var eventData []string
	completed := false
	finishSeen := false
	var parseErr error
	processEvent := func() {
		if len(eventData) == 0 || completed || parseErr != nil {
			return
		}
		data := strings.Join(eventData, "\n")
		eventData = eventData[:0]
		if strings.TrimSpace(data) == "[DONE]" {
			completed = true
			return
		}
		var chunk sseStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			parseErr = fmt.Errorf("malformed SSE data event: %w", err)
			return
		}
		if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
			usage = chunk.Usage.tokenUsage()
		}
		if len(chunk.Choices) == 0 {
			return
		}
		choice := chunk.Choices[0]
		if choice.FinishReason != nil {
			// Usage-only events commonly follow the finish chunk. Keep parsing
			// until [DONE]/EOF so settled token usage is not discarded.
			finishSeen = true
		}
		switch {
		case choice.Delta.Content != "":
			accumulated.WriteString(choice.Delta.Content)
		case choice.Message.Content != "":
			accumulated.Reset()
			accumulated.WriteString(choice.Message.Content)
		case choice.Text != "":
			accumulated.WriteString(choice.Text)
		default:
			return
		}
		if onChunk != nil {
			onChunk(accumulated.String())
		}
	}

	for scanner.Scan() {
		if onLine != nil {
			onLine()
		}
		line := scanner.Text()
		if line == "" {
			processEvent()
			if completed || parseErr != nil {
				break
			}
			continue
		}
		if data, ok := sseData(line); ok {
			eventData = append(eventData, data)
		}
	}
	processEvent()

	if err := scanner.Err(); err != nil {
		return accumulated.String(), usage, fmt.Errorf("stream interrupted: %w", err)
	}
	if parseErr != nil {
		return accumulated.String(), usage, parseErr
	}
	if !completed && !finishSeen {
		return accumulated.String(), usage, fmt.Errorf("stream ended without a completion marker")
	}

	return accumulated.String(), usage, nil
}

// DescribeSSEBody returns bounded, content-free diagnostics for an SSE body.
// It intentionally reports only shape counters so prompts and model output are
// never copied into logs or errors.
func DescribeSSEBody(body []byte) string {
	var lines, dataLines, jsonLines, malformed, choices int
	var contentChunks, reasoningChunks, messageChunks, textChunks int
	var done bool
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		lines++
		data, ok := sseData(scanner.Text())
		if !ok {
			continue
		}
		dataLines++
		if data == "[DONE]" {
			done = true
			continue
		}
		var chunk sseStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			malformed++
			continue
		}
		jsonLines++
		choices += len(chunk.Choices)
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				contentChunks++
			}
			if choice.Delta.ReasoningContent != "" {
				reasoningChunks++
			}
			if choice.Message.Content != "" {
				messageChunks++
			}
			if choice.Text != "" {
				textChunks++
			}
		}
	}
	if scanner.Err() != nil {
		malformed++
	}
	return fmt.Sprintf("lines=%d, data_lines=%d, json_lines=%d, malformed=%d, choices=%d, content_chunks=%d, reasoning_chunks=%d, message_chunks=%d, text_chunks=%d, done=%t",
		lines, dataLines, jsonLines, malformed, choices, contentChunks, reasoningChunks, messageChunks, textChunks, done)
}
