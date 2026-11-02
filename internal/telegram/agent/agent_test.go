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

	"vergex/internal/agent"
)

// mockClient implements agent.Client using pre-programmed Response objects.
type mockClient struct {
	responses []*agent.Response
	calls     int
	lastReq   *agent.Request
}

func (m *mockClient) SetAPIKey(_, _, _ string)                       {}
func (m *mockClient) SetTimeout(_ time.Duration)                     {}
func (m *mockClient) CallWithMessages(_, _ string) (string, error)   { return "", nil }
func (m *mockClient) CallWithRequest(*agent.Request) (string, error) { return "", nil }
func (m *mockClient) CallWithRequestFull(req *agent.Request) (*agent.Response, error) {
	m.lastReq = req
	if m.calls < len(m.responses) {
		r := m.responses[m.calls]
		m.calls++
		return r, nil
	}
	m.calls++
	return &agent.Response{Content: "OK"}, nil
}

func skill(id, name string, args any) *agent.Response {
	b, _ := json.Marshal(args)
	return &agent.Response{ToolCalls: []agent.ToolCall{{
		ID: id, Type: "function",
		Function: agent.ToolCallFunction{Name: name, Arguments: string(b)},
	}}}
}

func text(content string) *agent.Response { return &agent.Response{Content: content} }

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
		w.Write([]byte(`{"message":"ok","trader_id":"new-trader","kind":"agent"}`)) //nolint:errcheck
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
	"/api/traders": `[{"trader_id":"t1","trader_name":"BTC Trend","is_running":false,"kind":"agent"},
		{"trader_id":"t2","trader_name":"ETH Grid","is_running":true,"kind":"grid"},
		{"trader_id":"t3","trader_name":"ADA Scalp","is_running":true,"kind":"agent"},
		{"trader_id":"t4","trader_name":"XRP Grid","is_running":false,"kind":"grid"}]`,
	"/api/agent/models":              `[{"id":"u_deepseek","name":"DeepSeek","provider":"deepseek","enabled":true,"has_api_key":true},{"id":"u_openai","name":"OpenAI","provider":"openai","enabled":false}]`,
	"/api/accounts":                  `[{"id":"ex1","venue":"binance","name":"Main","enabled":true,"has_api_key":true,"has_secret_key":true}]`,
	"/api/traders/t1/open-positions": `[{"symbol":"BTCUSDT","side":"long","size":0.1}]`,
}

func newAgent(port int, model *mockClient) *Agent {
	return New(port, "tok", "user", func() agent.Client { return model }, "test prompt")
}

func TestOnlySkillToolsAreExposed(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{text("hi")}}
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
	model := &mockClient{responses: []*agent.Response{
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
		t.Fatal("model was not told that confirmation is required")
	}

	calls := model.calls
	reply := a.Run("确认", nil)
	if !api.called("POST /api/traders/t1/start") {
		t.Fatal("confirmed start was not executed")
	}
	if model.calls != calls {
		t.Fatal("confirmation must not go through the model")
	}
	if !strings.HasPrefix(reply, "✅ 已执行") {
		t.Fatalf("reply = %q", reply)
	}
}

func TestOtherReplyCancelsPendingAction(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{
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
	model := &mockClient{responses: []*agent.Response{
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
	a := newAgent(port, &mockClient{})

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

func TestCreateTraderInfersSingleEnabledModelAndAccount(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{})

	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"create","slots":{}}`)), &out)
	if out["error"] != "cannot_create_yet" || fmt.Sprint(out["missing"]) != "[name]" {
		t.Fatalf("missing name not reported: %v", out)
	}

	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"create","slots":{"name":"SOL bot","kind":"agent","preset":"careful","config_patch":{"coin_source":{"static_coins":["SOLUSDT"]}}}}`)), &out)
	if out["status"] != "created" {
		t.Fatalf("create failed: %v", out)
	}
	body := api.body("POST /api/traders")
	if body["agent_model_id"] != "u_deepseek" || body["account_id"] != "ex1" || body["kind"] != "agent" || body["preset"] != "careful" {
		t.Fatalf("create body = %v", body)
	}
	patch, _ := body["config"].(map[string]any)
	if _, ok := patch["coin_source"]; !ok {
		t.Fatalf("create body lost the config patch: %v", body)
	}
	if a.pending != nil {
		t.Fatal("create without auto_start must not hold a start")
	}
}

