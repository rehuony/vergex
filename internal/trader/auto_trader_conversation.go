package trader

import (
	"context"
	"time"
	"vergex/internal/agent"
	"vergex/internal/logger"
	"vergex/internal/service"
	traderagent "vergex/internal/trader/agent"

	"github.com/google/uuid"
)

// conversationWait bounds each write of a cycle's conversation. Keeping what was said
// is a record, and never worth more than this to the trading loop: a write that fails
// or takes too long is logged and the cycle goes on.
const conversationWait = 3 * time.Second

// openConversation starts the conversation of a cycle's call to the model. The call
// reports to it, so it has to exist first; it holds the loop up only until the wait
// runs out, and then the call goes ahead without one.
func (at *AutoTrader) openConversation() string {
	if at.stores == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), conversationWait)
	defer cancel()
	conversation := agent.Conversation{
		ID:           uuid.NewString(),
		UserID:       at.userID,
		Feature:      agent.FeatureTrader,
		SourceID:     at.id,
		AgentModelID: at.config.AgentModelID,
	}
	if err := at.stores.Conversations.WithContext(ctx).Open(&conversation); err != nil {
		logger.Warnf("⚠️ [%s] Failed to open a conversation: %v", at.name, err)
		return ""
	}
	return conversation.ID
}

// keepConversation stores what was sent to the model and what it said, in the
// background: orders are waiting on this cycle.
func (at *AutoTrader) keepConversation(decision *traderagent.FullDecision) {
	if at.stores == nil || decision == nil || decision.ConversationID == "" {
		return
	}
	messages := []agent.ConversationMessage{
		{Role: agent.RoleSystem, Content: decision.SystemPrompt},
		{Role: agent.RoleUser, Content: decision.UserPrompt},
	}
	if decision.RawResponse != "" {
		messages = append(messages, agent.ConversationMessage{Role: agent.RoleAssistant, Content: decision.RawResponse, Thinking: decision.CoTTrace})
	}
	service.Go("trader-conversation", func() {
		ctx, cancel := context.WithTimeout(context.Background(), conversationWait)
		defer cancel()
		if err := at.stores.Conversations.WithContext(ctx).Append(decision.ConversationID, messages...); err != nil {
			logger.Warnf("⚠️ [%s] Failed to keep a conversation: %v", at.name, err)
		}
	})
}
