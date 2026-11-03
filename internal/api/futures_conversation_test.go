package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/futures"
	futuresagent "vergex/internal/futures/agent"
	"vergex/internal/futures/plan"
	"vergex/internal/futures/run"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestFuturesConversationsReadBackInFull(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	s := &Server{stores: st}

	id := uuid.NewString()
	conversation := agent.Conversation{ID: id, UserID: user, Feature: agent.FeatureFutures, Title: "plan a trade", Context: map[string]string{"account_id": "ex", "symbol": "BTCUSDT"}}
	if err := st.Conversations.Open(&conversation); err != nil {
		t.Fatal(err)
	}
	record := s.recordFuturesMessage(id)
	record(agent.RoleUser, futuresagent.Message{Role: "user", Content: "plan a trade"})
	record(agent.RoleContext, futuresagent.Message{Role: "user", Content: `{"workspace":{}}`})
	call := futuresagent.ToolCall{ID: "call_1", Type: "function"}
	call.Function.Name = "get_market"
	call.Function.Arguments = `{"depth":5}`
	record(agent.RoleAssistant, futuresagent.Message{Role: "assistant", ReasoningContent: "read the book first", ToolCalls: []futuresagent.ToolCall{call}})
	record(agent.RoleTool, futuresagent.Message{Role: "tool", ToolCallID: "call_1", Content: `{"bid":"1"}`})
	record(agent.RoleAssistant, futuresagent.Message{Role: "assistant", Content: "No trade.", ReasoningContent: "the spread is too wide"})

	// Everything is kept, thinking and tool calls included.
	messages, err := st.Conversations.Messages(id)
	if err != nil || len(messages) != 5 {
		t.Fatalf("messages = %d, err %v", len(messages), err)
	}
	if messages[2].Thinking != "read the book first" || messages[4].Thinking != "the spread is too wide" || messages[3].ToolCallID != "call_1" {
		t.Fatalf("messages = %+v", messages)
	}
	var calls []futuresagent.ToolCall
	if err := json.Unmarshal([]byte(messages[2].ToolCalls), &calls); err != nil || len(calls) != 1 || calls[0].Function.Name != "get_market" || calls[0].Function.Arguments != `{"depth":5}` {
		t.Fatalf("tool calls = %s, err %v", messages[2].ToolCalls, err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	router.GET("/conversations", s.handleFuturesConversations)
	router.GET("/conversations/:id", s.handleFuturesConversation)
	get := func(path, as string, out any) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Test-User", as)
		router.ServeHTTP(w, r)
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
		return w.Code
	}

	// The list carries no messages, and opening one shows what was said.
	var list []map[string]any
	if code := get("/conversations?account_id=ex", user, &list); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list = %d %v", code, list)
	}
	if list[0]["messages"] != nil || list[0]["symbol"] != "BTCUSDT" || list[0]["account_id"] != "ex" {
		t.Fatalf("list row = %v", list[0])
	}
	// It is only listed on the account it started on.
	var elsewhere []map[string]any
	if code := get("/conversations?account_id=other", user, &elsewhere); code != http.StatusOK || len(elsewhere) != 0 {
		t.Fatalf("another account's list = %d %v", code, elsewhere)
	}
	if code := get("/conversations", user, &elsewhere); code != http.StatusOK || len(elsewhere) != 0 {
		t.Fatalf("the list without an account = %d %v", code, elsewhere)
	}
	var opened struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if code := get("/conversations/"+id, user, &opened); code != http.StatusOK {
		t.Fatalf("open = %d", code)
	}
	if len(opened.Messages) != 2 || opened.Messages[0].Content != "plan a trade" || opened.Messages[1].Content != "No trade." {
		t.Fatalf("opened = %+v", opened.Messages)
	}

	if code := get("/conversations/"+id, uuid.NewString(), nil); code != http.StatusNotFound {
		t.Errorf("another user got %d", code)
	}
	if code := get("/conversations/not-a-uuid", user, nil); code != http.StatusNotFound {
		t.Errorf("a malformed ID got %d", code)
	}
}

