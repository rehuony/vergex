// Package provider — ClaudeClient implements the Anthropic Messages API.
//
// Wire-format differences from the OpenAI-compatible base BaseClient:
//
//	┌─────────────────────┬───────────────────────────┬─────────────────────────────────┐
//	│ Concept             │ OpenAI format              │ Anthropic format                │
//	├─────────────────────┼───────────────────────────┼─────────────────────────────────┤
//	│ Endpoint            │ /v1/chat/completions       │ /v1/messages                    │
//	│ Auth header         │ Authorization: Bearer xxx  │ x-api-key: xxx                  │
//	│ System prompt       │ messages[0] role=system    │ top-level "system" field        │
//	│ Tool definition     │ type=function + parameters │ name + description + input_schema│
//	│ Tool choice         │ "auto" (string)            │ {"type":"auto"} (object)        │
//	│ Assistant tool call │ tool_calls array           │ content[{type:tool_use,...}]    │
//	│ Tool result         │ role=tool + tool_call_id   │ role=user content[tool_result]  │
//	│ Max tokens          │ max_tokens                 │ max_tokens (same)               │
//	└─────────────────────┴───────────────────────────┴─────────────────────────────────┘
package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"vergex/internal/agent"
)

const (
	DefaultClaudeBaseURL = "https://api.anthropic.com/v1"
	DefaultClaudeModel   = "claude-opus-5-5"
)

func init() {
	agent.RegisterProvider(agent.ProviderClaude, func(opts ...agent.ClientOption) agent.Client {
		return NewClaudeClientWithOptions(opts...)
	})
}

// ClaudeClient wraps the base BaseClient and overrides the methods that differ
// for the Anthropic Messages API.  All other behaviour (retry, timeout,
// logging) is inherited unchanged.
type ClaudeClient struct {
	*agent.BaseClient
}

func (c *ClaudeClient) Base() *agent.BaseClient { return c.BaseClient }

// NewClaudeClientWithOptions creates a ClaudeClient with optional overrides.
func NewClaudeClientWithOptions(opts ...agent.ClientOption) agent.Client {
	baseClient := agent.NewClient(append([]agent.ClientOption{
		agent.WithProvider(agent.ProviderClaude),
		agent.WithModel(DefaultClaudeModel),
		agent.WithBaseURL(DefaultClaudeBaseURL),
	}, opts...)...).(*agent.BaseClient)

	c := &ClaudeClient{BaseClient: baseClient}
	baseClient.Hooks = c // wire dynamic dispatch to ClaudeClient
	return c
}

// ── Hook overrides ────────────────────────────────────────────────────────────

// SetAPIKey stores credentials and optional custom endpoint / model.
func (c *ClaudeClient) SetAPIKey(apiKey, customURL, customModel string) {
	c.APIKey = apiKey
	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [agent] Claude BaseURL: %s", customURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [agent] Claude Model: %s", customModel)
	}
}

// SetAuthHeader uses x-api-key instead of Authorization: Bearer.
func (c *ClaudeClient) SetAuthHeader(h http.Header) {
	h.Set("x-api-key", c.APIKey)
	h.Set("anthropic-version", "2023-06-01")
}

// BuildUrl targets /messages instead of /chat/completions.
func (c *ClaudeClient) BuildUrl() string {
	return strings.TrimRight(c.BaseURL, "/") + "/messages"
}

// BuildRequestBody builds the Anthropic wire format for the simple
// CallWithMessages path (no tool support).
func (c *ClaudeClient) BuildRequestBody(systemPrompt, userPrompt string) map[string]any {
	return map[string]any{
		"model":      c.Model,
		"max_tokens": c.MaxTokens,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}
}

