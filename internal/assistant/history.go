package assistant

import (
	"encoding/json"
	"vergex/internal/agent"
	"vergex/internal/logger"
)

// roleContext is data sent with a turn that the user did not type, such as the
// account state, kept apart from their words.
const roleContext = "context"

// restoredMessages is how much of a conversation a restart brings back.
const restoredMessages = 40

// History keeps what is said in a chat, so it survives a restart. A failure to
// record never stops a reply, so an implementation logs its own errors: an empty
// conversation ID means there is nowhere to record for now.
type History interface {
	// Resume returns the chat's current conversation and the latest things said in
	// it, starting one if there is none.
	Resume(key string, limit int) (conversationID string, messages []Message)
	// Start begins a new conversation for the chat.
	Start(key string) (conversationID string)
	// Append adds one message to a conversation, under a role that says what it is
	// rather than how the model reads it.
	Append(conversationID, role string, message agent.Message)
	// Exists says whether a conversation is still there. One that cannot be checked counts
	// as there.
	Exists(conversationID string) bool
}

// HistoryExists checks a conversation for a History, logging what it cannot read.
func HistoryExists(conversations *agent.ConversationStore, conversationID string) bool {
	there, err := conversations.Exists(conversationID)
	if err != nil {
		logger.Warnf("Assistant: check conversation: %v", err)
		return true
	}
	return there
}

// Record stores one message of a conversation for a History. A failure is logged, since
// it must never stop a reply.
func Record(conversations *agent.ConversationStore, conversationID, role string, message agent.Message) {
	row := agent.ConversationMessage{Role: role, Content: message.Content, Thinking: message.ReasoningContent, ToolCallID: message.ToolCallID}
	if len(message.ToolCalls) > 0 {
		calls, err := json.Marshal(message.ToolCalls)
		if err != nil {
			logger.Warnf("Assistant: encode tool calls: %v", err)
			return
		}
		row.ToolCalls = string(calls)
	}
	if err := conversations.Append(conversationID, row); err != nil {
		logger.Warnf("Assistant: record message: %v", err)
	}
}

// resume connects the agent to its chat's history and restores its memory.
func (a *Agent) resume(history History, key string) {
	if history == nil {
		return
	}
	a.history = history
	a.key = key
	id, messages := history.Resume(key, restoredMessages)
	a.conversationID.Store(id)
	a.memory.ShortTerm = messages
}

// rejoin starts the chat afresh when its conversation was deleted since its last turn,
// wherever from: what it remembered of it, and anything waiting, went with it.
func (a *Agent) rejoin() {
	if id := a.conversation(); a.history != nil && id != "" && !a.history.Exists(id) {
		a.ResetMemory()
	}
}

func (a *Agent) conversation() string {
	id, _ := a.conversationID.Load().(string)
	return id
}

func (a *Agent) record(role string, message agent.Message) {
	if id := a.conversation(); a.history != nil && id != "" {
		a.history.Append(id, role, message)
	}
}
