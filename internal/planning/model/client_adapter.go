package model

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"vergex/internal/llm"
)

// ClientAdapter reuses Vergex's configured provider client and transient reasoning.
// Credentials remain in Vergex's configured client and never enter model messages.
type ClientAdapter struct {
	Client    llm.AIClient
	TextTools bool
	turn      atomic.Uint64
}

func (a *ClientAdapter) Chat(ctx context.Context, messages []Message, tools []Tool) (Message, error) {
	wire := make([]llm.Message, 0, len(messages))
	for _, m := range messages {
		v := llm.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, ReasoningContent: m.ReasoningContent}
		for _, t := range m.ToolCalls {
			v.ToolCalls = append(v.ToolCalls, llm.ToolCall{ID: t.ID, Type: t.Type, Function: llm.ToolCallFunction{Name: t.Function.Name, Arguments: t.Function.Arguments}})
		}
		wire = append(wire, v)
	}
	defs := []llm.Tool{}
	for _, t := range tools {
		raw, e := json.Marshal(t.Function.Parameters)
		if e != nil {
			return Message{}, e
		}
		var schema map[string]any
		if e = json.Unmarshal(raw, &schema); e != nil {
			return Message{}, e
		}
		defs = append(defs, llm.Tool{Type: "function", Function: llm.FunctionDef{Name: t.Function.Name, Description: t.Function.Description, Parameters: schema}})
	}
	maxTokens := 8192
	request := &llm.Request{Ctx: ctx, Messages: wire, Tools: defs, MaxTokens: &maxTokens}
	if len(defs) > 0 {
		request.ToolChoice = "auto"
	}
	if a.TextTools {
		request.Messages = textToolMessages(wire, defs)
		request.Tools = nil
		request.ToolChoice = ""
	}
	result, e := a.Client.CallWithRequestFull(request)
	if e != nil {
		if ctx.Err() != nil {
			return Message{}, ctx.Err()
		}
		// Never expose wrapped transport URLs, authorization headers or
		// provider response bodies to the planning conversation.
		return Message{}, errors.New("Vergex AI request failed. Check the AI provider key, balance and connectivity, then try again.")
	}
	if result == nil || len(result.Content) > 24000 || len(result.ToolCalls) > 5 || len(result.ReasoningContent) > 256<<10 {
		return Message{}, errors.New("invalid Vergex AI response")
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
		return Message{}, errors.New("Vergex AI did not return a final answer or a tool request")
	}
	return out, nil
}
