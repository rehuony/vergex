package agent

import (
	"testing"
	"time"

	"vergex/internal/database/testdb"

	"github.com/google/uuid"
)

func newTestAgentCallStore(t *testing.T) *CallStore {
	t.Helper()
	return NewCallStore(testdb.Open(t))
}

func TestAgentCallSummaryCountsTokensAndUnpricedCalls(t *testing.T) {
	s := newTestAgentCallStore(t)
	records := []*Call{
		{Feature: FeatureTrader, SourceID: "t1", Model: "deepseek-v4-pro", Provider: "deepseek", PromptTokens: 1000, CompletionTokens: 100, CostUSD: 0.002},
		{Feature: FeatureTrader, SourceID: "t1", Model: "deepseek-v4-pro", Provider: "deepseek", PromptTokens: 2000, CompletionTokens: 200, CostUSD: 0.004},
		{Feature: FeatureTrader, SourceID: "t2", Model: "my-custom", Provider: "openai", PromptTokens: 500, CompletionTokens: 50, CostUSD: 9, Unpriced: true},
	}
	for _, r := range records {
		if err := s.Record(r); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := s.GetSummary("", "all")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 3 || summary.UnpricedCount != 1 {
		t.Fatalf("count=%d unpriced=%d", summary.Count, summary.UnpricedCount)
	}
	if summary.PromptTokens != 3500 || summary.CompletionTokens != 350 {
		t.Fatalf("tokens = %d/%d", summary.PromptTokens, summary.CompletionTokens)
	}
	if summary.TotalUSD < 0.0059 || summary.TotalUSD > 0.0061 {
		t.Fatalf("unpriced calls must not add cost, total = %v", summary.TotalUSD)
	}
	if got := summary.ByModel["my-custom"]; got.UnpricedCount != 1 || got.TotalUSD != 0 {
		t.Fatalf("custom model summary = %+v", got)
	}

	one, err := s.GetSummary("t1", "all")
	if err != nil {
		t.Fatal(err)
	}
	if one.Count != 2 || len(one.ByModel) != 1 {
		t.Fatalf("trader summary = %+v", one)
	}

	calls, total, err := s.ForTrader("t1", "all")
	if err != nil || len(calls) != 2 || total < 0.0059 || total > 0.0061 {
		t.Fatalf("ForTrader = %d records, total %v, err %v", len(calls), total, err)
	}
}

func TestCallsAddUpForAConversationFromAMoment(t *testing.T) {
	db := testdb.Open(t)
	store := NewCallStore(db)
	conversations := NewConversationStore(db)
	userID := testdb.AddUser(t, db)
	conversation := Conversation{ID: uuid.NewString(), UserID: userID, Feature: FeatureFutures, Title: "t"}
	if err := conversations.Open(&conversation); err != nil {
		t.Fatal(err)
	}
	record := func(call Call) {
		t.Helper()
		call.ConversationID = conversation.ID
		call.Feature = FeatureFutures
		if call.Status == "" {
			call.Status = CallOK
		}
		if err := store.Record(&call); err != nil {
			t.Fatal(err)
		}
	}
	record(Call{Model: "old", PromptTokens: 1000, CostUSD: 1})
	time.Sleep(5 * time.Millisecond)
	since := time.Now()
	time.Sleep(5 * time.Millisecond)
	record(Call{Model: "m", PromptTokens: 100, CompletionTokens: 10, CostUSD: 0.5, DurationMs: 30, FinishReason: "tool_calls"})
	record(Call{Model: "m", PromptTokens: 200, CompletionTokens: 20, Unpriced: true, DurationMs: 40, FinishReason: "length"})
	record(Call{Model: "m", Status: CallFailed, Error: "stopped"})

	totals, err := store.ForConversation(conversation.ID, since)
	if err != nil {
		t.Fatal(err)
	}
	want := CallTotals{Model: "m", Calls: 3, Failed: 1, PromptTokens: 300, CompletionTokens: 30, CostUSD: 0.5, Unpriced: true, DurationMs: 70}
	if totals != want {
		t.Fatalf("totals = %+v, want %+v", totals, want)
	}
	all, err := store.ForConversation(conversation.ID, time.Time{})
	if err != nil || all.Calls != 4 || all.PromptTokens != 1300 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if empty, err := store.ForConversation("", time.Time{}); err != nil || empty.Calls != 0 {
		t.Fatalf("no conversation = %+v, %v", empty, err)
	}
}

func TestASummaryTellsCallsApartByWhatMadeThemAndSaysWhatFailed(t *testing.T) {
	s := newTestAgentCallStore(t)
	now := time.Now()
	for _, call := range []*Call{
		{Feature: FeatureTrader, SourceID: "t1", Model: "m1", Status: CallOK, PromptTokens: 10, CompletionTokens: 1, CostUSD: 1},
		{Feature: FeatureFutures, Model: "m1", Status: CallOK, PromptTokens: 20, CompletionTokens: 2, CostUSD: 2},
		{Feature: FeatureFutures, Model: "m2", Status: CallFailed, Error: "HTTP 400: no such model"},
		{Feature: FeatureTelegram, Model: "m1", Status: CallOK, PromptTokens: 30, CompletionTokens: 3, CostUSD: 3},
	} {
		call.CreatedAt = now
		if err := s.Record(call); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.GetSummary("", "all")
	if err != nil {
		t.Fatal(err)
	}
	if all.Count != 4 || all.FailedCount != 1 || all.TotalUSD != 6 {
		t.Fatalf("summary = %+v", all)
	}
	if all.ByModel["m1"].Count != 3 || all.ByModel["m2"].FailedCount != 1 {
		t.Fatalf("by model = %+v", all.ByModel)
	}
	if f := all.ByFeature[FeatureFutures]; f.Count != 2 || f.FailedCount != 1 || f.TotalUSD != 2 {
		t.Fatalf("by feature = %+v", all.ByFeature)
	}
	if len(all.RecentErrors) != 1 || all.RecentErrors[0].Error != "HTTP 400: no such model" {
		t.Fatalf("errors = %+v", all.RecentErrors)
	}

	// One trader sees only its own.
	one, err := s.GetSummary("t1", "all")
	if err != nil {
		t.Fatal(err)
	}
	if one.Count != 1 || one.FailedCount != 0 || len(one.RecentErrors) != 0 || len(one.ByFeature) != 1 {
		t.Fatalf("one trader = %+v", one)
	}
}
