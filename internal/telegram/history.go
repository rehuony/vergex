package telegram

import (
	"encoding/json"
	"errors"
	"strconv"

	"vergex/internal/agent"
	"vergex/internal/logger"
	"vergex/internal/store"
	telegramagent "vergex/internal/telegram/agent"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// chatHistory keeps Telegram chats in agent.conversations, one per chat until /start
// begins another.
type chatHistory struct {
	conversations *store.AgentConversationStore
	userID        string
}

func (h *chatHistory) Resume(chatID int64, limit int) (string, []telegramagent.Message) {
	conversation, err := h.conversations.Latest(h.userID, store.FeatureTelegram, strconv.FormatInt(chatID, 10))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return h.Start(chatID), nil
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
	messages := make([]telegramagent.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, telegramagent.Message{Role: row.Role, Content: row.Content})
	}
	return conversation.ID, messages
}

func (h *chatHistory) Start(chatID int64) string {
	conversation := store.AgentConversation{
		ID:       uuid.NewString(),
		UserID:   h.userID,
		Feature:  store.FeatureTelegram,
		SourceID: strconv.FormatInt(chatID, 10),
	}
	if err := h.conversations.Open(&conversation); err != nil {
		logger.Warnf("Telegram: start conversation: %v", err)
		return ""
	}
	return conversation.ID
}

func (h *chatHistory) Append(conversationID, role string, message agent.Message) {
	row := store.AgentMessage{Role: role, Content: message.Content, Thinking: message.ReasoningContent, ToolCallID: message.ToolCallID}
	if len(message.ToolCalls) > 0 {
		calls, err := json.Marshal(message.ToolCalls)
		if err != nil {
			logger.Warnf("Telegram: encode tool calls: %v", err)
			return
		}
		row.ToolCalls = string(calls)
	}
	if err := h.conversations.Append(conversationID, row); err != nil {
		logger.Warnf("Telegram: record message: %v", err)
	}
}
