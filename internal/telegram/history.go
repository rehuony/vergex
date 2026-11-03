package telegram

import (
	"errors"

	"vergex/internal/agent"
	"vergex/internal/assistant"
	"vergex/internal/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// chatHistory keeps Telegram chats in agent.conversations, one per chat until /start
// begins another. The chat's id is in the conversation's context.
// chatContext is the key of a conversation's context that holds the chat's id.
const chatContext = "chat_id"

type chatHistory struct {
	conversations *agent.ConversationStore
	userID        string
}

func (h *chatHistory) Resume(chat string, limit int) (string, []assistant.Message) {
	conversation, err := h.conversations.LatestByContext(h.userID, agent.FeatureTelegram, chatContext, chat)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return h.Start(chat), nil
	}
	if err != nil {
		logger.Warnf("Telegram: read conversation: %v", err)
		return "", nil
	}
	rows, err := h.conversations.Transcript(conversation.ID, limit)
	if err != nil {
		logger.Warnf("Telegram: read messages: %v", err)
		return conversation.ID, nil
	}
	messages := make([]assistant.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, assistant.Message{Role: row.Role, Content: row.Content})
	}
	return conversation.ID, messages
}

func (h *chatHistory) Start(chat string) string {
	conversation := agent.Conversation{
		ID:      uuid.NewString(),
		UserID:  h.userID,
		Feature: agent.FeatureTelegram,
		Context: map[string]string{chatContext: chat},
	}
	if err := h.conversations.Open(&conversation); err != nil {
		logger.Warnf("Telegram: start conversation: %v", err)
		return ""
	}
	return conversation.ID
}

func (h *chatHistory) Append(conversationID, role string, message agent.Message) {
	assistant.Record(h.conversations, conversationID, role, message)
}

func (h *chatHistory) Exists(conversationID string) bool {
	return assistant.HistoryExists(h.conversations, conversationID)
}
