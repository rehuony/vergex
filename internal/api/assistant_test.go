package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/security/auth"
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// assistantModel answers each call with the next of what it was given.
type assistantModel struct {
	mu       sync.Mutex
	rounds   []*agent.Response
	failure  error
	requests []*agent.Request
	// started and release hold a call back while a test does something meanwhile.
	started, release chan struct{}
}

func (m *assistantModel) SetAPIKey(string, string, string)                {}
func (m *assistantModel) SetTimeout(time.Duration)                        {}
func (m *assistantModel) CallWithMessages(string, string) (string, error) { return "", nil }
func (m *assistantModel) CallWithRequest(*agent.Request) (string, error)  { return "", nil }
func (m *assistantModel) CallWithRequestFull(req *agent.Request) (*agent.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if m.started != nil {
		close(m.started)
		m.started = nil
		m.mu.Unlock()
		<-m.release
		m.mu.Lock()
	}
	if m.failure != nil {
		return nil, m.failure
	}
	if len(m.rounds) == 0 {
		return &agent.Response{Content: "done"}, nil
	}
	round := m.rounds[0]
	m.rounds = m.rounds[1:]
	return round, nil
}

func (m *assistantModel) lastPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, message := range m.requests[len(m.requests)-1].Messages {
		if message.Role == "user" {
			return message.Content
		}
	}
	return ""
}

func skillCall(name, arguments string) agent.ToolCall {
	call := agent.ToolCall{ID: "call_" + name, Type: "function"}
	call.Function.Name, call.Function.Arguments = name, arguments
	return call
}

type assistantFixture struct {
	t      *testing.T
	stores *Stores
	server *httptest.Server
	token  string
	model  string
}

func assistantServerOn(t *testing.T, st *Stores) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New(), stores: st, traderManager: trader.NewManager(trader.NewStores(st.DB)), cryptoHandler: &CryptoHandler{}}
	s.setupRoutes()
	server := httptest.NewServer(s.router)
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if s.port, err = strconv.Atoi(address.Port()); err != nil {
		t.Fatal(err)
	}
	return server
}

// useModel makes every model client in this test the one given.
func useModel(t *testing.T, model agent.Client) {
	t.Helper()
	before := newModelClient
	newModelClient = func(string) agent.Client { return model }
	t.Cleanup(func() { newModelClient = before })
}

func assistantServer(t *testing.T) *assistantFixture {
	t.Helper()
	auth.SetJWTSecret("test-secret-for-the-assistant-api")
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	if err := st.Models.Save(user, "deepseek", "DeepSeek", true, "sk-test", "", ""); err != nil {
		t.Fatal(err)
	}
	model, err := st.Models.GetDefault(user)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateJWT(user, "me@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return &assistantFixture{t: t, stores: st, server: assistantServerOn(t, st), token: token, model: model.ID}
}

func (f *assistantFixture) send(method, path, token string, body any) *http.Response {
	f.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		f.t.Fatal(err)
	}
	req, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(string(raw)))
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

type assistantEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (f *assistantFixture) chat(conversation, message string) []assistantEvent {
	f.t.Helper()
	resp := f.send(http.MethodPost, "/api/assistant/chat", f.token, map[string]any{
		"conversation_id": conversation, "message": message, "agent_model_id": f.model, "language": "en",
	})
	if resp.StatusCode != http.StatusOK {
		f.t.Fatalf("chat answered %d", resp.StatusCode)
	}
	var events []assistantEvent
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
			var event assistantEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				f.t.Fatal(err)
			}
			events = append(events, event)
		}
	}
	return events
}

func (f *assistantFixture) read(path string, out any) int {
	f.t.Helper()
	resp := f.send(http.MethodGet, path, f.token, nil)
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			f.t.Fatal(err)
		}
	}
	return resp.StatusCode
}

type assistantReply struct {
	Text    string `json:"text"`
	Message string `json:"message"`
	Changed bool   `json:"changed"`
	Live    bool   `json:"live"`
	Confirm *struct {
		ID    string   `json:"id"`
		Lines []string `json:"lines"`
	} `json:"confirm"`
}

