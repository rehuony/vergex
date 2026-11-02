package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"vergex/internal/agent"
)

// A thinking model's reasoning counts against the length of its reply, so a reply needs room
// for both. A planning round reads tens of thousands of tokens of market data, and reasons at
// length over them. Every provider takes the smaller limit, for a model that refuses this one.
const (
	replyLimit        = 16384
	smallerReplyLimit = 8192
)

// errReplyRanOut is a reply whose length went on reasoning, with nothing left of an answer.
var errReplyRanOut = errors.New("the model's reply ran out of length while it was reasoning")

// ClientAdapter reuses Vergex's configured provider client and transient reasoning.
// Credentials remain in Vergex's configured client and never enter model messages.
type ClientAdapter struct {
	Client    agent.Client
	TextTools bool
	turn      atomic.Uint64
}

func (a *ClientAdapter) Chat(ctx context.Context, messages []Message, tools []Tool) (Message, error) {
	wire := make([]agent.Message, 0, len(messages))
	for _, m := range messages {
		v := agent.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, ReasoningContent: m.ReasoningContent}
		for _, t := range m.ToolCalls {
			v.ToolCalls = append(v.ToolCalls, agent.ToolCall{ID: t.ID, Type: t.Type, Function: agent.ToolCallFunction{Name: t.Function.Name, Arguments: t.Function.Arguments}})
		}
		wire = append(wire, v)
	}
	defs := []agent.Tool{}
	for _, t := range tools {
		raw, e := json.Marshal(t.Function.Parameters)
		if e != nil {
			return Message{}, e
		}
		var schema map[string]any
		if e = json.Unmarshal(raw, &schema); e != nil {
			return Message{}, e
		}
		defs = append(defs, agent.Tool{Type: "function", Function: agent.FunctionDef{Name: t.Function.Name, Description: t.Function.Description, Parameters: schema}})
	}
	maxTokens := replyLimit
	request := &agent.Request{Ctx: ctx, Messages: wire, Tools: defs, MaxTokens: &maxTokens}
	if len(defs) > 0 {
		request.ToolChoice = "auto"
	}
	if a.TextTools {
		request.Messages = textToolMessages(wire, defs)
		request.Tools = nil
		request.ToolChoice = ""
	}
	result, e := a.Client.CallWithRequestFull(request)
	if refusedLimit(e) {
		maxTokens = smallerReplyLimit
		result, e = a.Client.CallWithRequestFull(request)
	}
	if e != nil {
		if ctx.Err() != nil {
			return Message{}, ctx.Err()
		}
		// Never expose wrapped transport URLs, authorization headers or
		// provider response bodies to the planning conversation.
		return Message{}, errors.New("The model request failed. Check the model provider key, balance and connectivity, then try again.")
	}
	if result == nil || len(result.Content) > 24000 || len(result.ToolCalls) > 5 || len(result.ReasoningContent) > 256<<10 {
		return Message{}, errors.New("invalid model response")
	}
	if result.Content == "" && len(result.ToolCalls) == 0 && (result.FinishReason == "length" || result.FinishReason == "max_tokens") {
		return Message{}, errReplyRanOut
	}
	if a.TextTools {
		if len(result.ToolCalls) != 0 {
			return Message{}, errors.New("unexpected native tool response on text-only model")
		}
		return parseTextTools(result.Content, tools, a.turn.Add(1))
	}
	out := Message{Role: "assistant", Content: result.Content, ReasoningContent: result.ReasoningContent}
	for _, t := range result.ToolCalls {
		call := ToolCall{ID: t.ID, Type: t.Type}
		call.Function.Name = t.Function.Name
		call.Function.Arguments = t.Function.Arguments
		out.ToolCalls = append(out.ToolCalls, call)
	}
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return Message{}, errors.New("The model did not return a final answer or a tool request")
	}
	return out, nil
}

// refusedLimit says a provider turned the request down for asking too long a reply.
func refusedLimit(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "status 400") && (strings.Contains(text, "max_tokens") || strings.Contains(text, "max_completion_tokens") || strings.Contains(text, "max_output"))
}
