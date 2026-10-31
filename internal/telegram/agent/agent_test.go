package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"vergex/internal/llm"
)

// mockLLM implements llm.AIClient using pre-programmed LLMResponse objects.
type mockLLM struct {
	responses []*llm.LLMResponse
	calls     int
	lastReq   *llm.Request
}

func (m *mockLLM) SetAPIKey(_, _, _ string)                     {}
func (m *mockLLM) SetTimeout(_ time.Duration)                   {}
func (m *mockLLM) CallWithMessages(_, _ string) (string, error) { return "", nil }
func (m *mockLLM) CallWithRequest(*llm.Request) (string, error) { return "", nil }
func (m *mockLLM) CallWithRequestStream(*llm.Request, func(string)) (string, error) {
	return "", nil
}
func (m *mockLLM) CallWithRequestFull(req *llm.Request) (*llm.LLMResponse, error) {
	m.lastReq = req
	if m.calls < len(m.responses) {
		r := m.responses[m.calls]
		m.calls++
		return r, nil
	}
	m.calls++
	return &llm.LLMResponse{Content: "OK"}, nil
}

func skill(id, name string, args any) *llm.LLMResponse {
	b, _ := json.Marshal(args)
	return &llm.LLMResponse{ToolCalls: []llm.ToolCall{{
		ID: id, Type: "function",
		Function: llm.ToolCallFunction{Name: name, Arguments: string(b)},
	}}}
}

func text(content string) *llm.LLMResponse { return &llm.LLMResponse{Content: content} }

// fakeAPI serves canned GET responses and records every request.
type fakeAPI struct {
	mu       sync.Mutex
	gets     map[string]string
	requests []string
	bodies   map[string]string
}

