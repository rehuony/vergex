package provider

import (
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
