package provider

import (
	"errors"
	"strings"
	"testing"

	"vergex/internal/agent"
)

func TestClaudeKeepsThinkingAndEveryTextBlock(t *testing.T) {
	client := NewClaudeClientWithOptions().(agent.ClientEmbedder).Base().Hooks
	response, err := client.ParseResponseFull([]byte(`{
		"content": [
			{"type": "thinking", "thinking": "weigh it"},
			{"type": "text", "text": "first "},
			{"type": "text", "text": "second"}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 2, "cache_read_input_tokens": 4}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "first second" || response.ReasoningContent != "weigh it" || response.FinishReason != "end_turn" {
		t.Fatalf("response = %+v", response)
	}
	if usage := response.Usage; usage.PromptTokens != 14 || usage.CachedPromptTokens != 4 || usage.TotalTokens != 16 {
		t.Fatalf("usage = %+v", usage)
	}
}

// A request's own limit has to reach the wire, or the Futures assistant's room for long tool
// arguments, and its fallback when a model refuses that much, would never apply to Claude.
func TestClaudeUsesARequestsOwnReplyLimit(t *testing.T) {
	client := NewClaudeClientWithOptions(agent.WithMaxTokens(8000)).(agent.ClientEmbedder).Base().Hooks
	messages := []agent.Message{agent.NewUserMessage("hello")}

	body := client.BuildRequestBodyFromRequest(&agent.Request{Model: "claude-test", Messages: messages})
	if body["max_tokens"] != 8000 {
		t.Fatalf("with no limit of its own: max_tokens = %v", body["max_tokens"])
	}
	limit := 16384
	body = client.BuildRequestBodyFromRequest(&agent.Request{Model: "claude-test", Messages: messages, MaxTokens: &limit})
	if body["max_tokens"] != 16384 {
		t.Fatalf("with a limit of its own: max_tokens = %v", body["max_tokens"])
	}
}

// A base URL typed with a slash on the end has to reach the same place as one without.
func TestABaseURLWithATrailingSlashBuildsTheSameURL(t *testing.T) {
	for _, name := range []string{"openai", "claude", "deepseek"} {
		client := agent.NewClientByProvider(name)
		client.SetAPIKey("key", "https://example.com/v1", "")
		plain := client.(agent.ClientEmbedder).Base().Hooks.BuildUrl()
		client.SetAPIKey("key", "https://example.com/v1/", "")
		if got := client.(agent.ClientEmbedder).Base().Hooks.BuildUrl(); got != plain || strings.Contains(strings.TrimPrefix(got, "https://"), "//") {
			t.Errorf("%s: %q, want %q", name, got, plain)
		}
	}
}

func TestClaudeStreamReadsLikeAnOrdinaryReply(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":4,"output_tokens":1}}}

event: ping
data: {"type":"ping"}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"weigh it"}}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hel"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"lo"}}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"t1","name":"draw"}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"x\""}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":":1}"}}

event: content_block_start
data: {"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"t2","name":"read"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`
	client := NewClaudeClientWithOptions().(agent.ClientEmbedder).Base().Hooks
	var seen strings.Builder
	var last *agent.TokenUsage
	response, err := client.ParseStream(strings.NewReader(stream), func(delta agent.StreamDelta) {
		seen.WriteString(delta.Text)
		if delta.Usage != nil {
			last = delta.Usage
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen.String() != "Hello" || response.Content != "Hello" || response.ReasoningContent != "weigh it" || response.FinishReason != "tool_use" {
		t.Fatalf("seen=%q response=%+v", seen.String(), response)
	}
	if usage := response.Usage; usage.PromptTokens != 14 || usage.CachedPromptTokens != 4 || usage.CompletionTokens != 7 {
		t.Fatalf("usage = %+v", usage)
	}
	if last == nil || last.CompletionTokens != 7 {
		t.Fatalf("usage heard along the way = %+v", last)
	}
	if len(response.ToolCalls) != 2 || response.ToolCalls[0].Function.Arguments != `{"x":1}` || response.ToolCalls[0].ID != "t1" {
		t.Fatalf("tool calls = %+v", response.ToolCalls)
	}
	// A tool with no arguments still has to send valid JSON back next turn.
	if response.ToolCalls[1].Function.Arguments != "{}" {
		t.Fatalf("arguments = %q", response.ToolCalls[1].Function.Arguments)
	}
}

func TestClaudeStreamReportsAnErrorItCarries(t *testing.T) {
	stream := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"
	client := NewClaudeClientWithOptions().(agent.ClientEmbedder).Base().Hooks
	_, err := client.ParseStream(strings.NewReader(stream), func(agent.StreamDelta) {})
	var refused *agent.StatusError
	if !errors.As(err, &refused) || refused.Status != 529 {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeAsksForAStreamWithoutOpenAIsOptions(t *testing.T) {
	client := NewClaudeClientWithOptions().(agent.ClientEmbedder).Base().Hooks
	body := client.StreamBody(map[string]any{})
	if body["stream"] != true || body["stream_options"] != nil {
		t.Fatalf("body = %v", body)
	}
}
