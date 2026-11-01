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
	"fmt"
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultClaudeBaseURL = "https://api.anthropic.com/v1"
	DefaultClaudeModel   = "claude-opus-4-6"
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
	return fmt.Sprintf("%s/messages", c.BaseURL)
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
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": c.MaxTokens,
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
