package agent

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// pendingTTL is how long a high-risk action waits for the user's confirmation.
const pendingTTL = 5 * time.Minute

// pendingAction is a high-risk skill step (delete, start, stop, close, key
// risk changes) held back until the user explicitly confirms. Confirmation is
// decided here in Go from the user's next message, never by the LLM, so prompt
// injection cannot approve an action on the user's behalf.
type pendingAction struct {
	summary string
	run     func() (string, error)
	expires time.Time
}

var confirmPattern = regexp.MustCompile(`(?i)^\s*(确认|確認|确定|確定|执行|執行|是|好|好的|可以|继续|繼續|confirm|confirmed|yes|y|ok|okay|go|proceed|do it)\s*[.!。！]*\s*$`)

func isConfirmation(text string) bool {
	return confirmPattern.MatchString(text)
}

// prefersChinese reports whether a reply to text should be in Chinese.
func prefersChinese(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (p *pendingAction) expired() bool { return time.Now().After(p.expires) }

// takePending returns and clears the pending action.
func (a *Agent) takePending() *pendingAction {
	p := a.pending
	a.pending = nil
	return p
}

// hold registers a high-risk action and returns the tool result that tells the
// LLM to ask the user for confirmation.
func (a *Agent) hold(summary string, run func() (string, error)) map[string]any {
	a.pending = &pendingAction{summary: summary, run: run, expires: time.Now().Add(pendingTTL)}
	return map[string]any{
		"status":  "needs_confirmation",
		"summary": summary,
		"instruction": "Nothing has been executed. Show the summary to the user and ask them to reply " +
			"\"确认\" / \"confirm\" to proceed; any other reply cancels it. Do not call the tool again for this action.",
	}
}

// runPending executes a confirmed action and formats the outcome.
func runPending(p *pendingAction, zh bool) string {
	result, err := p.run()
	if err != nil {
		if zh {
			return "❌ 执行失败：" + p.summary + "\n" + err.Error()
		}
		return "❌ Failed: " + p.summary + "\n" + err.Error()
	}
	var b strings.Builder
	if zh {
		b.WriteString("✅ 已执行：")
	} else {
		b.WriteString("✅ Done: ")
	}
	b.WriteString(p.summary)
	if result != "" {
		b.WriteString("\n")
		b.WriteString(result)
	}
	return b.String()
}
