package assistant

import (
	"context"
	"fmt"
	"strings"
	"vergex/internal/agent"
)

const (
	compactionThresholdTokens = 3000
	charsPerToken             = 3 // rough estimate for token counting
)

type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Memory manages conversation history. When ShortTerm exceeds the threshold, the model
// summarizes everything so far into LongTerm, whose size that keeps bounded.
type Memory struct {
	LongTerm  string    // Durable summary (survives compaction, user never sees this happen)
	ShortTerm []Message // Recent conversation (cleared on compaction)
	// client is whichever model is in use by the time it is needed.
	client func() agent.Client
}

func NewMemory(client func() agent.Client) *Memory {
	return &Memory{client: client}
}

// Add appends a message.
func (m *Memory) Add(role, content string) {
	m.ShortTerm = append(m.ShortTerm, Message{Role: role, Content: content})
}

// Tidy compacts if the conversation has grown past the threshold. It calls the model, so it
// runs once a reply is out, not on the way to one. The call is the conversation's, and is
// deleted with it.
func (m *Memory) Tidy(conversationID string) {
	if m.estimateTokens() > compactionThresholdTokens {
		m.compact(conversationID)
	}
}

// BuildContext returns context string for the agent's conversation history.
func (m *Memory) BuildContext() string {
	var sb strings.Builder
	if m.LongTerm != "" {
		sb.WriteString("[Summary of earlier conversation]\n")
		sb.WriteString(m.LongTerm)
		sb.WriteString("\n\n")
	}
	if len(m.ShortTerm) > 0 {
		sb.WriteString("[Recent conversation]\n")
		for _, msg := range m.ShortTerm {
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
		}
	}
	return sb.String()
}

// ResetFull clears everything including long-term memory
func (m *Memory) ResetFull() {
	m.ShortTerm = []Message{}
	m.LongTerm = ""
}

func (m *Memory) estimateTokens() int {
	total := 0
	for _, msg := range m.ShortTerm {
		total += len(msg.Content)
	}
	return total / charsPerToken
}

// compact summarizes short-term history into long-term memory.
// This runs silently - the user never sees it happen.
// If model call fails, short-term is preserved as-is (no data loss).
func (m *Memory) compact(conversationID string) {
	client := m.client()
	if client == nil || len(m.ShortTerm) == 0 {
		return
	}
	history := m.BuildContext()
	systemPrompt := `You are a conversation summarizer. Compress the following trading assistant conversation into a concise summary.

Must preserve:
- What the user is configuring (account/model/trader)
- Confirmed parameters (trading pairs, leverage, stop loss, indicators, etc.)
- Pending or missing parameters
- User preferences and requirements

Output: plain text summary, under 200 words.`

	req, err := agent.NewRequestBuilder().WithSystemPrompt(systemPrompt).AddUserMessage(history).Build()
	if err != nil {
		return
	}
	req.Ctx = agent.WithConversation(context.Background(), conversationID)
	summary, err := client.CallWithRequest(req)
	if err != nil {
		// Compaction failed: keep short-term as-is, never lose user data
		return
	}
	// The history it summarized included the summary before, so this replaces it.
	m.LongTerm = summary
	m.ShortTerm = []Message{}
}
