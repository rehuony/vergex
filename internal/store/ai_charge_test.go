package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestAIChargeStore(t *testing.T) *AIChargeStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := NewAIChargeStore(db)
	if err := s.initTables(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAIChargeSummaryCountsTokensAndUnpricedCalls(t *testing.T) {
	s := newTestAIChargeStore(t)
	records := []*AICharge{
		{TraderID: "t1", Model: "deepseek-v4-pro", Provider: "deepseek", PromptTokens: 1000, CompletionTokens: 100, CostUSD: 0.002},
		{TraderID: "t1", Model: "deepseek-v4-pro", Provider: "deepseek", PromptTokens: 2000, CompletionTokens: 200, CostUSD: 0.004},
		{TraderID: "t2", Model: "my-custom", Provider: "openai", PromptTokens: 500, CompletionTokens: 50, CostUSD: 9, Unpriced: true},
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

	charges, total, err := s.GetCharges("t1", "all")
	if err != nil || len(charges) != 2 || total < 0.0059 || total > 0.0061 {
		t.Fatalf("GetCharges = %d records, total %v, err %v", len(charges), total, err)
	}
}
