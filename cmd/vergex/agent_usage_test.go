package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vergex/internal/agent"
	"vergex/internal/agent/pricing"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

// callModel records calls through a real client against a server that answers
// with status and body, and returns the row it stored.
func callModel(t *testing.T, st *store.Store, status int, body string) (*agent.Response, error, store.AgentCall) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	agent.CallCallback = recordAgentCalls(st, pricing.NewService(t.TempDir()))
	t.Cleanup(func() { agent.CallCallback = nil })

	client := agent.NewClientByProvider("deepseek", agent.WithMaxRetries(1), agent.WithHTTPClient(server.Client()))
	client.SetAPIKey("sk-secret", server.URL, "some-model")
	agent.TagUsage(client, agent.FeatureTrader, "t1")
	response, err := client.CallWithRequestFull(&agent.Request{Messages: []agent.Message{agent.NewUserMessage("hi")}})

	var call store.AgentCall
	if e := st.GormDB().Order("id DESC").First(&call).Error; e != nil {
		t.Fatalf("read call: %v", e)
	}
	return response, err, call
}

func TestAFailedCallIsRecordedWithItsTimeAndError(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	_, err, call := callModel(t, st, http.StatusUnauthorized, `{"error":{"message":"bad key sk-secret"}}`)
	if err == nil {
		t.Fatal("the call should fail")
	}
	if call.Status != store.CallFailed || call.DurationMs < 5 || call.Unpriced || call.CostUSD != 0 {
		t.Fatalf("call = %+v", call)
	}
	if call.Feature != agent.FeatureTrader || call.SourceID != "t1" {
		t.Fatalf("call belongs to %s/%s", call.Feature, call.SourceID)
	}
	if !strings.Contains(call.Error, "status 401") || strings.Contains(call.Error, "sk-secret") {
		t.Fatalf("error = %q", call.Error)
	}
}

func TestAWorkingCallKeepsItsTokensAndWhyItEnded(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	response, err, call := callModel(t, st, http.StatusOK, `{"choices":[{"message":{"content":"ok","reasoning_content":"because"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_cache_hit_tokens":4}}`)
	if err != nil || response.ReasoningContent != "because" {
		t.Fatalf("response = %+v, err %v", response, err)
	}
	if call.Status != store.CallOK || call.Error != "" || call.FinishReason != "stop" {
		t.Fatalf("call = %+v", call)
	}
	if call.PromptTokens != 10 || call.CachedPromptTokens != 4 || call.CompletionTokens != 2 {
		t.Fatalf("tokens = %+v", call)
	}
}
