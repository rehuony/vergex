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
	"vergex/internal/agent"
	"vergex/internal/database/testdb"
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
func (f *fakeModelClient) CallWithRequest(*agent.Request) (string, error) { return "OK", f.err }
func (f *fakeModelClient) CallWithRequestFull(*agent.Request) (*agent.Response, error) {
	return &agent.Response{Content: "OK"}, f.err
}

func TestModelTestUsesFormValuesAndSavedKey(t *testing.T) {
	// The handler reads config for transport encryption (off by default).
	t.Setenv("JWT_SECRET", strings.Repeat("model-test-secret-", 4))
	st := NewStores(testdb.Open(t))
	owner := testdb.AddUser(t, st.DB)
	if err := st.Models.Save(owner, "deepseek", "DeepSeek", true, "sk-saved-key", "", ""); err != nil {
		t.Fatalf("create model: %v", err)
	}
	saved, err := st.Models.Find(owner, "deepseek")
	if err != nil {
		t.Fatalf("find model: %v", err)
	}

	fake := &fakeModelClient{}
	var provider string
	orig := newModelTestClient
	newModelTestClient = func(p string) agent.Client { provider = p; return fake }
	t.Cleanup(func() { newModelTestClient = orig })

	s := &Server{stores: st}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", owner) })
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
	code, out := post(`{"agent_model_id":"` + saved.ID + `","api_key":""}`)
	if code != http.StatusOK || out["ok"] != true || out["model"] != agent.DefaultDeepSeekModel {
		t.Fatalf("saved-key test = %d %v", code, out)
	}
	if provider != "deepseek" || fake.key != "sk-saved-key" {
		t.Fatalf("called %q with key %q, want deepseek with the saved key", provider, fake.key)
	}

	// New provider: the typed key and model name are used, nothing is saved.
	code, out = post(`{"agent_model_id":"openai","api_key":"sk-typed","custom_model_name":"gpt-test"}`)
	if code != http.StatusOK || out["ok"] != true || out["model"] != "gpt-test" {
		t.Fatalf("new-provider test = %d %v", code, out)
	}
	if provider != "openai" || fake.key != "sk-typed" || fake.model != "gpt-test" {
		t.Fatalf("called %q key %q model %q", provider, fake.key, fake.model)
	}
	if models, _ := st.Models.List(owner); len(models) != 1 {
		t.Fatalf("test saved a model: %d rows", len(models))
	}

	// A provider error is reported as ok=false with its message, key masked.
	fake.err = errors.New(`API returned error (status 401): {"error":{"message":"Incorrect API key provided: sk-typed","type":"invalid_request_error"}}`)
	code, out = post(`{"agent_model_id":"openai","api_key":"sk-typed"}`)
	if code != http.StatusOK || out["ok"] != false || out["error"] != "HTTP 401: Incorrect API key provided: ***" {
		t.Fatalf("failed test = %d %v", code, out)
	}

	for _, body := range []string{
		`{"agent_model_id":"nope","api_key":"k"}`,
		`{"agent_model_id":"openai","api_key":""}`,
		`{"agent_model_id":"openai","api_key":"k","custom_api_url":"http://127.0.0.1:9/v1"}`,
	} {
		if code, _ := post(body); code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", body, code)
		}
	}
}

func TestModelListAnswersWithTheProvidersNames(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("model-list-secret-", 4))
	var key string
	answer, status := `{"data":[{"id":"b-model"},{"id":"a-model"},{"id":"an-embedding"}]}`, http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("Authorization")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer server.Close()
	st := NewStores(testdb.Open(t))
	owner := testdb.AddUser(t, st.DB)
	if err := st.Models.Save(owner, "deepseek", "DeepSeek", true, "sk-saved-key", server.URL, ""); err != nil {
		t.Fatal(err)
	}
	saved, err := st.Models.Find(owner, "deepseek")
	if err != nil {
		t.Fatal(err)
	}

	// The real client refuses to reach a private address such as the fixture's.
	orig := newModelListClient
	newModelListClient = func(p string) agent.Client {
		return agent.NewClientByProvider(p, agent.WithHTTPClient(server.Client()))
	}
	t.Cleanup(func() { newModelListClient = orig })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	s := &Server{stores: st}
	router.GET("/models/:id/available", s.handleSavedModelList)
	router.POST("/models/available", s.handleModelFormList)
	call := func(method, path, as, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Test-User", as)
		router.ServeHTTP(w, r)
		return w
	}

	w := call(http.MethodGet, "/models/"+saved.ID+"/available", owner, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `{"models":["a-model","b-model"]}`) {
		t.Fatalf("saved list = %d %s", w.Code, w.Body)
	}
	if key != "Bearer sk-saved-key" {
		t.Errorf("asked with %q", key)
	}
	if w := call(http.MethodGet, "/models/"+saved.ID+"/available", testdb.AddOtherUser(t, st.DB), ""); w.Code != http.StatusNotFound {
		t.Errorf("another user got %d", w.Code)
	}

	// What goes wrong comes back as the provider's message, without the key.
	answer, status = `{"error":{"message":"sk-saved-key is not valid"}}`, http.StatusUnauthorized
	w = call(http.MethodGet, "/models/"+saved.ID+"/available", owner, "")
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "sk-saved-key") || !strings.Contains(w.Body.String(), "HTTP 401") {
		t.Errorf("failed list = %d %s", w.Code, w.Body)
	}

	// The form's endpoint is held to the same rules as saving one.
	w = call(http.MethodPost, "/models/available", owner, `{"agent_model_id":"deepseek","api_key":"sk-typed","custom_api_url":"http://127.0.0.1:1/v1"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("a private endpoint got %d", w.Code)
	}
	w = call(http.MethodPost, "/models/available", owner, `{"agent_model_id":"nope","api_key":"sk-typed"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("an unknown provider got %d", w.Code)
	}
}
