package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"vergex/internal/agent"
	"vergex/internal/assistant"
	"vergex/internal/logger"
	"vergex/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	// assistantTurnLimit bounds one turn, across all its model and skill calls.
	assistantTurnLimit = 3 * time.Minute
	// assistantShown is how much of a conversation opening it shows.
	assistantShown = 40
	// assistantListed is how many conversations the list holds.
	assistantListed = 100
	// assistantMessageLimit bounds what is typed, in characters.
	assistantMessageLimit = 8000
	// assistantBodyLimit bounds a request, with the page it describes.
	assistantBodyLimit = 64 << 10
)

// assistantService is the assistant in the web app, for one user.
type assistantService struct {
	manager *assistant.Manager
	// busy refuses a second turn while one is under way, in another tab say.
	busy atomic.Bool
	// chats is held by whatever uses the manager's agents, none of which can be used by two
	// at once. Tidying up after a reply holds it too, without counting as busy.
	chats sync.Mutex
	// What the turn under way asked for, read while chats is held.
	client   agent.Client
	language string
}

// assistantHistory keeps a user's conversations with the assistant, each its own chat.
type assistantHistory struct {
	conversations *agent.ConversationStore
}

func (h *assistantHistory) Resume(id string, limit int) (string, []assistant.Message) {
	rows, err := h.conversations.Transcript(id, limit)
	if err != nil {
		logger.Warnf("Assistant: read messages: %v", err)
		return id, nil
	}
	messages := make([]assistant.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, assistant.Message{Role: row.Role, Content: row.Content})
	}
	return id, messages
}

// Start is never needed: a new conversation here is a new chat, with an ID of its own.
func (h *assistantHistory) Start(id string) string { return id }

func (h *assistantHistory) Append(conversationID, role string, message agent.Message) {
	assistant.Record(h.conversations, conversationID, role, message)
}

// Exists finds a conversation the turn has opened anyway: one deleted here was forgotten as
// it went.
func (h *assistantHistory) Exists(conversationID string) bool {
	return assistant.HistoryExists(h.conversations, conversationID)
}

func (s *Server) assistantFor(userID, email string) *assistantService {
	if known, ok := s.assistants.Load(userID); ok {
		return known.(*assistantService)
	}
	chat := &assistantService{}
	chat.manager = assistant.NewManager(s.port,
		func() (string, error) { return assistant.CallToken(userID) },
		email, userID,
		func() agent.Client { return chat.client },
		&assistantHistory{conversations: s.stores.Conversations},
		func() string { return chat.language },
		assistant.Web,
	)
	known, _ := s.assistants.LoadOrStore(userID, chat)
	return known.(*assistantService)
}

func (s *Server) registerAssistant(protected *gin.RouterGroup) {
	protected.POST("/assistant/chat", s.handleAssistantChat)
	protected.GET("/assistant/conversations", s.handleAssistantConversations)
	protected.DELETE("/assistant/conversations", s.handleAssistantConversationsClear)
	protected.GET("/assistant/conversations/:id", s.handleAssistantConversation)
	protected.POST("/assistant/conversations/:id/answer", s.handleAssistantAnswer)
	protected.DELETE("/assistant/conversations/:id", s.handleAssistantConversationDelete)
}

// assistantLanguage is what the assistant writes in, for the words that are Go's: the
// language of a message in Chinese, else the interface's.
func assistantLanguage(code, message string) string {
	if strings.HasPrefix(strings.ToLower(code), "zh") || assistant.PrefersChinese(message) {
		return "zh"
	}
	return "en"
}

var errAssistantBusy = gin.H{"error": "The assistant is still answering. Wait for it or stop it first"}

