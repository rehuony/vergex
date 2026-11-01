package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vergex/internal/planning/agent/model"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestFrontierConversationsReadBackInFull(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	user := storetest.Account(t, st)
	s := &Server{store: st}

	id := uuid.NewString()
	conversation := store.AgentConversation{ID: id, UserID: user, Feature: store.FeatureFrontier, Title: "plan a trade", Context: map[string]string{"exchange_id": "ex", "symbol": "BTCUSDT"}}
	if err := st.AgentConversation().Open(&conversation); err != nil {
		t.Fatal(err)
	}
	record := s.recordPlanningMessage(id)
	record(store.RoleUser, model.Message{Role: "user", Content: "plan a trade"})
	record(store.RoleContext, model.Message{Role: "user", Content: `{"workspace":{}}`})
	call := model.ToolCall{ID: "call_1", Type: "function"}
	call.Function.Name = "get_market"
	call.Function.Arguments = `{"depth":5}`
	record(store.RoleAssistant, model.Message{Role: "assistant", ReasoningContent: "read the book first", ToolCalls: []model.ToolCall{call}})
	record(store.RoleTool, model.Message{Role: "tool", ToolCallID: "call_1", Content: `{"bid":"1"}`})
	record(store.RoleAssistant, model.Message{Role: "assistant", Content: "No trade.", ReasoningContent: "the spread is too wide"})

	// Everything is kept, thinking and tool calls included.
	messages, err := st.AgentConversation().Messages(id)
	if err != nil || len(messages) != 5 {
		t.Fatalf("messages = %d, err %v", len(messages), err)
	}
	if messages[2].Thinking != "read the book first" || messages[4].Thinking != "the spread is too wide" || messages[3].ToolCallID != "call_1" {
		t.Fatalf("messages = %+v", messages)
	}
	var calls []model.ToolCall
	if err := json.Unmarshal([]byte(messages[2].ToolCalls), &calls); err != nil || len(calls) != 1 || calls[0].Function.Name != "get_market" || calls[0].Function.Arguments != `{"depth":5}` {
		t.Fatalf("tool calls = %s, err %v", messages[2].ToolCalls, err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	router.GET("/conversations", s.handlePlanningConversations)
	router.GET("/conversations/:id", s.handlePlanningConversation)
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
	if code := get("/conversations", user, &list); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list = %d %v", code, list)
	}
	if list[0]["messages"] != nil || list[0]["symbol"] != "BTCUSDT" || list[0]["exchange_id"] != "ex" {
		t.Fatalf("list row = %v", list[0])
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
