package agent

import (
	"testing"

	"vergex/internal/database/testdb"
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
