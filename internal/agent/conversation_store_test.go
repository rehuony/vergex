package agent

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"vergex/internal/database/testdb"
	"vergex/internal/user"

	"github.com/google/uuid"
)

const conversationUser = "00000000-0000-4000-8000-000000000001"

func newTestConversation(t *testing.T) (*ConversationStore, string) {
	t.Helper()
	db := testdb.Open(t)
	if err := user.NewStore(db).Create(&user.User{ID: conversationUser, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	s := NewConversationStore(db)
	id := uuid.NewString()
	err := s.Open(&Conversation{ID: id, UserID: conversationUser, Feature: FeatureFutures, Context: map[string]string{"symbol": "BTCUSDT"}})
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestConversationKeepsEverythingSaidInOrder(t *testing.T) {
	s, id := newTestConversation(t)
	calls := `[{"id":"call_1","type":"function","function":{"name":"get_market","arguments":"{}"}}]`
	said := []ConversationMessage{
		{Role: RoleUser, Content: "plan a trade"},
		{Role: RoleContext, Content: `{"workspace":{}}`},
		{Role: RoleAssistant, Thinking: "read the book first", ToolCalls: calls},
		{Role: RoleTool, Content: `{"bid":"1"}`, ToolCallID: "call_1"},
		{Role: RoleAssistant, Content: "no trade", Thinking: "spread too wide"},
	}
	for _, message := range said {
		if err := s.Append(id, message); err != nil {
			t.Fatal(err)
		}
	}

	messages, err := s.Messages(id)
	if err != nil || len(messages) != len(said) {
		t.Fatalf("messages = %d, err %v", len(messages), err)
	}
	for i, message := range messages {
		if message.Seq != i+1 || message.Role != said[i].Role || message.Content != said[i].Content || message.Thinking != said[i].Thinking || message.ToolCallID != said[i].ToolCallID {
			t.Errorf("message %d = %+v, want %+v", i, message, said[i])
		}
	}
	var read []struct {
		ID       string `json:"id"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(messages[2].ToolCalls), &read); err != nil || len(read) != 1 || read[0].ID != "call_1" || read[0].Function.Name != "get_market" {
		t.Fatalf("tool calls = %s, err %v", messages[2].ToolCalls, err)
	}

	transcript, err := s.Transcript(id, 0)
	if err != nil || len(transcript) != 2 || transcript[0].Content != "plan a trade" || transcript[1].Content != "no trade" {
		t.Fatalf("transcript = %+v, err %v", transcript, err)
	}
	latest, err := s.Transcript(id, 1)
	if err != nil || len(latest) != 1 || latest[0].Content != "no trade" {
		t.Fatalf("latest = %+v, err %v", latest, err)
	}
}

func TestConversationNumbersMessagesWhenWritersMeet(t *testing.T) {
	s, id := newTestConversation(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Append(id, ConversationMessage{Role: RoleUser, Content: "a"}, ConversationMessage{Role: RoleAssistant, Content: "b"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	messages, err := s.Messages(id)
	if err != nil || len(messages) != 16 {
		t.Fatalf("messages = %d, err %v", len(messages), err)
	}
	for i, message := range messages {
		if message.Seq != i+1 {
			t.Fatalf("message %d has seq %d", i, message.Seq)
		}
	}
}

func TestConversationCannotBeTakenOver(t *testing.T) {
	s, id := newTestConversation(t)
	if err := s.Open(&Conversation{ID: id, UserID: conversationUser, Feature: FeatureFutures}); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if err := s.Open(&Conversation{ID: id, UserID: conversationUser, Feature: FeatureTelegram}); err == nil {
		t.Fatal("opened as another feature")
	}
	if err := s.Open(&Conversation{ID: "not-a-uuid", UserID: conversationUser, Feature: FeatureFutures}); err == nil {
		t.Fatal("accepted a malformed ID")
	}
	if err := s.Append(uuid.NewString(), ConversationMessage{Role: RoleUser, Content: "x"}); err == nil {
		t.Fatal("appended to a missing conversation")
	}
}

func TestConversationDropsWhatPostgreSQLRefuses(t *testing.T) {
	s, id := newTestConversation(t)
	if err := s.Append(id, ConversationMessage{Role: RoleAssistant, Content: "a\x00b\xff", Thinking: "c\x00"}); err != nil {
		t.Fatal(err)
	}
	messages, err := s.Messages(id)
	if err != nil || messages[0].Content != "ab\uFFFD" || messages[0].Thinking != "c" {
		t.Fatalf("messages = %+v, err %v", messages, err)
	}
}

func TestOldMessagesCanBeReportedAndDeleted(t *testing.T) {
	db := testdb.Open(t)
	user := testdb.AddUser(t, db)
	store := NewConversationStore(db)
	id := uuid.NewString()
	conversation := Conversation{ID: id, UserID: user, Feature: FeatureFutures, Title: "old and new"}
	if err := store.Open(&conversation); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"old one", "old two", "old three", "new"} {
		if err := store.Append(id, ConversationMessage{Role: RoleUser, Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	old := now.Add(-100 * 24 * time.Hour)
	if err := db.Exec("UPDATE agent.messages SET created_at = ? WHERE content LIKE 'old%'", old).Error; err != nil {
		t.Fatal(err)
	}
	cutoff := now.Add(-90 * 24 * time.Hour)

	count, size, err := store.MessagesBefore(cutoff)
	if err != nil || count != 3 || size != int64(len("old oneold twoold three")) {
		t.Fatalf("report = %d messages, %d bytes, err %v", count, size, err)
	}
	if left, _ := store.Messages(id); len(left) != 4 {
		t.Fatalf("reporting deleted something: %d left", len(left))
	}

	// What was reported is what goes, whatever the batch size, and nothing newer does.
	deleted, err := store.DeleteMessagesBefore(cutoff, 2)
	if err != nil || deleted != count {
		t.Fatalf("deleted = %d of %d, err %v", deleted, count, err)
	}
	left, err := store.Messages(id)
	if err != nil || len(left) != 1 || left[0].Content != "new" {
		t.Fatalf("left = %+v, err %v", left, err)
	}
	if _, err := store.Get(user, id); err != nil {
		t.Fatalf("the conversation went with them: %v", err)
	}
	if err := store.Append(id, ConversationMessage{Role: RoleUser, Content: "after"}); err != nil {
		t.Fatalf("carrying on after a prune: %v", err)
	}
	if size, err := store.MessageStorage(); err != nil || size <= 0 {
		t.Fatalf("storage = %d, err %v", size, err)
	}
}
