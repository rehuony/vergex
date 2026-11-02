package agent

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// pendingTTL is how long a high-risk action waits for the user's confirmation.
const pendingTTL = 5 * time.Minute

// pendingAction is a high-risk skill step (delete, start, stop, close, key
// risk changes) held back until the user explicitly confirms.
type pendingAction struct {
	summary string
	run     func() (string, error)
}

// pendingBatch is everything one turn held. Confirmation is decided here in Go, from the
// user's next message or a button, never by the model, so prompt injection cannot approve
// an action on the user's behalf. The ID ties a button to this batch, so an old button can
// never approve something newer.
type pendingBatch struct {
	id      string
	actions []pendingAction
	expires time.Time
}

// Confirmation is what waits for the user's answer, in Go's words: what they confirm is
// what they read, whatever the model wrote around it.
type Confirmation struct {
	ID    string
	Lines []string
}

// Reply is what a turn came to.
type Reply struct {
	Text string
	// Confirm is what now waits for an answer, if anything does.
	Confirm *Confirmation
}

var confirmPattern = regexp.MustCompile(`(?i)^\s*(确认|確認|确定|確定|执行|執行|是|好|好的|可以|继续|繼續|confirm|confirmed|yes|y|ok|okay|go|proceed|do it)\s*[.!。！]*\s*$`)

func isConfirmation(text string) bool {
	return confirmPattern.MatchString(text)
}

// PrefersChinese reports whether a reply to text should be in Chinese.
func PrefersChinese(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (p *pendingBatch) expired() bool { return time.Now().After(p.expires) }

// summary is everything in the batch on one line each.
func (p *pendingBatch) summary() string {
	lines := make([]string, len(p.actions))
	for i, action := range p.actions {
		lines[i] = action.summary
	}
	return strings.Join(lines, "\n")
}

// takePending returns and clears the pending batch.
func (a *Agent) takePending() *pendingBatch {
	p := a.pending
	a.pending = nil
	return p
}

// confirmation says what now waits for an answer.
func (a *Agent) confirmation() *Confirmation {
	if a.pending == nil {
		return nil
	}
	lines := make([]string, len(a.pending.actions))
	for i, action := range a.pending.actions {
		lines[i] = action.summary
	}
	return &Confirmation{ID: a.pending.id, Lines: lines}
}

// hold adds a high-risk action to what this turn is holding, and returns the tool result
// that tells the model the system will ask. The same action held twice counts once.
func (a *Agent) hold(summary string, run func() (string, error)) map[string]any {
	if a.pending == nil {
		a.pending = &pendingBatch{id: uuid.NewString()[:8]}
	}
	a.pending.expires = time.Now().Add(pendingTTL)
	held := pendingAction{summary: summary, run: run}
	found := false
	for i := range a.pending.actions {
		if a.pending.actions[i].summary == summary {
			a.pending.actions[i], found = held, true
		}
	}
	if !found {
		a.pending.actions = append(a.pending.actions, held)
	}
	return map[string]any{
		"status":  "needs_confirmation",
		"summary": summary,
		"instruction": "Nothing has been executed. The system itself shows the user this summary with " +
			"confirm and cancel buttons once you finish, so do not repeat it or ask them to reply. " +
			"Do not call the tool again for this action, and never say it is done.",
	}
}

// runPending executes everything in a confirmed batch, one after another, and says how
// each went. One failing does not stop the rest: stopping several traders is worth doing
// even when one of them refuses.
func runPending(p *pendingBatch, zh bool) string {
	lines := make([]string, 0, len(p.actions))
	for _, action := range p.actions {
		result, err := action.run()
		switch {
		case err != nil && zh:
			lines = append(lines, "❌ 执行失败："+action.summary+"\n"+err.Error())
		case err != nil:
			lines = append(lines, "❌ Failed: "+action.summary+"\n"+err.Error())
		case zh:
			lines = append(lines, joinResult("✅ 已执行："+action.summary, result))
		default:
			lines = append(lines, joinResult("✅ Done: "+action.summary, result))
		}
	}
	return strings.Join(lines, "\n")
}

func joinResult(line, result string) string {
	if result == "" {
		return line
	}
	return line + "\n" + result
}

// Answer resolves a batch from its button. It reports false when id is not the batch now
// waiting: one already answered, cancelled or replaced.
func (a *Agent) Answer(id string, confirmed bool) (Reply, bool) {
	if a.pending == nil || a.pending.id != id {
		return Reply{}, false
	}
	p := a.takePending()
	zh := a.zh("")
	said := "[pressed cancel]"
	if confirmed {
		said = "[pressed confirm]"
	}
	a.record("user", agentMessage(said))
	var text string
	switch {
	case !confirmed:
		text = status(zh, "↩️ 已取消：\n"+p.summary(), "↩️ Cancelled:\n"+p.summary())
	case p.expired():
		text = status(zh, "⌛ 确认已超时，操作未执行：\n"+p.summary(), "⌛ Confirmation expired, nothing was executed:\n"+p.summary())
	default:
		text = runPending(p, zh)
	}
	return a.finish(said, text, ""), true
}

// CancelPending drops what is waiting for an answer, and reports whether anything was.
func (a *Agent) CancelPending() (Reply, bool) {
	p := a.takePending()
	if p == nil {
		return Reply{}, false
	}
	zh := a.zh("")
	return a.finish("[cancelled]", status(zh, "↩️ 已取消：\n"+p.summary(), "↩️ Cancelled:\n"+p.summary()), ""), true
}