func eventOf(t *testing.T, events []assistantEvent, kind string) assistantReply {
	t.Helper()
	for _, event := range events {
		if event.Type == kind {
			var reply assistantReply
			if err := json.Unmarshal(event.Data, &reply); err != nil {
				t.Fatal(err)
			}
			return reply
		}
	}
	t.Fatalf("no %s among %v", kind, events)
	return assistantReply{}
}

func TestAnAssistantTurnStreamsWhatIsUnderWayAndIsKept(t *testing.T) {
	f := assistantServer(t)
	useModel(t, &assistantModel{rounds: []*agent.Response{
		{ToolCalls: []agent.ToolCall{skillCall("account_management", `{"action":"query"}`)}},
		{Content: "you have no accounts"},
	}})
	id := uuid.NewString()
	events := f.chat(id, "what do I have?")
	if events[len(events)-1].Type != "done" {
		t.Fatalf("the stream should end with done: %v", events)
	}
	if reply := eventOf(t, events, "reply"); reply.Text != "you have no accounts" || reply.Changed {
		t.Fatalf("reply = %+v", reply)
	}
	stages := 0
	for _, event := range events {
		if event.Type == "stage" {
			stages++
		}
	}
	if stages != 2 {
		t.Fatalf("thinking and reading accounts should each say so: %v", events)
	}

	// Telegram's conversations are not the assistant's to list.
	telegram := agent.Conversation{ID: uuid.NewString(), UserID: testdb.UserID, Feature: agent.FeatureTelegram, Context: map[string]string{"chat_id": "1"}}
	if err := f.stores.Conversations.Open(&telegram); err != nil {
		t.Fatal(err)
	}
	var list []assistantConversation
	if code := f.read("/api/assistant/conversations", &list); code != http.StatusOK {
		t.Fatalf("list answered %d", code)
	}
	if len(list) != 1 || list[0].ID != id || list[0].Title != "what do I have?" {
		t.Fatalf("list = %+v", list)
	}
	var opened struct {
		Messages []agent.ConversationMessage `json:"messages"`
	}
	if code := f.read("/api/assistant/conversations/"+id, &opened); code != http.StatusOK {
		t.Fatalf("open answered %d", code)
	}
	if len(opened.Messages) != 2 || opened.Messages[0].Content != "what do I have?" || opened.Messages[1].Content != "you have no accounts" {
		t.Fatalf("messages = %+v", opened.Messages)
	}
	if code := f.read("/api/assistant/conversations/"+telegram.ID, nil); code != http.StatusNotFound {
		t.Fatalf("a Telegram chat opened with %d", code)
	}
}

func TestTheWebAssistantClosesNoPositions(t *testing.T) {
	f := assistantServer(t)
	model := &assistantModel{rounds: []*agent.Response{
		{ToolCalls: []agent.ToolCall{skillCall("close_position", `{"symbol":"BTCUSDT","side":"LONG"}`)}},
		{Content: "I can't do that from here"},
	}}
	useModel(t, model)
	events := f.chat(uuid.NewString(), "close my BTC")
	if reply := eventOf(t, events, "reply"); reply.Confirm != nil {
		t.Fatalf("nothing should be held: %+v", reply)
	}
	for _, tool := range model.requests[0].Tools {
		if tool.Function.Name == "close_position" {
			t.Fatal("close_position should not be on offer")
		}
	}
	var result string
	for _, message := range model.requests[1].Messages {
		if message.Role == "tool" {
			result = message.Content
		}
	}
	if !strings.Contains(result, "unknown_skill") {
		t.Fatalf("the call should have been refused: %q", result)
	}
}

