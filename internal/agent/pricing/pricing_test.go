package pricing

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"vergex/internal/database/testdb"
)

const sampleCatalog = `{
  "deepseek": {"models": {"deepseek-v4-pro": {"cost": {"input": 0.66, "output": 1.98, "cache_read": 0.022}}}},
  "anthropic": {"models": {"claude-opus-4-6": {"cost": {"input": 5, "output": 25, "cache_read": 0.5}}}},
  "alibaba": {"models": {"qwen3-max": {"cost": {"input": 1.2, "output": 6}}}},
  "alibaba-cn": {"models": {"qwen3-max": {"cost": {"input": 0.86, "output": 3.44}}}},
  "openai": {"models": {"gpt-5.4": {"cost": {"input": 2.5, "output": 15, "context_over_200k": {"input": 5, "output": 22.5}}}}},
  "unrelated": {"models": {"x": {"cost": {"input": 1, "output": 1}}}}
}`

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseCatalogKeepsSupportedProvidersOnly(t *testing.T) {
	catalog, err := parseCatalog([]byte(sampleCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog["unrelated"]; ok {
		t.Fatal("unrelated provider should be dropped")
	}
	if _, ok := catalog.Lookup("claude", "", "claude-opus-4-6"); !ok {
		t.Fatal("claude should map to anthropic")
	}
}

func TestLookupRoutesByBaseURL(t *testing.T) {
	catalog, _ := parseCatalog([]byte(sampleCatalog))

	cn, ok := catalog.Lookup("qwen", "", "qwen3-max")
	if !ok || cn.Input != 0.86 {
		t.Fatalf("default qwen should use alibaba-cn, got %+v %v", cn, ok)
	}
	intl, ok := catalog.Lookup("qwen", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", "qwen3-max")
	if !ok || intl.Input != 1.2 {
		t.Fatalf("intl qwen should use alibaba, got %+v %v", intl, ok)
	}
	if _, ok := catalog.Lookup("openai", "https://openrouter.ai/api/v1", "gpt-5.4"); ok {
		t.Fatal("custom base URL must stay unpriced")
	}
	if _, ok := catalog.Lookup("deepseek", "", "deepseek-chat"); ok {
		t.Fatal("unknown model must stay unpriced")
	}
	if _, ok := catalog.Lookup("DeepSeek", "https://api.deepseek.com/v1", "DEEPSEEK-V4-PRO"); !ok {
		t.Fatal("lookup should be case-insensitive")
	}
}

func TestEstimateUsesCacheReadAndLongContext(t *testing.T) {
	catalog, _ := parseCatalog([]byte(sampleCatalog))

	ds, _ := catalog.Lookup("deepseek", "", "deepseek-v4-pro")
	got := ds.Estimate(Usage{PromptTokens: 1_000_000, CachedPromptTokens: 400_000, CompletionTokens: 100_000})
	approx(t, got, 0.6*0.66+0.4*0.022+0.1*1.98)

	gpt, _ := catalog.Lookup("openai", "", "gpt-5.4")
	approx(t, gpt.Estimate(Usage{PromptTokens: 100_000, CompletionTokens: 1000}), (100_000*2.5+1000*15)/1e6)
	approx(t, gpt.Estimate(Usage{PromptTokens: 300_000, CompletionTokens: 1000}), (300_000*5+1000*22.5)/1e6)

	q, _ := catalog.Lookup("qwen", "", "qwen3-max")
	approx(t, q.Estimate(Usage{PromptTokens: 1000, CachedPromptTokens: 500}), 1000*0.86/1e6)
}

func TestRefreshWritesCacheAndKeepsOldOnFailure(t *testing.T) {
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(sampleCatalog))
	}))
	defer srv.Close()

	db := testdb.Open(t)
	svc := NewService(db)
	svc.sourceURL = srv.URL
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	fail = true
	if err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}
	if _, ok := svc.Estimate("claude", "", "claude-opus-4-6", Usage{PromptTokens: 1}); !ok {
		t.Fatal("previous catalog should be kept after a failed refresh")
	}

	reloaded := NewService(db)
	if err := reloaded.LoadCache(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Lookup("deepseek", "", "deepseek-v4-pro"); !ok {
		t.Fatal("cache should contain the downloaded catalog")
	}
}