// handleAssistantChat runs one turn, streaming what is under way and the reply as it is
// written. Ending the request stops the turn.
func (s *Server) handleAssistantChat(c *gin.Context) {
	user := c.GetString("user_id")
	var in struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
		AgentModelID   string `json:"agent_model_id"`
		AgentModelName string `json:"agent_model_name"`
		Language       string `json:"language"`
		// Page is what the user has open, as the page itself declares it.
		Page json.RawMessage `json:"page"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, assistantBodyLimit)
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Message) == "" || utf8.RuneCountInString(in.Message) > assistantMessageLimit {
		SafeBadRequest(c, "The message is empty or too long")
		return
	}
	model, client, err := s.modelClient(user, in.AgentModelID, in.AgentModelName, agent.FeatureAssistant)
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	chat := s.assistantFor(user, c.GetString("email"))
	if !chat.busy.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, errAssistantBusy)
		return
	}
	defer chat.busy.Store(false)
	conversation := agent.Conversation{
		ID:           in.ConversationID,
		UserID:       user,
		Feature:      agent.FeatureAssistant,
		AgentModelID: model.ID,
		Title:        conversationTitle(in.Message),
	}
	if err := s.stores.Conversations.Open(&conversation); err != nil {
		SafeBadRequest(c, "This conversation cannot be continued")
		return
	}

	chat.chats.Lock()
	defer chat.chats.Unlock()
	chat.client, chat.language = client, assistantLanguage(in.Language, in.Message)
	ctx, cancel := context.WithTimeout(assistant.WithPage(c.Request.Context(), string(in.Page)), assistantTurnLimit)
	defer cancel()

	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	emit := func(kind string, data any) {
		raw, err := json.Marshal(gin.H{"type": kind, "data": data})
		if err != nil {
			return
		}
		if _, err = fmt.Fprintf(c.Writer, "data: %s\n\n", raw); err == nil {
			c.Writer.Flush()
		}
	}
	started := time.Now()
	reply := chat.manager.Run(ctx, conversation.ID, in.Message, &assistant.Progress{
		Stage: func(stage string) { emit("stage", gin.H{"text": stage}) },
		Text:  func(text string) { emit("delta", gin.H{"text": text}) },
	})
	// Compacting calls the model, so it waits until the reply is out. It holds chats, and
	// the next turn waits for it rather than being refused.
	service.Go("assistant-tidy", func() {
		chat.chats.Lock()
		defer chat.chats.Unlock()
		chat.manager.Tidy(conversation.ID)
	})
	if c.Request.Context().Err() != nil {
		return
	}
	// What this turn came to. Every call is on record by the time it returns.
	if use, err := s.stores.Calls.ForConversation(conversation.ID, started); err != nil {
		logger.Warnf("Assistant: read a turn's model calls: %v", err)
	} else if use.Calls > 0 {
		emit("usage", use)
	}
	if reply.Failed {
		emit("error", gin.H{"message": assistant.WithoutMarks(reply.Text), "confirm": reply.Confirm, "changed": reply.Changed})
	} else {
		text, results := assistant.SplitResults(reply.Text)
		emit("reply", gin.H{"text": assistant.WithoutMarks(text), "results": results, "confirm": reply.Confirm, "changed": reply.Changed, "links": reply.Links, "proposals": reply.Proposals})
	}
	emit("done", gin.H{"ok": !reply.Failed})
}

// assistantConversation is a conversation as the list and its head show it.
type assistantConversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newAssistantConversation(row agent.Conversation) assistantConversation {
	return assistantConversation{ID: row.ID, Title: row.Title, UpdatedAt: row.UpdatedAt}
}

// handleAssistantConversations lists the latest conversations, without their messages.
func (s *Server) handleAssistantConversations(c *gin.Context) {
	rows, err := s.stores.Conversations.List(c.GetString("user_id"), agent.FeatureAssistant, assistantListed)
	if err != nil {
		SafeInternalError(c, "Read the assistant's conversations", err)
		return
	}
	out := make([]assistantConversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, newAssistantConversation(row))
	}
	c.JSON(http.StatusOK, out)
}

// assistantConversationOf finds the conversation a request names, or answers that there is
// none and returns nil.
func (s *Server) assistantConversationOf(c *gin.Context) *agent.Conversation {
	row, err := s.stores.Conversations.Get(c.GetString("user_id"), c.Param("id"))
	if err != nil || row.Feature != agent.FeatureAssistant {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return nil
	}
	return row
}

// handleAssistantConversation opens a conversation: what was said lately, and what is
// waiting for an answer.
func (s *Server) handleAssistantConversation(c *gin.Context) {
	row := s.assistantConversationOf(c)
	if row == nil {
		return
	}
	messages, err := s.stores.Conversations.Transcript(row.ID, assistantShown)
	if err != nil {
		SafeInternalError(c, "Read the conversation", err)
		return
	}
	// While a turn is under way what it holds is not settled, so this says nothing of it.
	var confirm *assistant.Confirmation
	chat := s.assistantFor(c.GetString("user_id"), c.GetString("email"))
	if chat.chats.TryLock() {
		confirm = chat.manager.Pending(row.ID)
		chat.chats.Unlock()
	}
	c.JSON(http.StatusOK, gin.H{"conversation": newAssistantConversation(*row), "messages": assistant.ForReading(messages), "confirm": confirm})
}

// handleAssistantAnswer resolves what a conversation is waiting for an answer to. live is
// false when that is no longer what waits: answered already, replaced or lost in a restart.
func (s *Server) handleAssistantAnswer(c *gin.Context) {
	row := s.assistantConversationOf(c)
	if row == nil {
		return
	}
	var in struct {
		ConfirmationID string `json:"confirmation_id"`
		Confirmed      bool   `json:"confirmed"`
		Language       string `json:"language"`
	}
	if c.ShouldBindJSON(&in) != nil {
		SafeBadRequest(c, "Invalid answer")
		return
	}
	chat := s.assistantFor(c.GetString("user_id"), c.GetString("email"))
	if !chat.busy.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, errAssistantBusy)
		return
	}
	defer chat.busy.Store(false)
	chat.chats.Lock()
	defer chat.chats.Unlock()
	// A button says nothing of what it answers, so this keeps what the question was in,
	// unless the interface is in Chinese anyway.
	if language := assistantLanguage(in.Language, ""); chat.language == "" || language == "zh" {
		chat.language = language
	}
	reply, live := chat.manager.Answer(row.ID, in.ConfirmationID, in.Confirmed)
	text, results := assistant.SplitResults(reply.Text)
	c.JSON(http.StatusOK, gin.H{"live": live, "text": assistant.WithoutMarks(text), "results": results, "confirm": reply.Confirm, "changed": reply.Changed, "links": reply.Links})
}

// handleAssistantConversationsClear removes all of the user's conversations with the
// assistant, with all they held as deleteConversations says, and what any of them was
// waiting for.
func (s *Server) handleAssistantConversationsClear(c *gin.Context) {
	user := c.GetString("user_id")
	chat := s.assistantFor(user, c.GetString("email"))
	if !chat.busy.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, errAssistantBusy)
		return
	}
	defer chat.busy.Store(false)
	chat.chats.Lock()
	defer chat.chats.Unlock()
	rows, err := s.stores.Conversations.ListChats(user, agent.FeatureAssistant, 0)
	if err == nil {
		_, err = s.deleteConversations(user, rows)
	}
	if err != nil {
		SafeInternalError(c, "Clear the conversations", err)
		return
	}
	chat.manager.ForgetAll()
	c.Status(http.StatusNoContent)
}

// handleAssistantConversationDelete removes a conversation with all it held, as
// deleteConversations says, and what it was waiting for.
func (s *Server) handleAssistantConversationDelete(c *gin.Context) {
	row := s.assistantConversationOf(c)
	if row == nil {
		return
	}
	chat := s.assistantFor(c.GetString("user_id"), c.GetString("email"))
	if !chat.busy.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, errAssistantBusy)
		return
	}
	defer chat.busy.Store(false)
	chat.chats.Lock()
	defer chat.chats.Unlock()
	if _, err := s.deleteConversations(row.UserID, []agent.Conversation{*row}); err != nil {
		SafeInternalError(c, "Delete the conversation", err)
		return
	}
	c.Status(http.StatusNoContent)
}