func TestAHeldActionWaitsForItsAnswer(t *testing.T) {
	f := assistantServer(t)
	held := testdb.AddTrader(t, f.stores.DB, &testdb.Trader{Name: "Grid one"})
	hold := func() *assistantModel {
		return &assistantModel{rounds: []*agent.Response{
			{ToolCalls: []agent.ToolCall{skillCall("trader_management", fmt.Sprintf(`{"action":"delete","target_ref":{"id":%q}}`, held.ID))}},
			{Content: "it is waiting for you"},
		}}
	}
	id := uuid.NewString()
	path := "/api/assistant/conversations/" + id
	exists := func() bool {
		rows, err := f.stores.Traders.List(testdb.UserID)
		if err != nil {
			t.Fatal(err)
		}
		return len(rows) == 1
	}
	answer := func(confirmation string, confirmed bool) assistantReply {
		t.Helper()
		resp := f.send(http.MethodPost, path+"/answer", f.token, map[string]any{"confirmation_id": confirmation, "confirmed": confirmed, "language": "en"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("answer answered %d", resp.StatusCode)
		}
		var reply assistantReply
		if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
			t.Fatal(err)
		}
		return reply
	}

	useModel(t, hold())
	waiting := eventOf(t, f.chat(id, "delete Grid one"), "reply").Confirm
	if waiting == nil || len(waiting.Lines) != 1 || !strings.Contains(waiting.Lines[0], "Grid one") {
		t.Fatalf("confirm = %+v", waiting)
	}
	if !exists() {
		t.Fatal("nothing should have happened yet")
	}

	// It survives the page being opened again, and only its own answer resolves it.
	var opened struct {
		Confirm *struct {
			ID string `json:"id"`
		} `json:"confirm"`
	}
	f.read(path, &opened)
	if opened.Confirm == nil || opened.Confirm.ID != waiting.ID {
		t.Fatalf("opening should say what waits: %+v", opened)
	}
	if reply := answer("something-older", true); reply.Live || !exists() {
		t.Fatalf("an old answer should do nothing: %+v", reply)
	}

	if reply := answer(waiting.ID, false); !reply.Live || reply.Changed || !exists() {
		t.Fatalf("cancelling should do nothing: %+v", reply)
	}
	opened.Confirm = nil
	f.read(path, &opened)
	if opened.Confirm != nil {
		t.Fatal("nothing should wait after cancelling")
	}

	useModel(t, hold())
	waiting = eventOf(t, f.chat(id, "delete Grid one"), "reply").Confirm
	reply := answer(waiting.ID, true)
	if !reply.Live || !reply.Changed || strings.HasPrefix(reply.Text, "✅") {
		t.Fatalf("confirming should run it and say so without a mark: %+v", reply)
	}
	if exists() {
		t.Fatal("confirming should have deleted the trader")
	}
}

func TestTheAssistantRunsOneTurnAtATime(t *testing.T) {
	f := assistantServer(t)
	model := &assistantModel{started: make(chan struct{}), release: make(chan struct{})}
	useModel(t, model)
	started := model.started
	id := uuid.NewString()
	done := make(chan []assistantEvent, 1)
	go func() { done <- f.chat(id, "first") }()
	<-started

	second := f.send(http.MethodPost, "/api/assistant/chat", f.token, map[string]any{
		"conversation_id": uuid.NewString(), "message": "second", "agent_model_id": f.model,
	})
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("a second turn answered %d", second.StatusCode)
	}
	if resp := f.send(http.MethodDelete, "/api/assistant/conversations/"+id, f.token, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("deleting what is under way answered %d", resp.StatusCode)
	}
	if resp := f.send(http.MethodPost, "/api/assistant/conversations/"+id+"/answer", f.token, map[string]any{"confirmation_id": "x"}); resp.StatusCode != http.StatusConflict {
		t.Fatalf("answering while busy answered %d", resp.StatusCode)
	}
	close(model.release)
	if events := <-done; eventOf(t, events, "reply").Text != "done" {
		t.Fatalf("the first turn should finish: %v", events)
	}
}

func TestAConversationIsItsOwnersAlone(t *testing.T) {
	f := assistantServer(t)
	useModel(t, &assistantModel{})
	id := uuid.NewString()
	f.chat(id, "mine")

	testdb.AddOtherUser(t, f.stores.DB)
	other, err := auth.GenerateJWT(testdb.OtherUserID, "other@example.com")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/assistant/conversations/" + id
	if resp := f.send(http.MethodGet, path, other, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("opening answered %d", resp.StatusCode)
	}
	if resp := f.send(http.MethodDelete, path, other, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting answered %d", resp.StatusCode)
	}
	if err := f.stores.Models.Save(testdb.OtherUserID, "deepseek", "DeepSeek", true, "sk-other", "", ""); err != nil {
		t.Fatal(err)
	}
	theirs, err := f.stores.Models.GetDefault(testdb.OtherUserID)
	if err != nil {
		t.Fatal(err)
	}
	resp := f.send(http.MethodPost, "/api/assistant/chat", other, map[string]any{"conversation_id": id, "message": "hello", "agent_model_id": theirs.ID})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("continuing it answered %d", resp.StatusCode)
	}
	// A model belongs to its user too.
	resp = f.send(http.MethodPost, "/api/assistant/chat", other, map[string]any{"conversation_id": uuid.NewString(), "message": "hello", "agent_model_id": f.model})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("using their model answered %d", resp.StatusCode)
	}
}

