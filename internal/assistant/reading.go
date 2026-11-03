package assistant

import (
	"regexp"
	"strings"
	"vergex/internal/agent"
)

// marks are what opens a line Go writes for Telegram, where they are how a line reads at a
// glance. An interface has icons for that.
var marks = regexp.MustCompile(`(?m)^(✅|❌|↩️|⌛|🛑|⚠️|⏳)\s*`)

// WithoutMarks is a reply as a page shows it.
func WithoutMarks(text string) string { return marks.ReplaceAllString(text, "") }

// ReadMessage is one thing said in a conversation as a page shows it.
type ReadMessage struct {
	Role    string         `json:"role"`
	Content string         `json:"content"`
	Results []ActionResult `json:"results,omitempty"`
}

// ForReading is what was said in a conversation as a page shows it: the words, without what
// the record keeps for the model about buttons and what was held, how confirmed actions went
// in parts, and a question asked again after a failure only once.
func ForReading(messages []agent.ConversationMessage) []ReadMessage {
	out := make([]ReadMessage, 0, len(messages))
	for _, message := range messages {
		switch message.Content {
		case pressedConfirm, pressedCancel, cancelledMark:
			continue
		}
		content := message.Content
		if at := strings.Index(content, waitingMark); at >= 0 {
			content = strings.TrimSpace(content[:at])
		}
		content, results := SplitResults(content)
		content = WithoutMarks(content)
		if content == "" && len(results) == 0 {
			continue
		}
		if last := len(out) - 1; last >= 0 && len(results) == 0 && out[last].Role == message.Role && out[last].Content == content {
			continue
		}
		out = append(out, ReadMessage{Role: message.Role, Content: content, Results: results})
	}
	return out
}
