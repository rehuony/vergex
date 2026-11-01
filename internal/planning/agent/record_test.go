package agent

import (
	"context"
	"encoding/json"
	"testing"

	"vergex/internal/planning/agent/model"
	"vergex/internal/planning/core"
)

// recordFixture asks for one tool, thinking aloud, then answers.
type recordFixture struct {
	loopFixture
	turn int
}

func (f *recordFixture) Chat(_ context.Context, _ []model.Message, tools []model.Tool) (model.Message, error) {
	f.turn++
	if f.turn > 1 || len(tools) == 0 {
		return model.Message{Role: "assistant", Content: "No trade.", ReasoningContent: "the spread is too wide"}, nil
	}
	call := model.ToolCall{ID: "call_1", Type: "function"}
	call.Function.Name = "get_market"
	call.Function.Arguments = "{}"
	return model.Message{Role: "assistant", ReasoningContent: "read the book first", ToolCalls: []model.ToolCall{call}}, nil
}

func TestATurnIsRecordedAsItHappens(t *testing.T) {
	f := &recordFixture{}
	type recorded struct {
		role    string
		message model.Message
	}
	var messages []recorded
	engine := Engine{
		Market: f, Charts: f, Model: f,
		Limits: core.Constraints{Equity: "1000", MaxRisk: "10", MaxNotional: "100", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"},
		Record: func(role string, message model.Message) { messages = append(messages, recorded{role, message}) },
	}
	input := Input{Message: "plan a trade", History: []History{{Role: "user", Content: "earlier"}}, Venue: "binance", Symbol: "BTCUSDT", Interval: "1h"}
	if err := engine.Run(context.Background(), input, func(Event) error { return nil }); err != nil {
		t.Fatal(err)
	}

	roles := map[string]int{}
	for _, m := range messages {
		roles[m.role]++
		if m.message.Content == "earlier" {
			t.Error("history already recorded by an earlier turn came back")
		}
		if m.message.Content == systemPrompt {
			t.Error("the fixed system prompt was recorded")
		}
	}
	if roles[RoleUser] != 1 || roles[RoleAssistant] != 2 || roles[RoleTool] != 1 || roles[RoleContext] < 3 {
		t.Fatalf("roles = %v", roles)
	}
	if first := messages[0]; first.role != RoleUser || first.message.Content != "plan a trade" {
		t.Fatalf("the user's own words should come first and alone, got %+v", first)
	}
	var workspace map[string]any
	if json.Unmarshal([]byte(messages[1].message.Content), &workspace) != nil || workspace["workspace"] == nil {
		t.Fatalf("the workspace should follow as context, got %+v", messages[1])
	}

	last := len(messages) - 1
	final, result, asked := messages[last], messages[last-1], messages[last-2]
	if asked.role != RoleAssistant || asked.message.ReasoningContent != "read the book first" || asked.message.ToolCalls[0].Function.Name != "get_market" {
		t.Fatalf("tool request = %+v", asked)
	}
	if result.role != RoleTool || result.message.ToolCallID != "call_1" {
		t.Fatalf("tool result = %+v", result)
	}
	if final.role != RoleAssistant || final.message.Content != "No trade." || final.message.ReasoningContent != "the spread is too wide" {
		t.Fatalf("answer = %+v", final)
	}
}