func TestAConversationContinuesAfterARestartAndGoesWhenDeleted(t *testing.T) {
	f := assistantServer(t)
	useModel(t, &assistantModel{rounds: []*agent.Response{{Content: "the answer is 42"}}})
	id := uuid.NewString()
	f.chat(id, "what is the answer?")

	f.server = assistantServerOn(t, f.stores)
	model := &assistantModel{}
	useModel(t, model)
	f.chat(id, "and again?")
	if prompt := model.lastPrompt(); !strings.Contains(prompt, "the answer is 42") {
		t.Fatalf("what was said should come back: %q", prompt)
	}

	if resp := f.send(http.MethodDelete, "/api/assistant/conversations/"+id, f.token, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete answered %d", resp.StatusCode)
	}
	if code := f.read("/api/assistant/conversations/"+id, nil); code != http.StatusNotFound {
		t.Fatalf("a deleted conversation opened with %d", code)
	}
}

func TestAFailedCallIsAnErrorRatherThanAReply(t *testing.T) {
	f := assistantServer(t)
	useModel(t, &assistantModel{failure: errors.New("the provider is down")})
	events := f.chat(uuid.NewString(), "hello")
	failed := eventOf(t, events, "error")
	if failed.Message == "" || strings.HasPrefix(failed.Message, "❌") {
		t.Fatalf("error = %+v", failed)
	}
	for _, event := range events {
		if event.Type == "reply" {
			t.Fatalf("a failure is not a reply: %v", events)
		}
	}
}

func TestTheAssistantsCallsAreCountedToIt(t *testing.T) {
	f := assistantServer(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"hel"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer provider.Close()
	// The real client refuses to reach a private address such as the fixture's.
	useModel(t, agent.NewClientByProvider("deepseek", agent.WithHTTPClient(provider.Client())))
	if err := f.stores.Models.Save(testdb.UserID, "deepseek", "DeepSeek", true, "sk-test", provider.URL, "some-model"); err != nil {
		t.Fatal(err)
	}
	before := agent.CallCallback
	defer func() { agent.CallCallback = before }()
	var mu sync.Mutex
	var feature, conversation string
	agent.CallCallback = func(report agent.CallReport) {
		mu.Lock()
		defer mu.Unlock()
		feature, conversation = report.Feature, report.ConversationID
		call := agent.Call{Feature: report.Feature, ConversationID: report.ConversationID, Provider: report.Provider, Model: report.Model, PromptTokens: report.PromptTokens, CompletionTokens: report.CompletionTokens, Status: agent.CallOK}
		if err := f.stores.Calls.Record(&call); err != nil {
			t.Error(err)
		}
	}

	id := uuid.NewString()
	events := f.chat(id, "hello")
	text := ""
	for _, event := range events {
		if event.Type == "delta" {
			var delta struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Data, &delta); err != nil {
				t.Fatal(err)
			}
			text += delta.Text
		}
	}
	if text != "hello" {
		t.Fatalf("the reply should stream, got %q from %v", text, events)
	}
	mu.Lock()
	defer mu.Unlock()
	if feature != agent.FeatureAssistant || conversation != id {
		t.Fatalf("counted to %q in %q", feature, conversation)
	}
	found := false
	for _, event := range events {
		if event.Type == "usage" {
			var use agent.CallTotals
			if err := json.Unmarshal(event.Data, &use); err != nil {
				t.Fatal(err)
			}
			found = use.PromptTokens == 10 && use.CompletionTokens == 2
		}
	}
	if !found {
		t.Fatalf("the turn should say what it came to: %v", events)
	}
}