func newFakeAPI(gets map[string]string) (*fakeAPI, int, func()) {
	f := &fakeAPI{gets: gets, bodies: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.requests = append(f.requests, key)
		f.bodies[key] = string(body)
		f.mu.Unlock()
		if r.Method == http.MethodGet {
			if resp, ok := f.gets[r.URL.Path]; ok {
				w.Write([]byte(resp)) //nolint:errcheck
				return
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`)) //nolint:errcheck
			return
		}
		w.Write([]byte(`{"message":"ok","trader_id":"new-trader","id":"new-strategy"}`)) //nolint:errcheck
	}))
	var port int
	fmt.Sscanf(srv.Listener.Addr().String(), "127.0.0.1:%d", &port)
	return f, port, srv.Close
}

func (f *fakeAPI) called(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r == key {
			return true
		}
	}
	return false
}

func (f *fakeAPI) body(key string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var m map[string]any
	_ = json.Unmarshal([]byte(f.bodies[key]), &m)
	return m
}

var account = map[string]string{
	"/api/my-traders": `[{"trader_id":"t1","trader_name":"BTC Trend","is_running":false,"strategy_name":"Trend"},
		{"trader_id":"t2","trader_name":"ETH Grid","is_running":true}]`,
	"/api/models":     `[{"id":"u_deepseek","name":"DeepSeek AI","provider":"deepseek","enabled":true,"has_api_key":true},{"id":"u_openai","name":"OpenAI","provider":"openai","enabled":false}]`,
	"/api/exchanges":  `[{"id":"ex1","exchange_type":"binance","account_name":"Main","enabled":true,"has_api_key":true,"has_secret_key":true}]`,
	"/api/strategies": `{"strategies":[{"id":"s1","name":"Trend","is_active":true},{"id":"s2","name":"Scalp"}]}`,
	"/api/positions":  `[{"symbol":"BTCUSDT","side":"long","size":0.1}]`,
}

func newAgent(port int, model *mockLLM) *Agent {
	return New(port, "tok", "user", func() llm.AIClient { return model }, "test prompt")
}

func TestOnlySkillToolsAreExposed(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockLLM{responses: []*llm.LLMResponse{text("hi")}}
	newAgent(port, model).Run("hello", nil)

	var names []string
	for _, tool := range model.lastReq.Tools {
		names = append(names, tool.Function.Name)
	}
	if strings.Contains(strings.Join(names, ","), "api_request") || len(names) != len(skillTools) {
		t.Fatalf("unexpected tools: %v", names)
	}
}

func TestStartTraderWaitsForConfirmation(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockLLM{responses: []*llm.LLMResponse{
		skill("c1", skillTrader, map[string]any{"action": "start", "target_ref": map[string]any{"name": "btc trend"}}),
		text("请回复「确认」以启动 BTC Trend。"),
	}}
	a := newAgent(port, model)

	a.Run("启动 BTC Trend", nil)
	if api.called("POST /api/traders/t1/start") {
		t.Fatal("trader started before confirmation")
	}
	if a.pending == nil {
		t.Fatal("expected a pending action")
	}
	if !strings.Contains(model.lastReq.Messages[len(model.lastReq.Messages)-1].Content, "needs_confirmation") {
		t.Fatal("LLM was not told that confirmation is required")
	}

	calls := model.calls
	reply := a.Run("确认", nil)
	if !api.called("POST /api/traders/t1/start") {
		t.Fatal("confirmed start was not executed")
	}
	if model.calls != calls {
		t.Fatal("confirmation must not go through the LLM")
	}
	if !strings.HasPrefix(reply, "✅ 已执行") {
		t.Fatalf("reply = %q", reply)
	}
}

func TestOtherReplyCancelsPendingAction(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockLLM{responses: []*llm.LLMResponse{
		skill("c1", skillTrader, map[string]any{"action": "delete", "target_ref": map[string]any{"id": "t2"}}),
		text("Reply confirm to delete."),
		text("Sure, kept it."),
	}}
	a := newAgent(port, model)
	a.Run("delete ETH Grid", nil)
	reply := a.Run("actually no, keep it", nil)
	if api.called("DELETE /api/traders/t2") {
		t.Fatal("cancelled delete was executed")
	}
	if !strings.HasPrefix(reply, "Cancelled the pending action") || a.pending != nil {
		t.Fatalf("reply = %q pending=%v", reply, a.pending)
	}
	if a.Run("confirm", nil); api.called("DELETE /api/traders/t2") {
		t.Fatal("a late confirmation revived a cancelled action")
	}
}

func TestExpiredConfirmationDoesNothing(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockLLM{responses: []*llm.LLMResponse{
		skill("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
		text("Confirm?"),
	}}
	a := newAgent(port, model)
	a.Run("stop eth grid", nil)
	a.pending.expires = time.Now().Add(-time.Second)
	if reply := a.Run("yes", nil); !strings.Contains(reply, "expired") || api.called("POST /api/traders/t2/stop") {
		t.Fatalf("expired confirmation executed or misreported: %q", reply)
	}
}

func TestAmbiguousAndUnknownTargetsAreNotGuessed(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockLLM{})

	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"stop"}`)), &out)
	if out["error"] != "ambiguous" {
		t.Fatalf("missing target with two traders: %v", out)
	}
	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"start","target_ref":{"name":"SOL"}}`)), &out)
	if out["error"] != "not_found" {
		t.Fatalf("unknown target: %v", out)
	}
	if a.pending != nil {
		t.Fatal("no action may be held for an unresolved target")
	}
}

func TestCreateTraderInfersSingleEnabledModelAndExchange(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockLLM{})

	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"create","slots":{}}`)), &out)
	if out["error"] != "cannot_create_yet" || fmt.Sprint(out["missing"]) != "[name]" {
		t.Fatalf("missing name not reported: %v", out)
	}

	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"create","slots":{"name":"SOL bot","strategy":"scalp"}}`)), &out)
	if out["status"] != "created" {
		t.Fatalf("create failed: %v", out)
	}
	body := api.body("POST /api/traders")
	if body["ai_model_id"] != "u_deepseek" || body["exchange_id"] != "ex1" || body["strategy_id"] != "s2" {
		t.Fatalf("create body = %v", body)
	}
	if a.pending != nil {
		t.Fatal("create without auto_start must not hold a start")
	}
}

func TestStrategyRiskChangesNeedConfirmation(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockLLM{})

	a.runSkill(skillStrategy, `{"action":"update","target_ref":{"name":"Scalp"},"slots":{"name":"Scalp v2"}}`)
	if !api.called("PUT /api/strategies/s2") || a.pending != nil {
		t.Fatal("a rename should apply directly")
	}

	api.requests = nil
	a.runSkill(skillStrategy, `{"action":"update","target_ref":{"name":"Trend"},"slots":{"config_patch":{"ai_config":{"risk_control":{"max_positions":5}}}}}`)
	if api.called("PUT /api/strategies/s1") || a.pending == nil {
		t.Fatal("a risk-control change must wait for confirmation")
	}
}

func TestModelAndExchangeAreReadOnly(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockLLM{})
	for _, name := range []string{skillModel, skillExchange} {
		var out map[string]any
		json.Unmarshal([]byte(a.runSkill(name, `{"action":"update"}`)), &out)
		if out["error"] != "not_available_in_chat" {
			t.Fatalf("%s update allowed: %v", name, out)
		}
	}
}

func TestClosePositionChecksTheOpenPosition(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockLLM{})

	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillClose, `{"trader_ref":{"name":"BTC Trend"},"symbol":"ETHUSDT","side":"LONG"}`)), &out)
	if out["error"] != "position_not_found" || a.pending != nil {
		t.Fatalf("closing a missing position: %v", out)
	}
	a.runSkill(skillClose, `{"trader_ref":{"name":"BTC Trend"},"symbol":"btcusdt","side":"long"}`)
	if a.pending == nil {
		t.Fatal("close must wait for confirmation")
	}
	if reply := runPending(a.takePending(), false); !strings.HasPrefix(reply, "✅ Done") {
		t.Fatalf("close reply = %q", reply)
	}
	if b := api.body("POST /api/traders/t1/close-position"); b["symbol"] != "BTCUSDT" || b["side"] != "LONG" {
		t.Fatalf("close body = %v", b)
	}
}

func TestAccountQueryRoutesToTraderData(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockLLM{})
	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillAccount, `{"scope":"positions","trader_ref":{"name":"BTC"}}`)), &out)
	if out["trader"] != "BTC Trend" || !api.called("GET /api/positions") {
		t.Fatalf("positions query = %v", out)
	}
}

func TestUnknownSkillAndMaxIterations(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	if out := newAgent(port, &mockLLM{}).runSkill("api_request", `{}`); !strings.Contains(out, "unknown_skill") {
		t.Fatalf("unknown skill result: %s", out)
	}

	loop := make([]*llm.LLMResponse, maxIterations+2)
	for i := range loop {
		loop[i] = skill(fmt.Sprint("c", i), skillAccount, map[string]any{"scope": "overview"})
	}
	model := &mockLLM{responses: loop}
	newAgent(port, model).Run("loop", nil)
	if model.calls != maxIterations {
		t.Fatalf("expected %d LLM calls, got %d", maxIterations, model.calls)
	}
}

func TestConfirmationPattern(t *testing.T) {
	for _, yes := range []string{"确认", "确认！", "confirm", "Yes", "ok"} {
		if !isConfirmation(yes) {
			t.Errorf("%q should confirm", yes)
		}
	}
	for _, no := range []string{"确认一下仓位", "yes but change the name", "不", "cancel"} {
		if isConfirmation(no) {
			t.Errorf("%q must not confirm", no)
		}
	}
}
