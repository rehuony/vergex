package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"vergex/internal/llm"
	"vergex/internal/store"
)

// fakeModelClient records the settings a model test used and answers with err.
type fakeModelClient struct {
	key, url, model string
	err             error
}

func (f *fakeModelClient) SetAPIKey(key, url, model string) { f.key, f.url, f.model = key, url, model }
func (f *fakeModelClient) SetTimeout(time.Duration)         {}
func (f *fakeModelClient) CallWithMessages(string, string) (string, error) {
	return "OK", f.err
}
func (f *fakeModelClient) CallWithRequest(*llm.Request) (string, error) { return "OK", f.err }
func (f *fakeModelClient) CallWithRequestStream(*llm.Request, func(string)) (string, error) {
	return "OK", f.err
}
func (f *fakeModelClient) CallWithRequestFull(*llm.Request) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: "OK"}, f.err
}

func TestModelTestUsesFormValuesAndSavedKey(t *testing.T) {
	// The handler reads config for transport encryption (off by default).
	t.Setenv("JWT_SECRET", strings.Repeat("model-test-secret-", 4))
	st, err := store.New(t.TempDir() + "/vergex.db")
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.AIModel().Create("owner", "owner_deepseek", "DeepSeek", "deepseek", true, "sk-saved-key", ""); err != nil {
		t.Fatalf("create model: %v", err)
	}

	fake := &fakeModelClient{}
	var provider string
	orig := newModelTestClient
	newModelTestClient = func(p string) llm.AIClient { provider = p; return fake }
	t.Cleanup(func() { newModelTestClient = orig })

	s := &Server{store: st}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "owner") })
	r.POST("/models/test", s.handleTestModelConfig)
	post := func(body string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/models/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	// Saved model, blank key: the saved key and the default model are used.
	code, out := post(`{"model_id":"owner_deepseek","api_key":""}`)
	if code != http.StatusOK || out["ok"] != true || out["model"] != llm.DefaultDeepSeekModel {
		t.Fatalf("saved-key test = %d %v", code, out)
	}
	if provider != "deepseek" || fake.key != "sk-saved-key" {
		t.Fatalf("called %q with key %q, want deepseek with the saved key", provider, fake.key)
	}

	// New provider: the typed key and model name are used, nothing is saved.
	code, out = post(`{"model_id":"openai","api_key":"sk-typed","custom_model_name":"gpt-test"}`)
	if code != http.StatusOK || out["ok"] != true || out["model"] != "gpt-test" {
		t.Fatalf("new-provider test = %d %v", code, out)
	}
	if provider != "openai" || fake.key != "sk-typed" || fake.model != "gpt-test" {
		t.Fatalf("called %q key %q model %q", provider, fake.key, fake.model)
	}
	if models, _ := st.AIModel().List("owner"); len(models) != 1 {
		t.Fatalf("test saved a model: %d rows", len(models))
	}

	// A provider error is reported as ok=false with its message, key masked.
	fake.err = errors.New(`API returned error (status 401): {"error":{"message":"Incorrect API key provided: sk-typed","type":"invalid_request_error"}}`)
	code, out = post(`{"model_id":"openai","api_key":"sk-typed"}`)
	if code != http.StatusOK || out["ok"] != false || out["error"] != "HTTP 401: Incorrect API key provided: ***" {
		t.Fatalf("failed test = %d %v", code, out)
	}

	for _, body := range []string{
		`{"model_id":"nope","api_key":"k"}`,
		`{"model_id":"openai","api_key":""}`,
		`{"model_id":"openai","api_key":"k","custom_api_url":"http://127.0.0.1:9/v1"}`,
	} {
		if code, _ := post(body); code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", body, code)
		}
	}
}