func TestGosWordsFollowAMessageInChinese(t *testing.T) {
	for _, c := range []struct{ code, message, want string }{
		{"en", "stop it", "en"},
		{"en", "停止它", "zh"},
		{"zh-CN", "stop it", "zh"},
		{"", "", "en"},
	} {
		if got := assistantLanguage(c.code, c.message); got != c.want {
			t.Errorf("%q, %q: got %q", c.code, c.message, got)
		}
	}
}

func TestClearingTakesOnlyThisUsersConversationsWithTheAssistant(t *testing.T) {
	f := assistantServer(t)
	held := testdb.AddTrader(t, f.stores.DB, &testdb.Trader{Name: "Grid one"})
	useModel(t, &assistantModel{rounds: []*agent.Response{
		{ToolCalls: []agent.ToolCall{skillCall("trader_management", fmt.Sprintf(`{"action":"delete","target_ref":{"id":%q}}`, held.ID))}},
		{Content: "it is waiting for you"},
	}})
	waitingIn := uuid.NewString()
	waiting := eventOf(t, f.chat(waitingIn, "delete Grid one"), "reply").Confirm
	if waiting == nil {
		t.Fatal("something should be waiting")
	}
	f.chat(uuid.NewString(), "another")

	// What is someone else's, or Telegram's, stays.
	testdb.AddOtherUser(t, f.stores.DB)
	theirs := agent.Conversation{ID: uuid.NewString(), UserID: testdb.OtherUserID, Feature: agent.FeatureAssistant}
	telegram := agent.Conversation{ID: uuid.NewString(), UserID: testdb.UserID, Feature: agent.FeatureTelegram, Context: map[string]string{"chat_id": "1"}}
	for _, row := range []*agent.Conversation{&theirs, &telegram} {
		if err := f.stores.Conversations.Open(row); err != nil {
			t.Fatal(err)
		}
	}
	// The calls go with their conversation.
	call := agent.Call{Feature: agent.FeatureAssistant, ConversationID: waitingIn, Provider: "deepseek", Model: "m", Status: agent.CallOK}
	if err := f.stores.Calls.Record(&call); err != nil {
		t.Fatal(err)
	}

	if resp := f.send(http.MethodDelete, "/api/assistant/conversations", f.token, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("clearing answered %d", resp.StatusCode)
	}
	var list []assistantConversation
	f.read("/api/assistant/conversations", &list)
	if len(list) != 0 {
		t.Fatalf("list = %+v", list)
	}
	if _, err := f.stores.Conversations.Get(testdb.OtherUserID, theirs.ID); err != nil {
		t.Fatalf("another user's should stay: %v", err)
	}
	if _, err := f.stores.Conversations.Get(testdb.UserID, telegram.ID); err != nil {
		t.Fatalf("Telegram's should stay: %v", err)
	}
	if err := f.stores.DB.First(&agent.Call{}, call.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("a call outlived its conversation: %v", err)
	}

	// What was waiting went with them: answering it does nothing, even under the same ID.
	if err := f.stores.Conversations.Open(&agent.Conversation{ID: waitingIn, UserID: testdb.UserID, Feature: agent.FeatureAssistant}); err != nil {
		t.Fatal(err)
	}
	resp := f.send(http.MethodPost, "/api/assistant/conversations/"+waitingIn+"/answer", f.token, map[string]any{"confirmation_id": waiting.ID, "confirmed": true})
	var reply assistantReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	if reply.Live {
		t.Fatal("what was waiting should have gone")
	}
	rows, err := f.stores.Traders.List(testdb.UserID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("the trader should still be there: %v %v", rows, err)
	}
}

func TestClearingWaitsForTheTurnUnderWay(t *testing.T) {
	f := assistantServer(t)
	model := &assistantModel{started: make(chan struct{}), release: make(chan struct{})}
	useModel(t, model)
	started := model.started
	done := make(chan []assistantEvent, 1)
	go func() { done <- f.chat(uuid.NewString(), "first") }()
	<-started
	if resp := f.send(http.MethodDelete, "/api/assistant/conversations", f.token, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("clearing during a turn answered %d", resp.StatusCode)
	}
	close(model.release)
	<-done
}