// BuildRequestBodyFromRequest converts a *Request into the Anthropic Messages
// API wire format.
func (c *ClaudeClient) BuildRequestBodyFromRequest(req *agent.Request) map[string]any {
	// ── 1. Separate system prompt from conversation messages ──────────────────
	var systemPrompt string
	var convMsgs []agent.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			convMsgs = append(convMsgs, m)
		}
	}

	// ── 2. Convert messages to Anthropic format ───────────────────────────────
	anthropicMsgs := ConvertMessagesToAnthropic(convMsgs)

	// ── 3. Convert tool definitions (parameters → input_schema) ──────────────
	var anthropicTools []map[string]any
	for _, t := range req.Tools {
		anthropicTools = append(anthropicTools, map[string]any{
			"name":         t.Function.Name,
			"description":  t.Function.Description,
			"input_schema": t.Function.Parameters,
		})
	}

	// ── 4. Assemble request body ──────────────────────────────────────────────
	maxTokens := c.MaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"system":     systemPrompt,
		"messages":   anthropicMsgs,
	}

	if len(anthropicTools) > 0 {
		body["tools"] = anthropicTools
	}

	// tool_choice: Anthropic uses an object, not a string.
	switch req.ToolChoice {
	case "auto":
		body["tool_choice"] = map[string]any{"type": "auto"}
	case "any":
		body["tool_choice"] = map[string]any{"type": "any"}
	case "none", "":
		// omit — no tool_choice sent
	}

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}

	return body
}

