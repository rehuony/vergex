package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"vergex/internal/futures/plan"
)

// Budget bounds user-initiated deliberation, not the number of risk failures.
// The final model round is reserved for a tool-free explanation.
type Budget struct {
	MaxRounds    int `json:"max_rounds"`
	MaxToolCalls int `json:"max_tool_calls"`
}

// Full immutable plans are persisted and streamed to the UI. Model follow-up
// needs prices, allocations and validation outcomes, not repeated book depth
// and analysis windows in every saved plan.
func modelToolResult(name string, result any) any {
	if name != "propose_plans" {
		return result
	}
	data, ok := result.(map[string]any)
	if !ok {
		return result
	}
	plans, ok := data["plans"].([]plan.Plan)
	if !ok {
		return result
	}
	copyData := make(map[string]any, len(data))
	for key, value := range data {
		copyData[key] = value
	}
	summaries := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		summaries = append(summaries, map[string]any{
			"id": plan.ID, "idea": plan.Idea, "quantity": plan.Quantity,
			"risk": plan.Risk, "reward": plan.Reward, "reward_risk": plan.RewardRisk,
			"orders": plan.Orders, "quote_asset": plan.Snapshot.QuoteAsset,
			"venue": plan.Snapshot.Venue, "symbol": plan.Snapshot.Symbol,
			"expires": plan.Expires,
		})
	}
	copyData["plans"] = summaries
	return copyData
}

func (b Budget) normalized() (Budget, error) {
	if b.MaxRounds == 0 {
		b.MaxRounds = 8
	}
	if b.MaxToolCalls == 0 {
		b.MaxToolCalls = 24
	}
	if b.MaxRounds < 2 || b.MaxRounds > 12 || b.MaxToolCalls < 1 || b.MaxToolCalls > 40 {
		return b, errors.New("invalid harness resource budget")
	}
	return b, nil
}

type outcome struct {
	Drafts     int      `json:"drafts"`
	Rejections []string `json:"rejections"`
	StopReason string   `json:"stop_reason"`
	Waiting    bool     `json:"waiting,omitempty"`
}

func (o outcome) summary(code string) string {
	l, ok := languages[code]
	if !ok {
		l = languages["en"]
	}
	if o.Drafts > 0 {
		return fmt.Sprintf(l.saved, o.Drafts)
	}
	if o.Waiting {
		return l.waiting
	}
	text := l.noTrade
	if len(o.Rejections) > 0 {
		text += "\n" + l.checked + "\n" + strings.Join(o.Rejections[max(0, len(o.Rejections)-4):], "\n")
	}
	if o.StopReason == "model_unavailable" {
		text += "\n" + l.modelUnavailable
	}
	if o.StopReason == "budget" {
		text += "\n" + l.outOfBudget
	}
	return text
}

// The roles Engine.Record receives. RoleContext is data sent with a turn that the
// user did not type, though the model reads it as a user message.
const (
	RoleSystem    = "system"
	RoleContext   = "context"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

func (e *Engine) record(role string, message Message) {
	if e.Record != nil {
		e.Record(role, message)
	}
}

func (e *Engine) finishTurn(ctx context.Context, messages []Message, result outcome, code string, emit Emit, explain bool) error {
	if explain && ctx.Err() == nil {
		prompt := Message{Role: "user", Content: "Planning is finished. Briefly explain validated results, assumptions and rejection reasons. Do not call tools, promise profit or claim approval/execution. If no draft passed, explicitly state no trade. " + replyLanguage(code) + "."}
		messages = append(messages, prompt)
		e.record(RoleContext, prompt)
		answer, err := chatWithProgress(ctx, e.Model, messages, nil, emit)
		if err == nil && len(answer.ToolCalls) == 0 && answer.Content != "" {
			e.record(RoleAssistant, answer)
			if err = emit(Event{"assistant", map[string]string{"message": answer.Content}}); err != nil {
				return err
			}
			return emit(Event{"outcome", result})
		}
	}
	// The user reads this as the answer, so it counts as one.
	summary := result.summary(code)
	e.record(RoleAssistant, Message{Role: "assistant", Content: summary})
	if err := emit(Event{"assistant", map[string]string{"message": summary}}); err != nil {
		return err
	}
	return emit(Event{"outcome", result})
}

// Remove stale read payloads while keeping assistant-call/tool-result pairing.
// Validation feedback is never removed here; exact plan authority stays outside model context.
func compactToolContext(messages []Message) []Message {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	if total < 64000 {
		return messages
	}
	copyMessages := append([]Message(nil), messages...)
	names := map[string]string{}
	for _, m := range messages {
		for _, call := range m.ToolCalls {
			names[call.ID] = call.Function.Name
		}
	}
	keep := 0
	for n := len(copyMessages) - 1; n >= 0; n-- {
		m := &copyMessages[n]
		if m.Role != "tool" {
			continue
		}
		name := names[m.ToolCallID]
		if name != "get_market" && name != "get_candles" {
			continue
		}
		keep++
		if keep > 2 {
			m.Content = "Prior market payload was compacted. Historical prices are not current quotes. Draft compilation and execution must validate a fresh order book."
		}
	}
	return copyMessages
}