func TestFuturesHistoryCanBeRenamedAndDeleted(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	s := &Server{stores: st}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	router.GET("/conversations/:id", s.handleFuturesConversation)
	router.PUT("/conversations/:id", s.handleFuturesConversationRename)
	router.DELETE("/conversations/:id", s.handleFuturesConversationDelete)
	router.DELETE("/plans/:id", s.handleFuturesPlanDelete)
	router.DELETE("/plans", s.handleFuturesPlansDelete)
	call := func(method, path, as, body string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Test-User", as)
		router.ServeHTTP(w, r)
		return w.Code
	}

	id := uuid.NewString()
	conversation := agent.Conversation{ID: id, UserID: user, Feature: agent.FeatureFutures, Title: "plan a trade"}
	if err := st.Conversations.Open(&conversation); err != nil {
		t.Fatal(err)
	}
	if err := st.Conversations.Append(id, agent.ConversationMessage{Role: agent.RoleUser, Content: "plan a trade"}); err != nil {
		t.Fatal(err)
	}
	before, err := st.Conversations.Get(user, id)
	if err != nil {
		t.Fatal(err)
	}

	// A new name is kept with its spaces tidied, and does not move the conversation.
	if code := call(http.MethodPut, "/conversations/"+id, user, `{"title":"  BTC   pullback "}`); code != http.StatusOK {
		t.Fatalf("rename = %d", code)
	}
	after, err := st.Conversations.Get(user, id)
	if err != nil || after.Title != "BTC pullback" || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("renamed = %+v, before %+v, err %v", after, before, err)
	}
	// Carrying on with it does not take the name back.
	again := agent.Conversation{ID: id, UserID: user, Feature: agent.FeatureFutures, Title: "another message"}
	if err := st.Conversations.Open(&again); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Conversations.Get(user, id); got.Title != "BTC pullback" {
		t.Fatalf("title after carrying on = %q", got.Title)
	}
	if code := call(http.MethodPut, "/conversations/"+id, user, `{"title":"   "}`); code != http.StatusBadRequest {
		t.Errorf("an empty name got %d", code)
	}

	// Another user's conversation is not there for them, whatever they try.
	other := uuid.NewString()
	if code := call(http.MethodPut, "/conversations/"+id, other, `{"title":"x"}`); code != http.StatusNotFound {
		t.Errorf("another user renaming got %d", code)
	}
	if code := call(http.MethodDelete, "/conversations/"+id, other, ""); code != http.StatusNotFound {
		t.Errorf("another user deleting got %d", code)
	}

	// Plans: two drafted in the conversation that never ran, one drafted in it that ran, and
	// one drafted by hand.
	account := testdb.AddAccount(t, st.DB, user, "binance", "main")
	plans := []futures.Plan{
		{UserID: user, ID: "draft", AccountID: account, Symbol: "BTCUSDT", Status: "cancelled", ConversationID: id, Body: "{}"},
		{UserID: user, ID: "revised", AccountID: account, Symbol: "BTCUSDT", Status: "draft", ConversationID: id, Body: "{}"},
		{UserID: user, ID: "ran", AccountID: account, Symbol: "BTCUSDT", Status: "completed", ConversationID: id, Body: "{}"},
		{UserID: user, ID: "manual", AccountID: account, Symbol: "BTCUSDT", Status: "draft", Body: "{}"},
		{UserID: user, ID: "working", AccountID: account, Symbol: "BTCUSDT", Status: "approved", Body: "{}"},
	}
	for i := range plans {
		if err := st.DB.Create(&plans[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	ranRun := uuid.NewString()
	for planID, state := range map[string]string{"ran": "completed", "working": "running"} {
		runID := uuid.NewString()
		if planID == "ran" {
			runID = ranRun
		}
		if err := st.DB.Exec(`INSERT INTO futures.runs (id, user_id, plan_id, account_id, account_identity, plan_hash, approval_hash, status, approved_at, updated_at)
			VALUES (?, ?, ?, ?, 'identity', 'plan', 'approval', ?, now(), now())`, runID, user, planID, account, state).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DB.Exec(`INSERT INTO futures.run_orders (run_id, seq, role, symbol, side, client_id, status) VALUES (?, 1, 'entry', 'BTCUSDT', 'long', 'vergex-1', 'FILLED')`, ranRun).Error; err != nil {
		t.Fatal(err)
	}

	// Deleting the conversation takes its messages and the plans it drafted that never ran.
	if code := call(http.MethodDelete, "/conversations/"+id, user, ""); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
	if code := call(http.MethodGet, "/conversations/"+id, user, ""); code != http.StatusNotFound {
		t.Errorf("deleted conversation got %d", code)
	}
	if messages, err := st.Conversations.Messages(id); err != nil || len(messages) != 0 {
		t.Errorf("messages left = %d, err %v", len(messages), err)
	}
	store := futures.NewPlanStore(st.DB, user, "", "")
	for _, gone := range []string{"draft", "revised"} {
		if _, err := store.Find(gone); err == nil {
			t.Errorf("plan %s outlived its conversation", gone)
		}
	}
	ran, err := store.Find("ran")
	if err != nil || ran.ConversationID != "" {
		t.Fatalf("the plan that ran = %+v, err %v", ran, err)
	}
	if _, err := store.Find("manual"); err != nil {
		t.Fatalf("a plan drafted by hand went with the conversation: %v", err)
	}

	// A plan still at work stays. One whose run has settled goes with the run and its orders,
	// and one that never ran can go too.
	if code := call(http.MethodDelete, "/plans/working", user, ""); code != http.StatusConflict {
		t.Errorf("deleting a plan at work got %d", code)
	}
	if code := call(http.MethodDelete, "/plans/ran", other, ""); code != http.StatusNotFound {
		t.Errorf("another user deleting a plan got %d", code)
	}
	if code := call(http.MethodDelete, "/plans/ran", user, ""); code != http.StatusOK {
		t.Fatalf("deleting a finished plan got %d", code)
	}
	var left int64
	if err := st.DB.Raw(`SELECT (SELECT count(*) FROM futures.runs WHERE id = ?) + (SELECT count(*) FROM futures.run_orders WHERE run_id = ?)`, ranRun, ranRun).Scan(&left).Error; err != nil || left != 0 {
		t.Fatalf("the finished plan left %d runs and orders, err %v", left, err)
	}

	// Several at once: what can go goes, and what is at work is named.
	var out struct {
		Deleted int      `json:"deleted"`
		AtWork  []string `json:"at_work"`
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/plans", strings.NewReader(`{"ids":["manual","working","gone"]}`))
	r.Header.Set("Test-User", user)
	router.ServeHTTP(w, r)
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK || out.Deleted != 1 || fmt.Sprint(out.AtWork) != "[working]" {
		t.Fatalf("batch = %d %s", w.Code, w.Body)
	}
	if _, err := store.Find("manual"); err == nil {
		t.Error("the draft is still there")
	}
	if _, err := store.Find("working"); err != nil {
		t.Errorf("the plan at work went: %v", err)
	}
}

func TestFuturesRefusalsNameTheirWording(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/plain", func(c *gin.Context) { futuresError(c, errors.New("plain")) })
	router.GET("/named", func(c *gin.Context) {
		futuresError(c, fmt.Errorf("compile: %w", plan.Rejection{Key: "rewardRisk", Params: map[string]string{"ratio": "0.5"}, Text: "too low"}))
	})
	read := func(path string) map[string]any {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", path, w.Code, w.Body)
		}
		return body
	}
	if got := read("/plain"); got["error"] != "plain" || got["error_key"] != nil {
		t.Errorf("plain = %v", got)
	}
	got := read("/named")
	if got["error_key"] != "futures.rejection.rewardRisk" || got["error"] != "compile: too low" {
		t.Errorf("named = %v", got)
	}
	if params, _ := got["error_params"].(map[string]any); params["ratio"] != "0.5" {
		t.Errorf("params = %v", got["error_params"])
	}
}

func TestOnlyAnOrderNoLivePlanHoldsCanBeCancelledByHand(t *testing.T) {
	open := []account.OpenOrder{{OrderID: "free", Conditional: true}, {OrderID: "held"}, {OrderID: "old"}}
	runs := []run.View{
		{Run: run.Run{AccountID: "a", Status: "position_open"}, Orders: []run.Order{{OrderID: "held"}}},
		{Run: run.Run{AccountID: "a", Status: "completed"}, Orders: []run.Order{{OrderID: "old"}}},
		{Run: run.Run{AccountID: "b", Status: "position_open"}, Orders: []run.Order{{OrderID: "free"}}},
	}
	order, err := cancellableOrder(open, runs, "a", "free")
	if err != nil || !order.Conditional {
		t.Fatalf("free = %+v, err %v", order, err)
	}
	if _, err := cancellableOrder(open, runs, "a", "old"); err != nil {
		t.Fatalf("a finished plan's leftover: %v", err)
	}
	var rejection plan.Rejection
	if _, err := cancellableOrder(open, runs, "a", "held"); !errors.As(err, &rejection) || rejection.Key != "orderInPlan" {
		t.Fatalf("held = %v", err)
	}
	// OKX lists an algo order's stop and target with a mark on the ID.
	if _, err := cancellableOrder(append(open, account.OpenOrder{OrderID: "held_sl"}), runs, "a", "held_sl"); !errors.As(err, &rejection) || rejection.Key != "orderInPlan" {
		t.Fatalf("held_sl = %v", err)
	}
	if _, err := cancellableOrder(open, runs, "a", "gone"); !errors.As(err, &rejection) || rejection.Key != "orderGone" {
		t.Fatalf("gone = %v", err)
	}
}

func TestDeletingAnAccountTakesItsFuturesHistory(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	s := &Server{stores: st, accountStateCache: NewAccountStateCache()}
	gone, err := st.Accounts.Create(user, "binance", "Gone", "gone-key", "secret", "", false)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := st.Accounts.Create(user, "binance", "Kept", "kept-key", "secret", "", false)
	if err != nil {
		t.Fatal(err)
	}
	for id, accountID := range map[string]string{uuid.NewString(): gone, uuid.NewString(): kept} {
		row := agent.Conversation{ID: id, UserID: user, Feature: agent.FeatureFutures, Context: map[string]string{futuresConversationAccount: accountID}}
		if err := st.Conversations.Open(&row); err != nil {
			t.Fatal(err)
		}
		if err := st.Conversations.Append(id, agent.ConversationMessage{Role: agent.RoleUser, Content: "plan a trade"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []futures.Plan{
		{UserID: user, ID: "gone-draft", AccountID: gone, Symbol: "BTCUSDT", Status: "draft", Body: "{}"},
		{UserID: user, ID: "gone-revision", AccountID: gone, Symbol: "BTCUSDT", Status: "draft", Body: "{}"},
		{UserID: user, ID: "kept-draft", AccountID: kept, Symbol: "BTCUSDT", Status: "draft", Body: "{}"},
	} {
		if err := st.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", user) })
	router.DELETE("/accounts/:id", s.handleDeleteAccount)
	remove := func(id string) int {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/accounts/"+id, nil))
		return w.Code
	}

	// Refused, it takes nothing with it.
	if err := st.DB.Create(&account.Lease{AccountID: gone, Source: account.SourceFutures, SourceID: uuid.NewString()}).Error; err != nil {
		t.Fatal(err)
	}
	if code := remove(gone); code != http.StatusBadRequest {
		t.Fatalf("deleting a leased account = %d", code)
	}
	count := func(accountID string) (conversations, plans int) {
		t.Helper()
		rows, err := st.Conversations.ListByContext(user, agent.FeatureFutures, futuresConversationAccount, accountID, 50)
		if err != nil {
			t.Fatal(err)
		}
		ids, err := futures.NewPlanStore(st.DB, user, accountID, "").OnAccount()
		if err != nil {
			t.Fatal(err)
		}
		return len(rows), len(ids)
	}
	if conversations, plans := count(gone); conversations != 1 || plans != 2 {
		t.Fatalf("a refused delete left %d conversations and %d plans", conversations, plans)
	}

	if err := st.DB.Where("account_id = ?", gone).Delete(&account.Lease{}).Error; err != nil {
		t.Fatal(err)
	}
	if code := remove(gone); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
	if conversations, plans := count(gone); conversations != 0 || plans != 0 {
		t.Errorf("left behind %d conversations and %d plans", conversations, plans)
	}
	var strays int64
	if err := st.DB.Model(&futures.Plan{}).Where("id IN ?", []string{"gone-draft", "gone-revision"}).Count(&strays).Error; err != nil || strays != 0 {
		t.Errorf("plans without an account left behind: %d, err %v", strays, err)
	}
	if conversations, plans := count(kept); conversations != 1 || plans != 1 {
		t.Errorf("the other account has %d conversations and %d plans", conversations, plans)
	}
}
