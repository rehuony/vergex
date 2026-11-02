package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"vergex/internal/agent"
)

func TestListModelsAsksEachProviderInItsOwnWay(t *testing.T) {
	var path, bearer, apiKey string
	answer, status := "", http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, bearer, apiKey = r.URL.RequestURI(), r.Header.Get("Authorization"), r.Header.Get("x-api-key")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer server.Close()
	list := func(provider, url string) ([]string, error) {
		// The real client refuses to reach a private address such as the fixture's.
		client := agent.NewClientByProvider(provider, agent.WithHTTPClient(server.Client()))
		client.SetAPIKey("sk-test", url, "")
		return agent.ListModels(context.Background(), client)
	}

	answer = `{"object":"list","data":[{"id":"gpt-b"},{"id":"text-embedding-3-small"},{"id":"gpt-a"},{"id":"whisper-1"},{"id":"gpt-image-1"},{"id":"gpt-a"}]}`
	got, err := list(agent.ProviderOpenAI, server.URL+"/v1/")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"gpt-a", "gpt-b"}) {
		t.Errorf("names = %v", got)
	}
	if path != "/v1/models" || bearer != "Bearer sk-test" {
		t.Errorf("asked %s with %q", path, bearer)
	}

	answer = `{"data":[{"id":"claude-a","type":"model"}],"has_more":false}`
	if got, err = list(agent.ProviderClaude, server.URL); err != nil || !slices.Equal(got, []string{"claude-a"}) {
		t.Errorf("claude = %v, err %v", got, err)
	}
	if path != "/models?limit=1000" || apiKey != "sk-test" {
		t.Errorf("asked %s with key %q", path, apiKey)
	}

	answer = `{"data":[{"id":"models/gemini-a"}]}`
	if got, err = list(agent.ProviderGemini, server.URL); err != nil || !slices.Equal(got, []string{"gemini-a"}) {
		t.Errorf("gemini = %v, err %v", got, err)
	}

	answer, status = `{"error":{"message":"bad key"}}`, http.StatusUnauthorized
	if _, err = list(agent.ProviderDeepSeek, server.URL); err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("err = %v", err)
	}
}

func TestListModelsHasNothingToAskOnAFullURL(t *testing.T) {
	client := agent.NewClientByProvider(agent.ProviderOpenAI)
	client.SetAPIKey("sk-test", "https://api.example.com/v2/chat#", "")
	got, err := agent.ListModels(context.Background(), client)
	if got != nil || err != nil {
		t.Errorf("names = %v, err %v", got, err)
	}
}