// ConvertMessagesToAnthropic translates from the OpenAI-shaped agent.Message
// slice to Anthropic's messages array.
func ConvertMessagesToAnthropic(msgs []agent.Message) []map[string]any {
	var out []map[string]any

	for i := 0; i < len(msgs); {
		msg := msgs[i]

		switch {
		// ── Assistant message carrying tool calls ─────────────────────────────
		case msg.Role == "assistant" && len(msg.ToolCalls) > 0:
			var blocks []map[string]any
			for _, tc := range msg.ToolCalls {
				// Arguments are a JSON string; Claude wants a parsed object.
				var input map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input = map[string]any{"_raw": tc.Function.Arguments}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			out = append(out, map[string]any{
				"role":    "assistant",
				"content": blocks,
			})
			i++

		// ── Tool result message(s) → single user turn ─────────────────────────
		case msg.Role == "tool":
			// Collect all consecutive tool-result messages.
			var blocks []map[string]any
			for i < len(msgs) && msgs[i].Role == "tool" {
				blocks = append(blocks, map[string]any{
					"type":        "tool_result",
					"tool_use_id": msgs[i].ToolCallID,
					"content":     msgs[i].Content,
				})
				i++
			}
			out = append(out, map[string]any{
				"role":    "user",
				"content": blocks,
			})

		// ── Regular user / assistant text message ─────────────────────────────
		default:
			out = append(out, map[string]any{
				"role":    msg.Role,
				"content": msg.Content,
			})
			i++
		}
	}

	return out
}

// ── Streaming ─────────────────────────────────────────────────────────────────

// StreamBody asks for a stream. Anthropic reports usage in its events whether asked or not.
func (c *ClaudeClient) StreamBody(body map[string]any) map[string]any {
	body["stream"] = true
	return body
}

// ParseStream reads Anthropic's events into the same Response an ordinary call gets.
func (c *ClaudeClient) ParseStream(stream io.Reader, onDelta func(agent.StreamDelta)) (*agent.Response, error) {
	response := &agent.Response{}
	var text, thinking strings.Builder
	type block struct {
		kind, id, name string
		input          strings.Builder
	}
	blocks := map[int]*block{}
	order := []int{}
	var input, cacheRead, cacheWrite, output int
	usage := func() *agent.TokenUsage {
		prompt := input + cacheRead + cacheWrite
		return &agent.TokenUsage{PromptTokens: prompt, CachedPromptTokens: cacheRead, CompletionTokens: output, TotalTokens: prompt + output}
	}
	finished := false
	err := agent.EachEvent(stream, func(event, data string) error {
		switch event {
		case "message_start":
			var start struct {
				Message struct {
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						OutputTokens             int `json:"output_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &start); err != nil {
				return fmt.Errorf("failed to parse the stream: %w", err)
			}
			u := start.Message.Usage
			input, cacheRead, cacheWrite, output = u.InputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens, u.OutputTokens
			onDelta(agent.StreamDelta{Usage: usage()})
		case "content_block_start":
			var start struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &start); err != nil {
				return fmt.Errorf("failed to parse the stream: %w", err)
			}
			blocks[start.Index] = &block{kind: start.ContentBlock.Type, id: start.ContentBlock.ID, name: start.ContentBlock.Name}
			order = append(order, start.Index)
		case "content_block_delta":
			var piece struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &piece); err != nil {
				return fmt.Errorf("failed to parse the stream: %w", err)
			}
			switch piece.Delta.Type {
			case "text_delta":
				text.WriteString(piece.Delta.Text)
				onDelta(agent.StreamDelta{Text: piece.Delta.Text})
			case "thinking_delta":
				thinking.WriteString(piece.Delta.Thinking)
			case "input_json_delta":
				if b := blocks[piece.Index]; b != nil {
					b.input.WriteString(piece.Delta.PartialJSON)
				}
			}
		case "message_delta":
			var delta struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				return fmt.Errorf("failed to parse the stream: %w", err)
			}
			if delta.Delta.StopReason != "" {
				response.FinishReason = delta.Delta.StopReason
			}
			output = delta.Usage.OutputTokens
			onDelta(agent.StreamDelta{Usage: usage()})
		case "message_stop":
			finished = true
		case "error":
			var failure struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &failure); err != nil {
				return fmt.Errorf("failed to parse the stream: %w", err)
			}
			status := http.StatusBadGateway
			if failure.Error.Type == "overloaded_error" {
				status = 529
			}
			return &agent.StatusError{Status: status, Body: data}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !finished && response.FinishReason == "" {
		return nil, errors.New("the stream ended before the reply did: EOF")
	}
	response.Content, response.ReasoningContent, response.Usage = text.String(), thinking.String(), usage()
	for _, index := range order {
		b := blocks[index]
		if b.kind != "tool_use" {
			continue
		}
		arguments := b.input.String()
		if arguments == "" {
			arguments = "{}"
		}
		response.ToolCalls = append(response.ToolCalls, agent.ToolCall{
			ID: b.id, Type: "function",
			Function: agent.ToolCallFunction{Name: b.name, Arguments: arguments},
		})
	}
	return response, nil
}

// ── Response parsers ──────────────────────────────────────────────────────────

// ParseResponseFull extracts text, thinking and tool calls from an Anthropic
// response envelope. A reply can hold several text blocks, in order.
func (c *ClaudeClient) ParseResponseFull(body []byte) (*agent.Response, error) {
	var raw struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text,omitempty"`
			Thinking string          `json:"thinking,omitempty"`
			ID       string          `json:"id,omitempty"`
			Name     string          `json:"name,omitempty"`
			Input    json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w — body: %s", err, body)
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("Anthropic API error: %s — %s", raw.Error.Type, raw.Error.Message)
	}

	// Anthropic reports cache reads/writes separately from input_tokens.
	prompt := raw.Usage.InputTokens + raw.Usage.CacheReadInputTokens + raw.Usage.CacheCreationInputTokens
	result := &agent.Response{
		Usage: &agent.TokenUsage{
			PromptTokens:       prompt,
			CachedPromptTokens: raw.Usage.CacheReadInputTokens,
			CompletionTokens:   raw.Usage.OutputTokens,
			TotalTokens:        prompt + raw.Usage.OutputTokens,
		},
		FinishReason: raw.StopReason,
	}
	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text

		case "thinking":
			result.ReasoningContent += block.Thinking

		case "tool_use":
			// Input is a JSON object; serialise back to a JSON string so it
			// matches the ToolCallFunction.Arguments field (always a string).
			argsJSON, err := json.Marshal(block.Input)
			if err != nil {
				argsJSON = []byte("{}")
			}
			result.ToolCalls = append(result.ToolCalls, agent.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: agent.ToolCallFunction{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}
	return result, nil
}
