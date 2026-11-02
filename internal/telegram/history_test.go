package telegram

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	telegramagent "vergex/internal/telegram/agent"
)

// scriptedClient asks for one skill, thinking aloud, then answers. It remembers the
// prompt it last saw and the conversation it was told about.
type scriptedClient struct {
	turn    atomic.Int32
	lastReq *agent.Request
}

func (c *scriptedClient) SetAPIKey(string, string, string)                {}
func (c *scriptedClient) SetTimeout(time.Duration)                        {}
func (c *scriptedClient) CallWithMessages(string, string) (string, error) { return "", nil }
func (c *scriptedClient) CallWithRequest(*agent.Request) (string, error)  { return "", nil }
func (c *scriptedClient) CallWithRequestFull(req *agent.Request) (*agent.Response, error) {
	c.lastReq = req
	if c.turn.Add(1) == 1 {
		call := agent.ToolCall{ID: "call_1", Type: "function"}
		call.Function.Name = "account_management"
		call.Function.Arguments = `{"action":"query"}`
		return &agent.Response{Content: "let me look", ReasoningContent: "check the accounts", ToolCalls: []agent.ToolCall{call}}, nil
	}
	return &agent.Response{Content: "you have no accounts", ReasoningContent: "the list is empty"}, nil
}

// signed stands in for what signs the bot's API calls; these tests make none.
func signed() (string, error) { return "tok", nil }

// noLanguage stands for a chat where none was chosen with /lang.
func noLanguage() string { return "" }

func TestAChatIsKeptAcrossARestart(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	history := &chatHistory{conversations: st.Conversations, userID: user}
	const chat = int64(42)

	client := &scriptedClient{}
	manager := telegramagent.NewManager(0, signed, "me@example.com", user, func() agent.Client { return client }, history, noLanguage)
	if reply := manager.Run(context.Background(), chat, "what do I have?", nil); reply.Text != "you have no accounts" {
		t.Fatalf("reply = %q", reply.Text)
	}

	conversation, err := st.Conversations.Latest(user, agent.FeatureTelegram, "42")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := st.Conversations.Messages(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	if got := strings.Join(roles, ","); got != "user,context,assistant,tool,assistant" {
		t.Fatalf("roles = %s", got)
	}
	if messages[0].Content != "what do I have?" {
		t.Fatalf("user message = %q", messages[0].Content)
	}
	asked, result, answer := messages[2], messages[3], messages[4]
	if asked.Thinking != "check the accounts" || asked.Content != "let me look" || !strings.Contains(asked.ToolCalls, "account_management") {
		t.Fatalf("tool request = %+v", asked)
	}
	if result.ToolCallID != "call_1" {
		t.Fatalf("tool result = %+v", result)
	}
	if answer.Content != "you have no accounts" || answer.Thinking != "the list is empty" {
		t.Fatalf("answer = %+v", answer)
	}

	// A new manager is what a restart makes: it remembers, in the same conversation.
	client = &scriptedClient{}
	client.turn.Store(1)
	restarted := telegramagent.NewManager(0, signed, "me@example.com", user, func() agent.Client { return client }, history, noLanguage)
	restarted.Run(context.Background(), chat, "and now?", nil)
	sent := client.lastReq.Messages[len(client.lastReq.Messages)-1].Content
	if !strings.Contains(sent, "what do I have?") || !strings.Contains(sent, "you have no accounts") {
		t.Fatalf("the model was not reminded: %q", sent)
	}
	if strings.Contains(sent, "let me look") {
		t.Fatalf("a step on the way to an answer came back as if it were one: %q", sent)
	}
	if again, err := st.Conversations.Latest(user, agent.FeatureTelegram, "42"); err != nil || again.ID != conversation.ID {
		t.Fatalf("resumed %+v, err %v", again, err)
	}

	// /start begins a new conversation, even as the first thing after a restart.
	client = &scriptedClient{}
	client.turn.Store(1)
	afterStart := telegramagent.NewManager(0, signed, "me@example.com", user, func() agent.Client { return client }, history, noLanguage)
	afterStart.Reset(chat)
	afterStart.Run(context.Background(), chat, "fresh", nil)
	sent = client.lastReq.Messages[len(client.lastReq.Messages)-1].Content
	if strings.Contains(sent, "what do I have?") {
		t.Fatalf("/start kept the old chat: %q", sent)
	}
	latest, err := st.Conversations.Latest(user, agent.FeatureTelegram, "42")
	if err != nil || latest.ID == conversation.ID {
		t.Fatalf("latest = %+v, err %v", latest, err)
	}
}
