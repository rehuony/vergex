package agent

import "vergex/internal/agent"

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
	Resume(chatID int64, limit int) (conversationID string, messages []Message)
	// Start begins a new conversation for the chat.
	Start(chatID int64) (conversationID string)
	// Append adds one message to a conversation, under a role that says what it is
	// rather than how the model reads it.
	Append(conversationID, role string, message agent.Message)
}

// resume connects the agent to its chat's history and restores its memory.
func (a *Agent) resume(history History, chatID int64) {
	if history == nil {
		return
	}
	a.history = history
	a.chatID = chatID
	id, messages := history.Resume(chatID, restoredMessages)
	a.conversationID.Store(id)
	a.memory.ShortTerm = messages
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
