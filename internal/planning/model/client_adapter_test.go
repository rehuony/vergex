package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"vergex/internal/llm"
)

type fakeAIClient struct {
	llm.AIClient
	req      *llm.Request
	response *llm.LLMResponse
	err      error
	calls    int
}

func (f *fakeAIClient) CallWithRequestFull(r *llm.Request) (*llm.LLMResponse, error) {
	f.req = r
	f.calls++
	return f.response, f.err
}
func TestClientAdapterPreservesToolRoundTripAndPrivateReasoning(t *testing.T) {
	f := &fakeAIClient{response: &llm.LLMResponse{Content: "answer", ReasoningContent: "private reasoning", ToolCalls: []llm.ToolCall{{ID: "call1", Type: "function", Function: llm.ToolCallFunction{Name: "read_market", Arguments: `{}`}}}}}
	a := ClientAdapter{Client: f}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, e := a.Chat(ctx, []Message{{Role: "assistant", ReasoningContent: "prior private"}, {Role: "tool", ToolCallID: "call0", Content: "observed"}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if f.req.Ctx != ctx || f.req.Messages[0].ReasoningContent != "prior private" || f.req.Messages[1].ToolCallID != "call0" || out.ToolCalls[0].ID != "call1" {
		t.Fatal("tool protocol lost")
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "private reasoning") {
		t.Fatal("reasoning leaked to persisted chat")
	}
}
func TestClientAdapterRejectsEmptyAndOversizedResponses(t *testing.T) {
	for _, r := range []*llm.LLMResponse{nil, {}, {Content: strings.Repeat("x", 24001)}} {
		a := ClientAdapter{Client: &fakeAIClient{response: r}}
		if _, e := a.Chat(context.Background(), nil, nil); e == nil {
			t.Fatal("invalid response accepted")
		}
	}
}

func TestClientAdapterHidesProviderErrorDetails(t *testing.T) {
	f := &fakeAIClient{err: fmt.Errorf("wrapper secret url: %w", errors.New("https://secret.invalid/credential malformed HTTP response"))}
	a := ClientAdapter{Client: f}
	_, got := a.Chat(context.Background(), nil, nil)
	if got == nil || f.calls != 1 || strings.Contains(got.Error(), "secret") || strings.Contains(got.Error(), "https://") {
		t.Fatal("provider error detail leaked or call retried", got, f.calls)
	}
}