func TestTraderConfigChangesNeedConfirmationWhenRisky(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{})

	// A stopped trader: what does not touch risk applies directly and sends only itself.
	a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"BTC Trend"},"slots":{"name":"BTC v2","config_patch":{"coin_source":{"static_coins":["BTCUSDT"]}}}}`)
	if !api.called("PUT /api/traders/t1") || a.pending != nil {
		t.Fatal("a rename and a coin list should apply directly")
	}
	body := api.body("PUT /api/traders/t1")
	if body["name"] != "BTC v2" || body["agent_model_id"] != nil || body["config"] == nil {
		t.Fatalf("update body = %v", body)
	}

	// A stopped trader: risk fields of its kind wait, whichever kind it is.
	api.requests = nil
	a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"BTC Trend"},"slots":{"config_patch":{"risk_control":{"max_positions":5}}}}`)
	if api.called("PUT /api/traders/t1") || a.pending == nil {
		t.Fatal("a risk change must wait for confirmation")
	}
	if reply := runPending(a.takePending(), false); !strings.HasPrefix(reply, "✅ Done") || !api.called("PUT /api/traders/t1") {
		t.Fatalf("confirmed reply = %q", reply)
	}

	api.requests = nil
	a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"XRP Grid"},"slots":{"config_patch":{"leverage":2}}}`)
	if api.called("PUT /api/traders/t4") || a.pending == nil {
		t.Fatal("a grid risk change must wait for confirmation")
	}
	a.takePending()

	a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"XRP Grid"},"slots":{"config_patch":{"use_maker_only":false}}}`)
	if !api.called("PUT /api/traders/t4") || a.pending != nil {
		t.Fatal("a grid setting without risk should apply directly")
	}

	// A running trader always waits.
	api.requests = nil
	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"ADA Scalp"},"slots":{"config_patch":{"custom_prompt":"be calm"}}}`)), &out)
	if api.called("PUT /api/traders/t3") || a.pending == nil {
		t.Fatal("changing a running trader must wait for confirmation")
	}
	if summary := fmt.Sprint(out["summary"]); !strings.Contains(summary, "next cycle") {
		t.Fatalf("an agent trader's config takes effect next cycle: %v", out)
	}
	a.takePending()

	json.Unmarshal([]byte(a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"ETH Grid"},"slots":{"config_patch":{"use_maker_only":false}}}`)), &out)
	if summary := fmt.Sprint(out["summary"]); !strings.Contains(summary, "restarted") {
		t.Fatalf("a grid trader restarts with its config: %v", out)
	}
	a.takePending()
}

func TestStrategyManagementIsGone(t *testing.T) {
	for _, name := range skillNames() {
		if strings.Contains(name, "strategy") {
			t.Fatalf("skill %s is still offered", name)
		}
	}
}

func TestModelAndAccountAreReadOnly(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{})
	for _, name := range []string{skillModel, skillAccounts} {
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
	a := newAgent(port, &mockClient{})

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
	a := newAgent(port, &mockClient{})
	var out map[string]any
	json.Unmarshal([]byte(a.runSkill(skillAccount, `{"scope":"positions","trader_ref":{"name":"BTC"}}`)), &out)
	if out["trader"] != "BTC Trend" || !api.called("GET /api/traders/t1/open-positions") {
		t.Fatalf("positions query = %v", out)
	}
}

func TestUnknownSkillAndMaxIterations(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	if out := newAgent(port, &mockClient{}).runSkill("api_request", `{}`); !strings.Contains(out, "unknown_skill") {
		t.Fatalf("unknown skill result: %s", out)
	}

	loop := make([]*agent.Response, maxIterations+2)
	for i := range loop {
		loop[i] = skill(fmt.Sprint("c", i), skillAccount, map[string]any{"scope": "overview"})
	}
	model := &mockClient{responses: loop}
	newAgent(port, model).Run("loop", nil)
	if model.calls != maxIterations {
		t.Fatalf("expected %d model calls, got %d", maxIterations, model.calls)
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
