package assistant

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
	"/api/traders": `[{"trader_id":"t1","trader_name":"BTC Trend","running":false,"kind":"agent"},
		{"trader_id":"t2","trader_name":"ETH Grid","running":true,"kind":"grid"},
		{"trader_id":"t3","trader_name":"ADA Scalp","running":true,"kind":"agent"},
		{"trader_id":"t4","trader_name":"XRP Grid","running":false,"kind":"grid"}]`,
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
	auth.SetJWTSecret("test-secret-for-the-assistant")
	issued := 0
	token := func() (string, error) {
		issued++
		return CallToken("user")
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
		"/api/traders":  `[{"trader_id":"t2","trader_name":"ETH Grid","running":true,"kind":"grid","account_id":"ex2"},{"trader_id":"t3","trader_name":"ADA Scalp","running":true,"kind":"agent","account_id":"ex1"}]`,
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
		"/api/traders":  `[{"trader_id":"t2","trader_name":"ETH Grid","running":true,"kind":"grid","account_id":"ex1"}]`,
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
	if out["status"] != "needs_confirmation" || api.called("POST /api/traders") {
		t.Fatalf("creating should wait for a confirmation: %v", out)
	}
	line := a.pending.summary()
	for _, want := range []string{"SOL bot", "Main", "real money", "preset careful", "SOLUSDT"} {
		if !strings.Contains(line, want) {
			t.Fatalf("what is confirmed should say %q: %s", want, line)
		}
	}
	reply, live := a.Answer(a.pending.id, true)
	if !live || !strings.Contains(reply.Text, "SOL bot") || !reply.Changed {
		t.Fatalf("reply = %+v", reply)
	}
	if len(reply.Links) != 1 || reply.Links[0].To != "/traders?id=new-trader&tab=settings" {
		t.Fatalf("it should lead to the new trader: %+v", reply.Links)
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

func TestCreatingAndStartingAreTwoConfirmations(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{})
	a.runSkill(skillTrader, `{"action":"create","slots":{"name":"SOL bot","auto_start":true}}`)
	if len(a.pending.actions) != 1 {
		t.Fatalf("only creating should wait first: %s", a.pending.summary())
	}
	reply, _ := a.Answer(a.pending.id, true)
	if !api.called("POST /api/traders") || api.called("POST /api/traders/new-trader/start") {
		t.Fatal("it should exist, and not be running yet")
	}
	if reply.Confirm == nil || !strings.Contains(reply.Confirm.Lines[0], "start trader") {
		t.Fatalf("starting it should wait next: %+v", reply)
	}
	a.Answer(reply.Confirm.ID, true)
	if !api.called("POST /api/traders/new-trader/start") {
		t.Fatal("confirming should start it")
	}
}

func TestAFailedCreateSaysWhyAndLeadsNowhere(t *testing.T) {
	api, port, stop := newFakeAPI(account)
	defer stop()
	api.refuse("POST /api/traders")
	a := newAgent(port, &mockClient{})
	a.runSkill(skillTrader, `{"action":"create","slots":{"name":"SOL bot"}}`)
	reply, _ := a.Answer(a.pending.id, true)
	if !strings.Contains(reply.Text, "refused") || len(reply.Links) != 0 {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestOnlyWhatCannotBeTakenBackIsMarkedSo(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{})
	a.runSkill(skillTrader, `{"action":"start","target_ref":{"name":"BTC Trend"}}`)
	if a.confirmation().Destructive {
		t.Fatal("starting a trader can be undone")
	}
	a.runSkill(skillTrader, `{"action":"delete","target_ref":{"name":"BTC Trend"}}`)
	if !a.confirmation().Destructive {
		t.Fatal("a batch with a delete in it cannot all be undone")
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
	if reply, results := runPending(a.takePending(), false); !strings.HasPrefix(reply, "✅ Done") || !api.called("PUT /api/traders/t1") ||
		len(results) != 1 || !results[0].OK || results[0].Item == nil || results[0].Item.Action != actionUpdate {
		t.Fatalf("confirmed reply = %q, results = %+v", reply, results)
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
	for _, tool := range skillTools {
		if name := tool.Function.Name; strings.Contains(name, "strategy") {
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
	if reply, _ := runPending(a.takePending(), false); !strings.HasPrefix(reply, "✅ Done") {
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
		"/api/futures/state":   `{"records":{"a":{"status":"draft","plan":` + plan("Draft BTC", "BTCUSDT") + `},"b":{"status":"completed","plan":` + plan("Done", "BTCUSDT") + `},"c":{"status":"draft","plan":` + plan("Draft ETH", "ETHUSDT") + `}},"runs":[{"id":"r1","account_id":"ex1","status":"needs_attention","error":"check it","plan":` + plan("Live BTC", "BTCUSDT") + `,"orders":[{"role":"entry","status":"FILLED","filled":"0.1"}]}]}`,
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
	if runs.Total != 1 || runs.Shown[0]["name"] != "Live BTC" || runs.Shown[0]["status"] != "needs_attention" || runs.Shown[0]["error"] != "check it" || runs.Shown[0]["testnet"] != true {
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
		"/api/forecast/signals":        `{"coins":[{"symbol":"BTCUSDT","signal":{"display_prob":61}}]}`,
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
	// In parts as well: what was dropped, and why the turn ended, for a surface to lay out.
	if reply.Ended != EndStopped || reply.Dropped == nil || reply.Dropped.Expired || len(reply.Dropped.Items) != 1 || reply.Dropped.Items[0].Target != "old action" {
		t.Fatalf("reply parts = %+v", reply)
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
		"/api/traders":                `[{"trader_id":"t1","trader_name":"Trend","running":true,"kind":"agent"},{"trader_id":"t2","trader_name":"Grid","running":false,"kind":"grid"}]`,
		"/api/accounts":               `[{"id":"ex1","venue":"binance","name":"Main"},{"id":"ex2","venue":"binance","name":"Practice","testnet":true},{"id":"ex3","venue":"okx","name":"Broken"}]`,
		"/api/accounts/state":         `{"states":{"ex1":{"status":"ok","total_equity":1000.5},"ex2":{"status":"ok","total_equity":50},"ex3":{"status":"invalid_credentials"}}}`,
		"/api/accounts/ex1/positions": `[{"symbol":"BTCUSDT","side":"long","positionAmt":"0.5","entryPrice":"70000","unRealizedProfit":"12.345"},{"symbol":"ETHUSDT","side":"SHORT","quantity":-2,"entry_price":3000,"unrealized_pnl":-4}]`,
		"/api/accounts/ex2/positions": `[]`,
	}
	_, port, stop := newFakeAPI(gets)
	defer stop()
	model := &mockClient{}
	a := newAgent(port, model)

	holdings, err := a.Holdings()
	if err != nil || len(holdings) != 3 {
		t.Fatalf("holdings = %+v, %v", holdings, err)
	}
	main, practice, broken := holdings[0], holdings[1], holdings[2]
	if main.Name != "Main" || main.Testnet || !main.EquityRead || main.Equity != 1000.5 || !main.PositionsRead || len(main.Positions) != 2 {
		t.Fatalf("main = %+v", main)
	}
	if want := (Position{Symbol: "BTCUSDT", Side: "long", Quantity: 0.5, Entry: 70000, Gain: 12.345, HasGain: true}); main.Positions[0] != want {
		t.Fatalf("a venue's names were not read: %+v", main.Positions[0])
	}
	if want := (Position{Symbol: "ETHUSDT", Side: "short", Quantity: 2, Entry: 3000, Gain: -4, HasGain: true}); main.Positions[1] != want {
		t.Fatalf("a short's quantity should read as its size: %+v", main.Positions[1])
	}
	if !practice.Testnet || !practice.PositionsRead || len(practice.Positions) != 0 || broken.EquityRead || broken.PositionsRead {
		t.Fatalf("practice = %+v, broken = %+v", practice, broken)
	}
	traders, err := a.TraderStates()
	if err != nil || len(traders) != 2 || traders[0] != (TraderState{Name: "Trend", Kind: "agent", Running: true}) || traders[1].Running {
		t.Fatalf("traders = %+v, %v", traders, err)
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

func (c *summaryClient) CallWithRequest(req *agent.Request) (string, error) {
	c.summaries++
	c.lastInput = req.Messages[len(req.Messages)-1].Content
	return "summary " + fmt.Sprint(c.summaries), nil
}

func TestMemoryStaysBoundedAndCompactsOnlyWhenAsked(t *testing.T) {
	client := &summaryClient{}
	m := NewMemory(func() agent.Client { return client })
	long := strings.Repeat("x", compactionThresholdTokens*charsPerToken)
	m.Add("user", long)
	m.Add("assistant", long)
	if client.summaries != 0 {
		t.Fatal("adding a message should not call the model")
	}
	m.Tidy("conversation")
	if client.summaries != 1 || m.LongTerm != "summary 1" || len(m.ShortTerm) != 0 {
		t.Fatalf("summaries=%d long=%q short=%d", client.summaries, m.LongTerm, len(m.ShortTerm))
	}
	// A summary takes in the last one and replaces it, and nothing more is due.
	m.Tidy("conversation")
	if client.summaries != 1 {
		t.Fatal("nothing new should mean no new summary")
	}
	m.Add("user", long)
	m.Add("assistant", long)
	m.Tidy("conversation")
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

func TestWhatAPageShowsLeavesOutWhatIsKeptForTheModel(t *testing.T) {
	said := func(role, content string) agent.ConversationMessage {
		return agent.ConversationMessage{Role: role, Content: content}
	}
	got := ForReading([]agent.ConversationMessage{
		said("user", "stop it"),
		said("assistant", "ok\n\n"+waitingMark+"\nstop trader \"a\"]"),
		said("user", pressedConfirm),
		said("assistant", doneEn+"stop trader \"a\"\n"+failedEn+"stop trader \"b\"\nit refused"),
		said("user", "again"),
		said("user", "again"),
		said("assistant", waitingMark+"\nsomething]"),
		said("user", cancelledMark),
		said("assistant", "⌛ That ran out of time"),
	})
	want := []string{"stop it", "ok", "", "again", "That ran out of time"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Content != want[i] {
			t.Fatalf("%d: got %q, want %q", i, got[i].Content, want[i])
		}
	}
	results := got[2].Results
	if len(results) != 2 || !results[0].OK || results[0].Text != `stop trader "a"` || results[1].OK || results[1].Note != "it refused" {
		t.Fatalf("results = %+v", results)
	}
}

func TestOnlyWhatRunPendingWroteIsAResult(t *testing.T) {
	text, results := SplitResults("✅ Looks good to me\nDone: nothing here")
	if len(results) != 0 || !strings.HasPrefix(text, "✅ Looks good") {
		t.Fatalf("a reply of the model's should stay one: %q %+v", text, results)
	}
	text, results = SplitResults(doneZh + "启动交易员“甲”\n注意：余额不足")
	if text != "" || len(results) != 1 || results[0].Text != "启动交易员“甲”" || results[0].Note != "注意：余额不足" {
		t.Fatalf("%q %+v", text, results)
	}
}

func TestAHeldActionComesInParts(t *testing.T) {
	gets := map[string]string{}
	for path, body := range account {
		gets[path] = body
	}
	gets["/api/accounts"] = `[{"id":"ex1","venue":"binance","name":"Main"},{"id":"ex2","venue":"binance","name":"Test","testnet":true}]`
	gets["/api/traders"] = `[{"trader_id":"t1","trader_name":"BTC Trend","running":true,"kind":"grid","account_id":"ex1"},
		{"trader_id":"t2","trader_name":"Paper","running":false,"kind":"grid","account_id":"ex2"}]`
	_, port, stop := newFakeAPI(gets)
	defer stop()
	a := newAgent(port, &mockClient{})

	a.runSkill(skillTrader, `{"action":"start","target_ref":{"name":"Paper"}}`)
	a.runSkill(skillTrader, `{"action":"delete","target_ref":{"name":"BTC Trend"}}`)
	a.runSkill(skillTrader, `{"action":"update","target_ref":{"name":"BTC Trend"},"slots":{"config_patch":{"risk_control":{"max_positions":2},"coin_source":{"static_coins":["BTCUSDT"]}}}}`)
	a.runSkill(skillTrader, `{"action":"create","slots":{"name":"New","account":"Test","model":"DeepSeek","kind":"grid","config_patch":{"grid_count":8}}}`)
	got := a.confirmation()
	if !got.Destructive || len(got.Items) != 4 || len(got.Lines) != 4 {
		t.Fatalf("confirmation = %+v", got)
	}
	start, del, update, create := got.Items[0], got.Items[1], got.Items[2], got.Items[3]
	if start.Action != actionStart || start.Target != "Paper" || start.Account != "Test" || start.Money != moneyTestnet || start.Destructive {
		t.Fatalf("start = %+v", start)
	}
	if del.Action != actionDelete || del.Money != moneyReal || !del.Destructive || len(del.Details) != 1 {
		t.Fatalf("delete = %+v", del)
	}
	if strings.Join(update.Details[:2], "|") != `coin_source.static_coins: ["BTCUSDT"]|risk_control.max_positions: 2` {
		t.Fatalf("a patch should read key by key: %+v", update.Details)
	}
	if !strings.Contains(strings.Join(create.Details, "|"), "grid_count: 8") || strings.Contains(strings.Join(create.Details, "|"), "{") {
		t.Fatalf("no JSON in the parts: %+v", create.Details)
	}
	// What Telegram reads says the config key by key too.
	if !strings.Contains(got.Lines[3], "config grid_count: 8") || strings.Contains(got.Lines[3], "{") {
		t.Fatalf("lines = %+v", got.Lines)
	}

	// An account that cannot be read says nothing of its money, rather than guess.
	a2 := newAgent(port, &mockClient{})
	a2.hold("old action", func() (string, error) { return "", nil })
	if item := a2.confirmation().Items[0]; item.Target != "old action" || item.Action != "" {
		t.Fatalf("item = %+v", item)
	}
}

func TestEachSurfaceOffersItsOwnSkills(t *testing.T) {
	if !Telegram.offers(skillClose) || Web.offers(skillClose) {
		t.Fatal("only Telegram closes positions")
	}
	if !Web.offers(skillFillForm) || !Web.offers(skillOpenPage) || Telegram.offers(skillFillForm) || Telegram.offers(skillOpenPage) {
		t.Fatal("what needs a page is only for the web")
	}
	if len(Web.Tools) != len(Telegram.Tools)-1+len(webTools) {
		t.Fatal("the web should lack nothing else")
	}
	telegram, web := BuildAgentPrompt("me@example.com", Telegram), BuildAgentPrompt("me@example.com", Web)
	if !strings.Contains(telegram, "- close_position:") || strings.Contains(web, "- close_position:") {
		t.Fatal("the prompt should offer what the surface does")
	}
	if !strings.Contains(telegram, "Telegram") || strings.Contains(web, "Telegram") {
		t.Fatal("the prompt should say where replies are read")
	}
}

func readSkill(t *testing.T, a *Agent, name string, args any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(args)
	var out map[string]any
	if err := json.Unmarshal([]byte(a.runSkill(name, string(raw))), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestForecastQueryListsTheStrongestCallsFirstAndLeavesTheCandlesOut(t *testing.T) {
	_, port, stop := newFakeAPI(map[string]string{
		"/api/forecast/signals": `{"strategy_id":"s1","interval":"1h","coins":[
			{"symbol":"AAAUSDT","signal":{"direction":"up","display_prob":55}},
			{"symbol":"BBBUSDT","pending":"Not enough history"},
			{"symbol":"CCCUSDT","signal":{"direction":"down","display_prob":20}}]}`,
		"/api/forecast/detail/BTCUSDT": `{"symbol":"BTCUSDT","latest":{"direction":"up"},"stats":{"moments":12},
			"signals":[` + strings.Repeat(`{"outcome":"hit"},`, 30) + `{"outcome":"miss"}],
			"candles":[{"close":1}],"forming":{"close":1},"projection":[{"close":1}],"indicators":{"rsi":[1]}}`,
	})
	defer stop()
	a := newAgent(port, &mockClient{})

	out := readSkill(t, a, skillForecast, map[string]any{"scope": "signals", "interval": "1h"})
	coins := out["coins"].([]any)
	order := []string{}
	for _, coin := range coins {
		order = append(order, coin.(map[string]any)["symbol"].(string))
	}
	if strings.Join(order, " ") != "CCCUSDT AAAUSDT BBBUSDT" {
		t.Fatalf("order = %v", order)
	}
	if !strings.Contains(fmt.Sprint(out["note"]), "chance_rate") {
		t.Fatalf("what it says should come with how to read it: %v", out)
	}

	detail := readSkill(t, a, skillForecast, map[string]any{"scope": "detail", "symbol": "btcusdt"})
	data := detail["data"].(map[string]any)
	for _, heavy := range []string{"candles", "forming", "projection", "indicators"} {
		if _, there := data[heavy]; there {
			t.Fatalf("%s should be left out", heavy)
		}
	}
	if signals := data["signals"].([]any); len(signals) != recentSignalsShown {
		t.Fatalf("%d recent calls", len(signals))
	}
	if data["stats"] == nil || data["latest"] == nil {
		t.Fatalf("what the call came to should stay: %v", data)
	}
}

func TestForecastPerformanceGoesByTheStrategyInUseUnlessOneIsNamed(t *testing.T) {
	gets := map[string]string{
		"/api/forecast/strategies": `{"active_strategy_id":"s2","strategies":[
			{"id":"s1","name":"Old","retired":true,"config":{"horizon":4}},
			{"id":"s2","name":"Current","config":{"horizon":8}}],"default_config":{"horizon":1}}`,
		"/api/forecast/strategies/s1/performance": `{"overall":{"moments":3}}`,
		"/api/forecast/strategies/s2/performance": `{"overall":{"moments":40}}`,
		"/api/forecast/backtests": `{"backtests":[
			{"id":9,"strategy_id":"s2","status":"running"},
			{"id":8,"strategy_id":"s2","status":"done","summary":{"hit_rate":0.5}},
			{"id":7,"strategy_id":"s1","status":"done"}]}`,
	}
	_, port, stop := newFakeAPI(gets)
	defer stop()
	a := newAgent(port, &mockClient{})

	list := readSkill(t, a, skillForecast, map[string]any{"scope": "strategies"})
	if rows := list["strategies"].([]any); len(rows) != 2 || rows[1].(map[string]any)["in_use"] != true {
		t.Fatalf("strategies = %v", list)
	}
	if strings.Contains(fmt.Sprint(list), "horizon") {
		t.Fatalf("listing them should leave their rules out: %v", list)
	}

	out := readSkill(t, a, skillForecast, map[string]any{"scope": "performance"})
	strategy := out["strategy"].(map[string]any)
	if strategy["id"] != "s2" || strategy["rules"].(map[string]any)["horizon"] != float64(8) {
		t.Fatalf("strategy = %v", strategy)
	}
	if out["latest_backtest"].(map[string]any)["id"] != float64(8) {
		t.Fatalf("a run still going is no result: %v", out["latest_backtest"])
	}

	named := readSkill(t, a, skillForecast, map[string]any{"scope": "performance", "strategy_ref": map[string]any{"name": "old"}})
	if named["strategy"].(map[string]any)["id"] != "s1" {
		t.Fatalf("named = %v", named)
	}
	gone := readSkill(t, a, skillForecast, map[string]any{"scope": "performance", "strategy_ref": map[string]any{"name": "nothing like it"}})
	if gone["error"] != "not_found" {
		t.Fatalf("it should not guess: %v", gone)
	}
	if out := readSkill(t, a, skillForecast, map[string]any{"scope": "backtest"}); out["error"] != "missing_slots" {
		t.Fatalf("a run needs naming: %v", out)
	}
}

func TestFuturesCompareLaysOutTheCompilersFigures(t *testing.T) {
	soon := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	gone := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	plan := func(id, side, expires string) string {
		return fmt.Sprintf(`{"plan":{"id":%q,"quantity":"1","risk":"10","reward":"25","reward_risk":"2.5","created":%q,"expires":%q,
			"idea":{"name":%q,"side":%q,"entry":"100","stop":"95","target":"110","rationale":"because","execution":{"kind":"limit"}},
			"snapshot":{"symbol":"BTCUSDT","venue":"binance"}},"status":"draft"}`, id, soon, expires, "plan "+id, side)
	}
	f, port, stop := newFakeAPI(map[string]string{
		"/api/futures/state": `{"records":{"p1":` + plan("p1", "long", soon) + `,"p2":` + plan("p2", "short", soon) + `,"p3":` + plan("p3", "long", gone) + `},"runs":[]}`,
	})
	defer stop()
	a := newAgent(port, &mockClient{})

	out := readSkill(t, a, skillFutures, map[string]any{"scope": "compare", "plan_ids": []string{"p2", "p1", "nope"}})
	rows := out["plans"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["id"] != "p2" {
		t.Fatalf("plans should keep the order asked for: %v", rows)
	}
	first := rows[0].(map[string]any)
	if first["kind"] != "limit" || first["risk"] != "10" || first["reward_risk"] != "2.5" || first["rationale"] != "because" {
		t.Fatalf("row = %v", first)
	}
	if missed := out["not_found"].([]any); len(missed) != 1 || missed[0] != "nope" {
		t.Fatalf("what it could not find should be said: %v", out)
	}

	// Without any named it is the drafts still waiting, and never one that ran out.
	waiting := readSkill(t, a, skillFutures, map[string]any{"scope": "compare"})
	if rows := waiting["plans"].([]any); len(rows) != 2 {
		t.Fatalf("waiting = %v", rows)
	}
	asked := readSkill(t, a, skillFutures, map[string]any{"scope": "compare", "plan_ids": []string{"p3"}})
	if asked["plans"].([]any)[0].(map[string]any)["expired"] != true {
		t.Fatalf("a plan that ran out should say so: %v", asked)
	}
	if f.called("POST /api/futures/compile") || f.called("POST /api/futures/approve") {
		t.Fatal("comparing should only read")
	}
}

func TestTheWholeSystemIsOneRead(t *testing.T) {
	_, port, stop := newFakeAPI(map[string]string{"/api/dashboard/overview": `{"attention":[{"kind":"service_failed"}]}`})
	defer stop()
	a := newAgent(port, &mockClient{})
	out := readSkill(t, a, skillAccount, map[string]any{"scope": "system"})
	if !strings.Contains(fmt.Sprint(out["data"]), "service_failed") {
		t.Fatalf("out = %v", out)
	}
}

func TestAPageGoesWithTheTurnItWasAskedFrom(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{}
	a := newAgent(port, model)
	a.Run(WithPage(context.Background(), `{"page":"traders","trader":{"id":"t1"}}`), "how is this one doing?", nil)
	if prompt := model.firstContent(); !strings.Contains(prompt, "[Current page") || !strings.Contains(prompt, `"id":"t1"`) {
		t.Fatalf("prompt = %q", prompt)
	}
	a.Run(context.Background(), "and anything else?", nil)
	if prompt := model.firstContent(); strings.Contains(prompt, "[Current page") {
		t.Fatalf("a page belongs to its own turn: %q", prompt)
	}
	long := strings.Repeat("x", maxPageChars*2)
	if len(pageContext(WithPage(context.Background(), long))) > maxPageChars+200 {
		t.Fatal("what a page says should be bounded")
	}
}

const pageWithAForm = `{"page":"forecast_backtests","forms":[{"id":"forecast.backtest","title":"New backtest","fields":[
	{"name":"interval","type":"choice","options":[{"value":"1h"},{"value":"4h"}],"value":"1h"},
	{"name":"horizon","type":"number","min":1,"max":10,"value":4},
	{"name":"name","type":"text","value":""},
	{"name":"coins","type":"list","value":["BTCUSDT"]},
	{"name":"config","type":"object","value":{"risk":{"max":2,"on":true},"coins":["BTCUSDT"]}},
	{"name":"odd","type":"secret","value":""}]}]}`

func webAgent(port int, model *mockClient) *Agent {
	a := newAgent(port, model)
	a.tools = Web.Tools
	a.page = pageWithAForm
	return a
}

func TestFillFormShowsOnlyWhatTheFormDeclares(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := webAgent(port, &mockClient{})
	try := func(args map[string]any) map[string]any { return readSkill(t, a, skillFillForm, args) }

	if out := try(map[string]any{"form": "trader.settings", "values": map[string]any{"name": "x"}}); out["error"] != "form_not_open" {
		t.Fatalf("a form not on the page: %v", out)
	}
	if out := try(map[string]any{"form": "forecast.backtest"}); out["error"] != "missing_slots" {
		t.Fatalf("nothing to fill: %v", out)
	}
	for name, values := range map[string]map[string]any{
		"unknown field":    {"password": "x"},
		"not an option":    {"interval": "5m"},
		"not a number":     {"horizon": "4"},
		"above the most":   {"horizon": 11},
		"below the least":  {"horizon": 0},
		"not text":         {"name": 3},
		"not a list":       {"coins": "BTCUSDT"},
		"list of numbers":  {"coins": []any{1}},
		"not an object":    {"config": 2},
		"deep type":        {"config": map[string]any{"risk": map[string]any{"max": "two"}}},
		"deep shape":       {"config": map[string]any{"risk": 3}},
		"cannot be filled": {"odd": "x"},
	} {
		out := try(map[string]any{"form": "forecast.backtest", "values": values})
		if out["error"] != "invalid_values" {
			t.Errorf("%s: %v", name, out)
		}
	}
	if len(a.proposals) != 0 {
		t.Fatalf("nothing refused should reach the user: %+v", a.proposals)
	}

	out := try(map[string]any{"form": "forecast.backtest", "reason": "longer is steadier", "values": map[string]any{
		"interval": "4h", "horizon": 8, "name": "Long", "coins": []any{"ETHUSDT"},
		"config": map[string]any{"risk": map[string]any{"max": 3}, "new_key": "left to the form"},
	}})
	if out["status"] != "shown_to_user" {
		t.Fatalf("a good suggestion: %v", out)
	}
	// A second try replaces the first, and the reply carries it once.
	try(map[string]any{"form": "forecast.backtest", "reason": "shorter after all", "values": map[string]any{"interval": "1h"}})
	reply := a.finish("q", "here", "")
	if len(reply.Proposals) != 1 || reply.Proposals[0].Reason != "shorter after all" || reply.Proposals[0].Title != "New backtest" {
		t.Fatalf("proposals = %+v", reply.Proposals)
	}
	if next := a.finish("q", "again", ""); len(next.Proposals) != 0 {
		t.Fatal("a suggestion belongs to its own reply")
	}
}

func TestFillFormNeedsAPage(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := webAgent(port, &mockClient{})
	a.page = ""
	if out := readSkill(t, a, skillFillForm, map[string]any{"form": "forecast.backtest", "values": map[string]any{"name": "x"}}); out["error"] != "form_not_open" {
		t.Fatalf("out = %v", out)
	}
	a.page = "{not json"
	if out := readSkill(t, a, skillFillForm, map[string]any{"form": "forecast.backtest", "values": map[string]any{"name": "x"}}); out["error"] != "form_not_open" {
		t.Fatalf("out = %v", out)
	}
}

func TestOpenPageLeadsOnlyWhereTheTableSays(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := webAgent(port, &mockClient{})
	try := func(args map[string]any) map[string]any { return readSkill(t, a, skillOpenPage, args) }

	if out := try(map[string]any{"page": "https://example.com"}); out["error"] != "unknown_page" {
		t.Fatalf("out = %v", out)
	}
	if out := try(map[string]any{"page": "traders", "params": map[string]any{"next": "https://example.com"}}); out["error"] != "unknown_param" {
		t.Fatalf("out = %v", out)
	}
	if len(a.links) != 0 {
		t.Fatalf("nothing refused should become a button: %+v", a.links)
	}
	try(map[string]any{"page": "traders", "params": map[string]any{"id": "a b&c", "tab": "settings"}})
	try(map[string]any{"page": "settings_models"})
	reply := a.finish("q", "there", "")
	if len(reply.Links) != 2 || reply.Links[0].To != "/traders?id=a+b%26c&tab=settings" || reply.Links[1].To != "?settings=models" {
		t.Fatalf("links = %+v", reply.Links)
	}
}

func TestWhatNeedsAPageIsNotForTelegram(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	a := newAgent(port, &mockClient{})
	a.tools = Telegram.Tools
	a.page = pageWithAForm
	for _, skill := range []string{skillFillForm, skillOpenPage} {
		if out := a.runSkill(skill, `{"form":"forecast.backtest","values":{"name":"x"},"page":"dashboard"}`); !strings.Contains(out, "unknown_skill") {
			t.Fatalf("%s = %s", skill, out)
		}
	}
}

// chatsAPI serves a user's conversations across features, with the one in use among them.
func chatsAPI() map[string]string {
	gets := map[string]string{}
	for path, body := range account {
		gets[path] = body
	}
	gets["/api/conversations"] = `[
		{"id":"c1","feature":"futures","title":"BTC pullback","account_id":"ex1","symbol":"BTCUSDT","updated_at":"2026-11-02T10:00:00Z"},
		{"id":"c2","feature":"assistant","title":"BTC summary","updated_at":"2026-11-02T09:00:00Z"},
		{"id":"c3","feature":"telegram","title":"","updated_at":"2026-11-01T09:00:00Z"},
		{"id":"here","feature":"assistant","title":"Delete my chats","updated_at":"2026-11-03T09:00:00Z"}]`
	return gets
}

func TestDeletingConversationsResolvesEachAndWaitsForConfirmation(t *testing.T) {
	api, port, stop := newFakeAPI(chatsAPI())
	defer stop()
	a := newAgent(port, &mockClient{})
	a.conversationID.Store("here")

	// A name that fits two, or none, holds nothing, and neither does the conversation in use.
	for _, ref := range []map[string]any{{"name": "btc"}, {"name": "eth"}, {"id": "here"}} {
		out := readSkill(t, a, skillChats, map[string]any{"action": "delete", "targets": []any{map[string]any{"name": "BTC pullback"}, ref}})
		if out["error"] == nil || a.pending != nil {
			t.Fatalf("%v = %v, pending %v", ref, out, a.pending)
		}
	}
	if out := readSkill(t, a, skillChats, map[string]any{"action": "delete"}); out["error"] != "missing_slots" {
		t.Fatalf("no targets = %v", out)
	}

	out := readSkill(t, a, skillChats, map[string]any{"action": "delete", "targets": []any{map[string]any{"name": "btc pullback"}, map[string]any{"id": "c3"}}})
	if out["status"] != "needs_confirmation" || api.called("DELETE /api/conversations") {
		t.Fatalf("delete = %v", out)
	}
	got := a.confirmation()
	if !got.Destructive || len(got.Items) != 2 {
		t.Fatalf("confirmation = %+v", got)
	}
	futures, telegram := got.Items[0], got.Items[1]
	if futures.Action != actionDeleteChat || futures.Target != "BTC pullback" || futures.Account != "Main" || futures.Money != "" || !strings.Contains(strings.Join(futures.Details, "|"), "plans that never ran") {
		t.Fatalf("futures = %+v", futures)
	}
	// A chat with no title goes by where and when it was.
	if !strings.HasPrefix(telegram.Target, "Telegram 2026-11-01") || strings.Contains(strings.Join(telegram.Details, "|"), "plans") {
		t.Fatalf("telegram = %+v", telegram)
	}

	a.Run(context.Background(), "confirm", nil)
	if ids := api.body("DELETE /api/conversations")["ids"]; fmt.Sprint(ids) != "[c3]" {
		t.Fatalf("the last request deleted %v", ids)
	}
}

func TestClearingConversationsTakesWhatIsListedButTheOneInUse(t *testing.T) {
	api, port, stop := newFakeAPI(chatsAPI())
	defer stop()
	a := newAgent(port, &mockClient{})
	a.conversationID.Store("here")

	out := readSkill(t, a, skillChats, map[string]any{"action": "query"})
	if out["total"] != float64(4) {
		t.Fatalf("query = %v", out)
	}
	if shown := out["shown"].([]any); shown[3].(map[string]any)["current"] != true || shown[0].(map[string]any)["account"] != "Main" {
		t.Fatalf("shown = %v", shown)
	}

	readSkill(t, a, skillChats, map[string]any{"action": "clear", "account_ref": map[string]any{"name": "main"}, "feature": "futures"})
	if !api.queried("/api/conversations", "account_id=ex1&feature=futures") {
		t.Fatal("the filters were not asked of the API")
	}
	a.pending = nil

	readSkill(t, a, skillChats, map[string]any{"action": "clear"})
	got := a.confirmation()
	if len(got.Items) != 1 {
		t.Fatalf("confirmation = %+v", got)
	}
	item := got.Items[0]
	if item.Action != actionDeleteChats || item.Target != "3" || !strings.Contains(strings.Join(item.Details, "|"), "this conversation stays") || !strings.Contains(got.Lines[0], "Futures 1, Assistant 1, Telegram 1") {
		t.Fatalf("clear = %+v / %v", item, got.Lines)
	}
	if api.called("DELETE /api/conversations") {
		t.Fatal("deleted before confirmation")
	}
	a.Run(context.Background(), "确认", nil)
	if ids := api.body("DELETE /api/conversations")["ids"]; fmt.Sprint(ids) != "[c1 c2 c3]" {
		t.Fatalf("cleared %v", ids)
	}
	if out := readSkill(t, a, skillChats, map[string]any{"action": "query", "feature": "trader"}); out["error"] != "unknown_feature" {
		t.Fatalf("a trader's cycles = %v", out)
	}
}

// goneHistory is a chat history whose conversation was deleted from elsewhere.
type goneHistory struct {
	started int
	exists  bool
}

func (h *goneHistory) Resume(string, int) (string, []Message) {
	return "old", []Message{{Role: "user", Content: "what was said"}}
}
func (h *goneHistory) Start(string) string                  { h.started++; return "new" }
func (h *goneHistory) Append(string, string, agent.Message) {}
func (h *goneHistory) Exists(string) bool                   { return h.exists }

func TestAChatWhoseConversationWasDeletedStartsAfresh(t *testing.T) {
	_, port, stop := newFakeAPI(account)
	defer stop()
	model := &mockClient{responses: []*agent.Response{text("hi")}}
	a := newAgent(port, model)
	history := &goneHistory{exists: true}
	a.resume(history, "chat")
	a.hold("stop a trader", func() (string, error) { return "", nil })

	history.exists = false
	a.Run(context.Background(), "hello", nil)
	if a.conversation() != "new" || history.started != 1 || a.pending != nil {
		t.Fatalf("conversation %q, started %d, pending %v", a.conversation(), history.started, a.pending)
	}
	if strings.Contains(model.firstContent(), "what was said") {
		t.Fatal("the model was given what the deleted conversation held")
	}
}

// plansAPI serves two finished plans under one name, one at work and a draft.
func plansAPI() map[string]string {
	gets := map[string]string{}
	for path, body := range account {
		gets[path] = body
	}
	plan := func(id, name, created string) string {
		return fmt.Sprintf(`{"id":%q,"created":%q,"idea":{"name":%q},"snapshot":{"symbol":"BTCUSDT"}}`, id, created, name)
	}
	fin1 := plan("fin1", "Manual BTCUSDT long", "2026-11-02T10:00:00Z")
	fin2 := plan("fin2", "Manual BTCUSDT long", "2026-11-01T10:00:00Z")
	work := plan("work", "Breakout", "2026-11-03T10:00:00Z")
	draft := plan("d1", "Pullback", "2026-10-30T10:00:00Z")
	gets["/api/futures/state"] = `{"records":{
		"fin1":{"plan":` + fin1 + `,"status":"completed"},"fin2":{"plan":` + fin2 + `,"status":"completed"},
		"work":{"plan":` + work + `,"status":"approved"},"d1":{"plan":` + draft + `,"status":"expired"}},
		"accounts":{"fin1":"ex1","fin2":"ex1","work":"ex1","d1":"ex1"},
		"runs":[{"id":"r1","account_id":"ex1","status":"completed","plan":` + fin1 + `},
			{"id":"r2","account_id":"ex1","status":"cancelled","plan":` + fin2 + `},
			{"id":"r3","account_id":"ex1","status":"running","plan":` + work + `}]}`
	return gets
}

func TestDeletingPlansTakesOnlyThoseDoneWith(t *testing.T) {
	api, port, stop := newFakeAPI(plansAPI())
	defer stop()
	a := newAgent(port, &mockClient{})

	// A plan at work, a name two plans share and one that fits none hold nothing.
	for _, ref := range []map[string]any{{"name": "breakout"}, {"name": "manual btcusdt long"}, {"name": "scalp"}} {
		if out := readSkill(t, a, skillPlans, map[string]any{"action": "delete", "targets": []any{ref}}); out["error"] == nil || a.pending != nil {
			t.Fatalf("%v = %v", ref, out)
		}
	}

	readSkill(t, a, skillPlans, map[string]any{"action": "delete", "targets": []any{map[string]any{"id": "fin1"}, map[string]any{"name": "pullback"}}})
	got := a.confirmation()
	if !got.Destructive || len(got.Items) != 2 || api.called("DELETE /api/futures/plans") {
		t.Fatalf("confirmation = %+v", got)
	}
	finished, draft := got.Items[0], got.Items[1]
	if finished.Action != actionDeletePlan || finished.Account != "Main" || !strings.Contains(strings.Join(finished.Details, "|"), "stay in the account") {
		t.Fatalf("finished = %+v", finished)
	}
	if !strings.Contains(strings.Join(draft.Details, "|"), "never sent an order") {
		t.Fatalf("draft = %+v", draft)
	}
	a.Run(context.Background(), "confirm", nil)
	if ids := api.body("DELETE /api/futures/plans")["ids"]; fmt.Sprint(ids) != "[d1]" {
		t.Fatalf("the last request deleted %v", ids)
	}

	if out := readSkill(t, a, skillPlans, map[string]any{"action": "clear"}); out["error"] != "missing_slots" {
		t.Fatalf("clear without a kind = %v", out)
	}
	readSkill(t, a, skillPlans, map[string]any{"action": "clear", "kind": "finished", "account_ref": map[string]any{"name": "Main"}})
	item := a.confirmation().Items[0]
	if item.Action != actionDeletePlans || item.Target != "2" || !strings.Contains(strings.Join(item.Details, "|"), "1 still at work stay") {
		t.Fatalf("clear = %+v", item)
	}
	a.Run(context.Background(), "确认", nil)
	if ids := api.body("DELETE /api/futures/plans")["ids"]; fmt.Sprint(ids) != "[fin1 fin2]" {
		t.Fatalf("cleared %v", ids)
	}
}
