package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"vergex/internal/agent"
)

type fakeClient struct {
	agent.Client
	req      *agent.Request
	response *agent.Response
	err      error
	calls    int
}

func (f *fakeClient) CallWithRequestFull(r *agent.Request) (*agent.Response, error) {
	f.req = r
	f.calls++
	return f.response, f.err
}
func TestClientAdapterPreservesToolRoundTripAndPrivateReasoning(t *testing.T) {
	f := &fakeClient{response: &agent.Response{Content: "answer", ReasoningContent: "private reasoning", ToolCalls: []agent.ToolCall{{ID: "call1", Type: "function", Function: agent.ToolCallFunction{Name: "read_market", Arguments: `{}`}}}}}
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
	for _, r := range []*agent.Response{nil, {}, {Content: strings.Repeat("x", 24001)}} {
		a := ClientAdapter{Client: &fakeClient{response: r}}
		if _, e := a.Chat(context.Background(), nil, nil); e == nil {
			t.Fatal("invalid response accepted")
		}
	}
}

func TestClientAdapterHidesProviderErrorDetails(t *testing.T) {
	f := &fakeClient{err: fmt.Errorf("wrapper secret url: %w", errors.New("https://secret.invalid/credential malformed HTTP response"))}
	a := ClientAdapter{Client: f}
	_, got := a.Chat(context.Background(), nil, nil)
	if got == nil || f.calls != 1 || strings.Contains(got.Error(), "secret") || strings.Contains(got.Error(), "https://") {
		t.Fatal("provider error detail leaked or call retried", got, f.calls)
	}
}

// refusingClient turns down any reply longer than it can give, as an older model does.
type refusingClient struct {
	agent.Client
	asked []int
}

func (c *refusingClient) CallWithRequestFull(r *agent.Request) (*agent.Response, error) {
	c.asked = append(c.asked, *r.MaxTokens)
	if *r.MaxTokens > smallerReplyLimit {
		return nil, errors.New(`API returned error (status 400): {"error":{"message":"Range of max_tokens should be [1, 8192]"}}`)
	}
	return &agent.Response{Content: "answer"}, nil
}

func TestClientAdapterAsksLessOfAModelThatRefusesTheLongerReply(t *testing.T) {
	c := &refusingClient{}
	out, e := (&ClientAdapter{Client: c}).Chat(context.Background(), nil, nil)
	if e != nil || out.Content != "answer" {
		t.Fatal(out, e)
	}
	if len(c.asked) != 2 || c.asked[0] != replyLimit || c.asked[1] != smallerReplyLimit {
		t.Fatal("unexpected reply limits", c.asked)
	}
}

func TestClientAdapterSaysWhenAReplyRanOutReasoning(t *testing.T) {
	for _, finish := range []string{"length", "max_tokens"} {
		a := ClientAdapter{Client: &fakeClient{response: &agent.Response{ReasoningContent: "still thinking", FinishReason: finish}}}
		if _, e := a.Chat(context.Background(), nil, nil); !errors.Is(e, errReplyRanOut) {
			t.Fatal(finish, e)
		}
	}
}
