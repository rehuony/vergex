package agent

import (
	"context"
	"testing"

	"vergex/internal/planning/agent/model"
	"vergex/internal/planning/core"
)

func TestDeferredEntriesDoNotSaveInvalidOrConflictingAssessments(t *testing.T) {
	for _, raw := range []string{
		`{"ideas":[],"deferred":[{"kind":"stop_market","reason":"Watch"}]}`,
		`{"ideas":[],"deferred":[{"kind":"market","reason":"Watch"},{"kind":"market","reason":"Other"}]}`,
		`{"ideas":[],"deferred":[{"kind":"limit","reason":" "}]}`,
		`{"ideas":[{"side":"long","execution":{"kind":"limit"}}],"deferred":[{"kind":"limit","reason":"Watch"}]}`,
	} {
		store := &textDraftFixture{}
		engine := Engine{Store: store, Market: textMarketFixture{}}
		call := model.ToolCall{}
		call.Function.Name, call.Function.Arguments = "propose_plans", raw
		proposed := false
		_, err := engine.invoke(context.Background(), Input{}, core.Constraints{}, nil, call, &proposed, func(Event) error { return nil })
		if err == nil || proposed || len(store.plans) != 0 {
			t.Fatalf("invalid assessment accepted: %v", err)
		}
	}
}
