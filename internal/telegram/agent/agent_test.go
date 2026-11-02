package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"vergex/internal/agent"
	"vergex/internal/security/auth"
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

func toolCall(id, name string, args any) agent.ToolCall {
	b, _ := json.Marshal(args)
	return agent.ToolCall{ID: id, Type: "function", Function: agent.ToolCallFunction{Name: name, Arguments: string(b)}}
}

// firstContent is the user message of the last request.
func (m *mockClient) firstContent() string {
	for _, message := range m.lastReq.Messages {
		if message.Role == "user" {
			return message.Content
		}
	}
	return ""
}

func skill(id, name string, args any) *agent.Response {
	return &agent.Response{ToolCalls: []agent.ToolCall{toolCall(id, name, args)}}
}

func text(content string) *agent.Response { return &agent.Response{Content: content} }

// fakeAPI serves canned GET responses and records every request.
type fakeAPI struct {
	mu       sync.Mutex
	gets     map[string]string
	requests []string
	bodies   map[string]string
	tokens   []string
	queries  map[string]string
	refused  map[string]bool
}

func newFakeAPI(gets map[string]string) (*fakeAPI, int, func()) {
	f := &fakeAPI{gets: gets, bodies: map[string]string{}, queries: map[string]string{}, refused: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.requests = append(f.requests, key)
		f.bodies[key] = string(body)
		f.tokens = append(f.tokens, r.Header.Get("Authorization"))
		f.queries[r.URL.Path] = r.URL.RawQuery
		refused := f.refused[key]
		f.mu.Unlock()
		if refused {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"refused"}`)) //nolint:errcheck
			return
		}
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

// refuse makes a request fail from now on.
func (f *fakeAPI) refuse(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refused[key] = true
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

func (f *fakeAPI) queried(path, query string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[path] == query
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
	token := func() (string, error) { return "tok", nil }
	return New(port, token, "user", func() agent.Client { return model }, "test prompt", func() string { return "" })
}

// A JWT lasts a day and the bot runs for months, so skills must not hold on to one.
func TestEveryAPICallIsSignedAnew(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	auth.SetJWTSecret("test-secret-for-the-telegram-agent")
	issued := 0
	token := func() (string, error) {
		issued++
		return GenerateBotToken("user")
	}
	a := New(port, token, "user", func() agent.Client { return &mockClient{} }, "test prompt", func() string { return "" })
	for range 2 {
		if _, err := a.traders(); err != nil {
			t.Fatal(err)
		}
	}
	if issued != 2 {
		t.Fatalf("signed %d tokens for 2 calls", issued)
	}
	claims, err := auth.ValidateJWT(strings.TrimPrefix(api.tokens[1], "Bearer "))
	if err != nil || claims.UserID != "user" {
		t.Fatalf("token did not validate: %v", err)
	}
}

func TestOnlySkillToolsAreExposed(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{text("hi")}}
	newAgent(port, model).Run(context.Background(), "hello", nil)

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

	a.Run(context.Background(), "启动 BTC Trend", nil)
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
	reply := a.Run(context.Background(), "确认", nil).Text
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
	a.Run(context.Background(), "delete ETH Grid", nil)
	reply := a.Run(context.Background(), "actually no, keep it", nil).Text
	if api.called("DELETE /api/traders/t2") {
		t.Fatal("cancelled delete was executed")
	}
	if !strings.HasPrefix(reply, "↩️ Cancelled the pending actions") || a.pending != nil {
		t.Fatalf("reply = %q pending=%v", reply, a.pending)
	}
	if a.Run(context.Background(), "confirm", nil); api.called("DELETE /api/traders/t2") {
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
	a.Run(context.Background(), "stop eth grid", nil)
	a.pending.expires = time.Now().Add(-time.Second)
	if reply := a.Run(context.Background(), "yes", nil).Text; !strings.Contains(reply, "expired") || api.called("POST /api/traders/t2/stop") {
		t.Fatalf("expired confirmation executed or misreported: %q", reply)
	}
}

// "Stop all my traders" holds one action per trader, and confirming must run them all.
func TestConfirmingRunsEverythingHeldAndSaysHowEachWent(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	api.refuse("POST /api/traders/t3/stop")
	model := &mockClient{responses: []*agent.Response{
		{ToolCalls: []agent.ToolCall{
			toolCall("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
			toolCall("c2", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ADA Scalp"}}),
		}},
		text("Both are held."),
	}}
	a := newAgent(port, model)

	reply := a.Run(context.Background(), "stop all my traders", nil)
	if reply.Text != "Both are held." {
		t.Fatalf("text = %q", reply.Text)
	}
	if reply.Confirm == nil || len(reply.Confirm.Lines) != 2 {
		t.Fatalf("confirmation = %+v", reply.Confirm)
	}
	if api.called("POST /api/traders/t2/stop") || api.called("POST /api/traders/t3/stop") {
		t.Fatal("something ran before confirmation")
	}

	done := a.Run(context.Background(), "confirm", nil).Text
	if !api.called("POST /api/traders/t2/stop") || !api.called("POST /api/traders/t3/stop") {
		t.Fatalf("confirming did not run everything: %v", api.requests)
	}
	if !strings.Contains(done, "✅ Done: stop running trader \"ETH Grid\"") || !strings.Contains(done, "❌ Failed: stop running trader \"ADA Scalp\"") {
		t.Fatalf("result = %q", done)
	}
}

// What is confirmed is what Go wrote, whatever the model says it is.
func TestConfirmationIsWrittenByGoAndNamesTheAccount(t *testing.T) {
	gets := map[string]string{
		"/api/traders":  `[{"trader_id":"t2","trader_name":"ETH Grid","is_running":true,"kind":"grid","account_id":"ex2"},{"trader_id":"t3","trader_name":"ADA Scalp","is_running":true,"kind":"agent","account_id":"ex1"}]`,
		"/api/accounts": `[{"id":"ex1","venue":"binance","name":"Main"},{"id":"ex2","venue":"binance","name":"Practice","testnet":true}]`,
	}
	_, port, stop := newFakeAPI(gets)
	defer stop()
	model := &mockClient{responses: []*agent.Response{
		{ToolCalls: []agent.ToolCall{
			toolCall("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
			toolCall("c2", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ADA Scalp"}}),
		}},
		text("I will only stop the small one."),
	}}
	reply := newAgent(port, model).Run(context.Background(), "stop", nil)
	lines := strings.Join(reply.Confirm.Lines, "\n")
	for _, want := range []string{`"ETH Grid" on account "Practice" (testnet)`, `"ADA Scalp" on account "Main" (real money)`} {
		if !strings.Contains(lines, want) {
			t.Fatalf("%q is missing %q", lines, want)
		}
	}
}

func TestAskingForTheSameActionTwiceHoldsItOnce(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	call := map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}
	model := &mockClient{responses: []*agent.Response{skill("c1", skillTrader, call), skill("c2", skillTrader, call), text("ok")}}
	if lines := newAgent(port, model).Run(context.Background(), "stop", nil).Confirm.Lines; len(lines) != 1 {
		t.Fatalf("held %v", lines)
	}
}

func TestAButtonAnswersOnlyTheBatchItCameWith(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	stopETH := skill("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}})
	stopADA := skill("c2", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ADA Scalp"}})
	model := &mockClient{responses: []*agent.Response{stopETH, text("held"), stopADA, text("held")}}
	a := newAgent(port, model)

	first := a.Run(context.Background(), "stop eth", nil).Confirm
	// A new request cancels the first batch and holds another.
	second := a.Run(context.Background(), "no, stop ada", nil).Confirm
	if first.ID == second.ID {
		t.Fatal("two batches share an ID")
	}
	if _, ok := a.Answer(first.ID, true); ok {
		t.Fatal("an old button answered a newer batch")
	}
	if api.called("POST /api/traders/t2/stop") || api.called("POST /api/traders/t3/stop") {
		t.Fatal("an old button ran something")
	}

	// Cancelling runs nothing, and the same button does nothing after that.
	reply, ok := a.Answer(second.ID, false)
	if !ok || !strings.Contains(reply.Text, "Cancelled") || api.called("POST /api/traders/t3/stop") {
		t.Fatalf("cancel: ok=%v reply=%q", ok, reply.Text)
	}
	if _, ok := a.Answer(second.ID, true); ok || api.called("POST /api/traders/t3/stop") {
		t.Fatal("a button worked after its batch was answered")
	}
}

func TestAButtonAfterTheBatchExpiredRunsNothing(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{
		skill("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
		text("held"),
	}}
	a := newAgent(port, model)
	id := a.Run(context.Background(), "stop", nil).Confirm.ID
	a.pending.expires = time.Now().Add(-time.Second)
	reply, ok := a.Answer(id, true)
	if !ok || !strings.Contains(reply.Text, "expired") || api.called("POST /api/traders/t2/stop") {
		t.Fatalf("ok=%v reply=%q", ok, reply.Text)
	}
}

// Only the user's whole message can confirm: not a sentence that holds the word, and not
// anything the model or a tool result says.
func TestNothingButTheUsersOwnWordConfirms(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{
		skill("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
		text("确认"),
		text("confirmed, it is done"),
	}}
	a := newAgent(port, model)
	a.Run(context.Background(), "stop", nil)
	if api.called("POST /api/traders/t2/stop") {
		t.Fatal("the model's own words confirmed an action")
	}
	a.Run(context.Background(), "please do not confirm yet", nil)
	if api.called("POST /api/traders/t2/stop") || a.pending != nil {
		t.Fatal("a sentence holding the word confirmed an action")
	}
}

func TestChosenLanguageDecidesWhatGoWrites(t *testing.T) {
	gets := map[string]string{
		"/api/traders":  `[{"trader_id":"t2","trader_name":"ETH Grid","is_running":true,"kind":"grid","account_id":"ex1"}]`,
		"/api/accounts": `[{"id":"ex1","venue":"binance","name":"Main"}]`,
	}
	api, port, stop := newFakeAPI(gets)
	defer stop()
	model := &mockClient{responses: []*agent.Response{
		skill("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
		text("held"),
	}}
	a := newAgent(port, model)
	a.language = func() string { return "zh" }
	held := a.Run(context.Background(), "stop", nil).Confirm
	if want := "停止运行中的交易员“ETH Grid”，账户“Main”（实盘）"; held == nil || held.Lines[0] != want {
		t.Fatalf("held = %+v, want %q", held, want)
	}
	if !strings.Contains(model.firstContent(), "language setting is Simplified Chinese") {
		t.Fatalf("the model was not told the language: %s", model.firstContent())
	}
	if reply := a.Run(context.Background(), "yes", nil).Text; !strings.HasPrefix(reply, "✅ 已执行：停止运行中的交易员") || !api.called("POST /api/traders/t2/stop") {
		t.Fatalf("reply = %q", reply)
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

// What a cycle came to must reach the model: the full record leads with prompts that
// are longer than a whole tool result.
func TestDecisionsReachTheModelWithoutThePrompts(t *testing.T) {
	prompt := strings.Repeat("market data ", 3000)
	full, _ := json.Marshal(map[string]any{
		"id": 7, "system_prompt": prompt, "input_prompt": prompt, "cot_trace": "RSI is stretched",
		"execution_log": []string{"blocked: halted"}, "error_message": "margin too low",
		"decisions": []map[string]any{{"action": "open_long", "symbol": "SOLUSDT"}},
	})
	gets := map[string]string{
		"/api/traders":                      account["/api/traders"],
		"/api/traders/t1/decisions/summary": `[{"id":7,"execution_log":["blocked: halted"],"error_message":"margin too low","decisions":[{"action":"open_long"}]}]`,
		"/api/traders/t1/decisions/7":       string(full),
		"/api/traders/t1/status":            `{}`,
		"/api/traders/t1/open-orders":       `[]`,
	}
	api, port, stop := newFakeAPI(gets)
	defer stop()
	a := newAgent(port, &mockClient{})
	ref := map[string]any{"name": "BTC Trend"}

	list, _ := json.Marshal(map[string]any{"scope": "decisions", "trader_ref": ref})
	out := a.runSkill(skillAccount, string(list))
	if !strings.Contains(out, "margin too low") || !strings.Contains(out, "open_long") {
		t.Fatalf("decisions result: %s", out)
	}
	if !api.queried("/api/traders/t1/decisions/summary", "limit=5") {
		t.Fatal("decisions should read the summaries, five by default")
	}

	detail, _ := json.Marshal(map[string]any{"scope": "decision_detail", "trader_ref": ref, "decision_id": 7})
	out = a.runSkill(skillAccount, string(detail))
	if strings.Contains(out, "market data") {
		t.Fatal("a cycle's detail should leave out its prompts")
	}
	for _, want := range []string{"RSI is stretched", "blocked: halted", "margin too low", "open_long"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail is missing %q: %s", want, out)
		}
	}

	diagnosis, _ := json.Marshal(map[string]any{"topic": "order_execution", "target_ref": ref})
	if out := a.runSkill(skillDiagnose, string(diagnosis)); !strings.Contains(out, "margin too low") {
		t.Fatalf("diagnosis result: %s", out)
	}
}

func TestAccountPositionsReadEachAccountAndSayWhichFailed(t *testing.T) {
	gets := map[string]string{
		"/api/accounts":               `[{"id":"ex1","venue":"binance","name":"Main"},{"id":"ex2","venue":"okx","name":"Side","testnet":true}]`,
		"/api/accounts/ex1/positions": `[{"symbol":"BTCUSDT","side":"long"}]`,
	}
	api, port, stop := newFakeAPI(gets)
	defer stop()
	a := newAgent(port, &mockClient{})

	var all struct {
		Accounts []map[string]any `json:"accounts"`
	}
	json.Unmarshal([]byte(a.runSkill(skillAccounts, `{"action":"positions"}`)), &all)
	if len(all.Accounts) != 2 || all.Accounts[0]["positions"] == nil || all.Accounts[1]["error"] == nil || all.Accounts[1]["testnet"] != true {
		t.Fatalf("positions = %+v", all.Accounts)
	}
	if !api.called("GET /api/accounts/ex1/positions") {
		t.Fatal("the account was not read")
	}

	var one struct {
		Accounts []map[string]any `json:"accounts"`
	}
	json.Unmarshal([]byte(a.runSkill(skillAccounts, `{"action":"positions","target_ref":{"name":"Main"}}`)), &one)
	if len(one.Accounts) != 1 || one.Accounts[0]["account"] != "Main" {
		t.Fatalf("one account = %+v", one.Accounts)
	}
	// Still nothing but reading.
	if out := a.runSkill(skillAccounts, `{"action":"update"}`); !strings.Contains(out, "not_available_in_chat") {
		t.Fatalf("update = %s", out)
	}
}

func TestFuturesQuerySummarizesAndNeverWrites(t *testing.T) {
	plan := func(name, symbol string) string {
		return `{"id":"p","quantity":"0.1","reward_risk":"2","expires":"2026-11-02T10:00:00Z","idea":{"name":"` + name + `","side":"long","entry":"100","stop":"90","target":"120"},"snapshot":{"symbol":"` + symbol + `","venue":"binance","testnet":true},"body":"` + strings.Repeat("x", 500) + `"}`
	}
	gets := map[string]string{
		"/api/accounts":        `[{"id":"ex1","venue":"binance","name":"Main"}]`,
		"/api/futures/state":   `{"records":{"a":{"state":"draft","plan":` + plan("Draft BTC", "BTCUSDT") + `},"b":{"state":"completed","plan":` + plan("Done", "BTCUSDT") + `},"c":{"state":"draft","plan":` + plan("Draft ETH", "ETHUSDT") + `}},"runs":[{"id":"r1","account_id":"ex1","state":"needs_attention","error":"check it","plan":` + plan("Live BTC", "BTCUSDT") + `,"orders":[{"role":"entry","status":"FILLED","filled":"0.1"}]}]}`,
		"/api/futures/results": `{"r1":{"realized_pnl":12.5,"closes":1}}`,
	}
	api, port, stop := newFakeAPI(gets)
	defer stop()
	a := newAgent(port, &mockClient{})

	var runs struct {
		Total int              `json:"total"`
		Shown []map[string]any `json:"shown"`
	}
	json.Unmarshal([]byte(a.runSkill(skillFutures, `{"scope":"runs"}`)), &runs)
	if runs.Total != 1 || runs.Shown[0]["name"] != "Live BTC" || runs.Shown[0]["state"] != "needs_attention" || runs.Shown[0]["error"] != "check it" || runs.Shown[0]["testnet"] != true {
		t.Fatalf("runs = %+v", runs)
	}
	var drafts struct {
		Total int              `json:"total"`
		Shown []map[string]any `json:"shown"`
	}
	json.Unmarshal([]byte(a.runSkill(skillFutures, `{"scope":"drafts","symbol":"ethusdt"}`)), &drafts)
	if drafts.Total != 1 || drafts.Shown[0]["name"] != "Draft ETH" {
		t.Fatalf("drafts = %+v", drafts)
	}
	if out := a.runSkill(skillFutures, `{"scope":"drafts"}`); strings.Contains(out, "xxxx") {
		t.Fatalf("the plan's body came along: %s", out)
	}
	// With one account, results need no name.
	if out := a.runSkill(skillFutures, `{"scope":"results"}`); !strings.Contains(out, "12.5") {
		t.Fatalf("results = %s", out)
	}
	if out := a.runSkill(skillFutures, `{"scope":"execute"}`); !strings.Contains(out, "unsupported_scope") {
		t.Fatalf("execute = %s", out)
	}
	for key := range api.bodies {
		if !strings.HasPrefix(key, "GET ") {
			t.Fatalf("a query wrote: %s", key)
		}
	}
}

func TestForecastQueryReadsSignalsAndAsksForTheCoin(t *testing.T) {
	gets := map[string]string{
		"/api/forecast/signals":        `{"coins":[{"symbol":"BTCUSDT","score":61}]}`,
		"/api/forecast/detail/BTCUSDT": `{"symbol":"BTCUSDT"}`,
	}
	_, port, stop := newFakeAPI(gets)
	defer stop()
	a := newAgent(port, &mockClient{})
	if out := a.runSkill(skillForecast, `{"scope":"signals","interval":"4h"}`); !strings.Contains(out, "61") || !strings.Contains(out, "experimental") {
		t.Fatalf("signals = %s", out)
	}
	if out := a.runSkill(skillForecast, `{"scope":"detail","symbol":"btcusdt"}`); !strings.Contains(out, "BTCUSDT") {
		t.Fatalf("detail = %s", out)
	}
	if out := a.runSkill(skillForecast, `{"scope":"detail"}`); !strings.Contains(out, "symbol") || !strings.Contains(out, "missing") {
		t.Fatalf("detail without a coin = %s", out)
	}
}

func TestProgressSaysWhatIsUnderWayInGosWords(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{
		skill("c1", skillAccount, map[string]any{"scope": "positions", "trader_ref": map[string]any{"name": "BTC Trend"}}),
		skill("c2", skillTrader, map[string]any{"action": "query"}),
		text("done"),
	}}
	var stages []string
	newAgent(port, model).Run(context.Background(), "持仓？", &Progress{Stage: func(stage string) { stages = append(stages, stage) }})
	want := []string{"正在思考…", "正在读取持仓…", "正在读取交易员…"}
	if strings.Join(stages, "|") != strings.Join(want, "|") {
		t.Fatalf("stages = %v", stages)
	}
}

// blockingClient waits for its context, as a slow model does.
type blockingClient struct {
	mockClient
	started chan struct{}
}

func (c *blockingClient) CallWithRequestFull(req *agent.Request) (*agent.Response, error) {
	close(c.started)
	<-req.Ctx.Done()
	return nil, req.Ctx.Err()
}

func TestStoppingATurnDropsWhatItHeldAndSaysSo(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	token := func() (string, error) { return "tok", nil }
	client := &blockingClient{started: make(chan struct{})}
	a := New(port, token, "user", func() agent.Client { return client }, "test prompt", func() string { return "" })
	// Something was held, and the next message starts a turn that gets stopped.
	a.hold("old action", func() (string, error) { return "", nil })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-client.started
		cancel()
	}()
	reply := a.Run(ctx, "something else", nil)
	if !strings.Contains(reply.Text, "Stopped") || reply.Confirm != nil || a.pending != nil {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestATurnOutOfTimeSaysItDidNotFinishAndKeepsWhatItHeld(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	a := newAgent(port, &mockClient{})
	a.record("user", agentMessage("x"))
	reply := a.Run(ctx, "something", nil)
	if !strings.Contains(reply.Text, "did not finish") {
		t.Fatalf("reply = %q", reply.Text)
	}
}

func TestCommandsAnswerFromTheAPIWithoutTheModel(t *testing.T) {
	gets := map[string]string{
		"/api/traders":                `[{"trader_id":"t1","trader_name":"Trend","is_running":true,"kind":"agent"},{"trader_id":"t2","trader_name":"Grid","is_running":false,"kind":"grid"}]`,
		"/api/accounts":               `[{"id":"ex1","venue":"binance","name":"Main"},{"id":"ex2","venue":"binance","name":"Practice","testnet":true},{"id":"ex3","venue":"okx","name":"Broken"}]`,
		"/api/accounts/state":         `{"states":{"ex1":{"status":"ok","display_balance":"1000.50 USDT"},"ex2":{"status":"ok","display_balance":"50.00 USDT"},"ex3":{"status":"invalid_credentials"}}}`,
		"/api/accounts/ex1/positions": `[{"symbol":"BTCUSDT","side":"long","positionAmt":"0.5","entryPrice":"70000","unRealizedProfit":"12.345"},{"symbol":"ETHUSDT","side":"SHORT","quantity":-2,"entry_price":3000,"unrealized_pnl":-4}]`,
		"/api/accounts/ex2/positions": `[]`,
	}
	_, port, stop := newFakeAPI(gets)
	defer stop()
	model := &mockClient{}
	a := newAgent(port, model)

	positions := a.PositionsText()
	for _, want := range []string{"**Main**", "`BTCUSDT` long 0.5 @ 70000 · +12.35 USDT", "`ETHUSDT` short 2 @ 3000 · -4.00 USDT", "**Practice** (testnet)", "Nothing open", "**Broken**", "Could not be read"} {
		if !strings.Contains(positions, want) {
			t.Fatalf("positions are missing %q:\n%s", want, positions)
		}
	}
	status := a.StatusText()
	for _, want := range []string{"Main: 1000.50 USDT", "Practice (testnet): 50.00 USDT", "Broken: could not be read", "**Positions** at least 2", "**Traders** 1 running of 2"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status is missing %q:\n%s", want, status)
		}
	}
	traders := a.TradersText()
	for _, want := range []string{"**Traders** 1 running of 2", "🟢 Trend · agent · running", "⚪ Grid · grid · stopped"} {
		if !strings.Contains(traders, want) {
			t.Fatalf("traders are missing %q:\n%s", want, traders)
		}
	}
	if model.calls != 0 {
		t.Fatalf("the commands called the model %d times", model.calls)
	}
}

func TestCancelDropsWhatIsWaiting(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{responses: []*agent.Response{
		skill("c1", skillTrader, map[string]any{"action": "stop", "target_ref": map[string]any{"name": "ETH Grid"}}),
		text("held"),
	}})
	a.Run(context.Background(), "stop", nil)
	reply, ok := a.CancelPending()
	if !ok || !strings.Contains(reply.Text, "Cancelled") || a.pending != nil || api.called("POST /api/traders/t2/stop") {
		t.Fatalf("ok=%v reply=%q", ok, reply.Text)
	}
	if _, ok := a.CancelPending(); ok {
		t.Fatal("nothing should be left to cancel")
	}
}

func TestTruncateNeverCutsACharacter(t *testing.T) {
	for n := 1; n < 9; n++ {
		cut := truncate("交易员交易员", n)
		if !utf8.ValidString(cut) {
			t.Fatalf("n=%d gave %q", n, cut)
		}
	}
}

// summaryClient counts summaries and answers each with a short one.
type summaryClient struct {
	mockClient
	summaries int
	lastInput string
}

func (c *summaryClient) CallWithMessages(_, input string) (string, error) {
	c.summaries++
	c.lastInput = input
	return "summary " + fmt.Sprint(c.summaries), nil
}

func TestMemoryStaysBoundedAndCompactsOnlyWhenAsked(t *testing.T) {
	client := &summaryClient{}
	m := NewMemory(client)
	long := strings.Repeat("x", compactionThresholdTokens*charsPerToken)
	m.Add("user", long)
	m.Add("assistant", long)
	if client.summaries != 0 {
		t.Fatal("adding a message should not call the model")
	}
	m.Tidy()
	if client.summaries != 1 || m.LongTerm != "summary 1" || len(m.ShortTerm) != 0 {
		t.Fatalf("summaries=%d long=%q short=%d", client.summaries, m.LongTerm, len(m.ShortTerm))
	}
	// A summary takes in the last one and replaces it, and nothing more is due.
	m.Tidy()
	if client.summaries != 1 {
		t.Fatal("nothing new should mean no new summary")
	}
	m.Add("user", long)
	m.Add("assistant", long)
	m.Tidy()
	if m.LongTerm != "summary 2" || !strings.Contains(client.lastInput, "summary 1") {
		t.Fatalf("long=%q input held the last summary: %v", m.LongTerm, strings.Contains(client.lastInput, "summary 1"))
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
	reply := newAgent(port, model).Run(context.Background(), "loop", nil)
	if model.calls != maxIterations {
		t.Fatalf("expected %d model calls, got %d", maxIterations, model.calls)
	}
	if !strings.Contains(reply.Text, "did not finish") {
		t.Fatalf("a turn cut short must say so: %q", reply.Text)
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
