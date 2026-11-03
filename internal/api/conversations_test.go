package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/agent"
	"vergex/internal/database/testdb"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestDeletingConversationsTakesAllTheyHeldInAnyChat(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	stranger := testdb.AddOtherUser(t, st.DB)
	s := &Server{stores: st}

	open := func(owner, feature string, context map[string]string) string {
		t.Helper()
		id := uuid.NewString()
		row := agent.Conversation{ID: id, UserID: owner, Feature: feature, Context: context}
		if err := st.Conversations.Open(&row); err != nil {
			t.Fatal(err)
		}
		if err := st.Conversations.Append(id, agent.ConversationMessage{Role: agent.RoleUser, Content: "hello"}); err != nil {
			t.Fatal(err)
		}
		if err := st.Calls.Record(&agent.Call{Feature: feature, ConversationID: id, Provider: "deepseek", Model: "deepseek-chat"}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	futuresChat := open(user, agent.FeatureFutures, map[string]string{futuresConversationAccount: "main", "symbol": "BTCUSDT"})
	assistantChat := open(user, agent.FeatureAssistant, nil)
	telegramChat := open(user, agent.FeatureTelegram, nil)
	traderCycle := open(user, agent.FeatureTrader, nil)
	theirs := open(stranger, agent.FeatureFutures, nil)
	if err := st.DB.Exec(`INSERT INTO futures.calculators (id, conversation_id, body) VALUES ('h', ?, '{}')`, futuresChat).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", user) })
	s.registerConversations(&router.RouterGroup)
	call := func(method, path, body string, out any) int {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
		return w.Code
	}

	// Every chat is listed, whatever its feature, and a trader's cycles are not chats.
	var listed []chatConversation
	if code := call(http.MethodGet, "/conversations", "", &listed); code != http.StatusOK || len(listed) != 3 {
		t.Fatalf("list = %d %+v", code, listed)
	}
	if code := call(http.MethodGet, "/conversations?account_id=main", "", &listed); code != http.StatusOK || len(listed) != 1 || listed[0].Symbol != "BTCUSDT" {
		t.Fatalf("one account's list = %d %+v", code, listed)
	}
	if code := call(http.MethodGet, "/conversations?feature=trader", "", nil); code != http.StatusBadRequest {
		t.Errorf("listing a trader's cycles got %d", code)
	}

	// Only the user's own chats go, and with them what was said, the model calls and the
	// entry calculators.
	ids, _ := json.Marshal(map[string]any{"ids": []string{futuresChat, assistantChat, telegramChat, traderCycle, theirs, "not-an-id"}})
	var out struct {
		Deleted int `json:"deleted"`
	}
	if code := call(http.MethodDelete, "/conversations", string(ids), &out); code != http.StatusOK || out.Deleted != 3 {
		t.Fatalf("delete = %d, deleted %d", code, out.Deleted)
	}
	count := func(table string, id string) int64 {
		t.Helper()
		var n int64
		if err := st.DB.Table(table).Where("conversation_id = ?", id).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, id := range []string{futuresChat, assistantChat, telegramChat} {
		if messages, calls := count("agent.messages", id), count("agent.calls", id); messages+calls != 0 {
			t.Errorf("%s left %d messages and %d calls", id, messages, calls)
		}
	}
	if n := count("futures.calculators", futuresChat); n != 0 {
		t.Errorf("%d calculators left", n)
	}
	for _, id := range []string{traderCycle, theirs} {
		if messages, calls := count("agent.messages", id), count("agent.calls", id); messages != 1 || calls != 1 {
			t.Errorf("%s kept %d messages and %d calls", id, messages, calls)
		}
	}
	if code := call(http.MethodDelete, "/conversations", `{"ids":[]}`, nil); code != http.StatusBadRequest {
		t.Errorf("deleting nothing got %d", code)
	}
}
