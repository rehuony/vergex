package assistant

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"vergex/internal/agent"

	"github.com/google/uuid"
)

// pendingTTL is how long a high-risk action waits for the user's confirmation.
const pendingTTL = 5 * time.Minute

// What stands in the record for a button, so the model knows what happened.
const (
	pressedConfirm = "[pressed confirm]"
	pressedCancel  = "[pressed cancel]"
	cancelledMark  = "[cancelled]"
)

// waitingMark opens what the record adds to a reply when something is held.
const waitingMark = "[Waiting for the user's confirmation:"

// pendingAction is a high-risk skill step (delete, start, stop, close, key
// risk changes) held back until the user explicitly confirms.
type pendingAction struct {
	summary string
	run     func() (string, error)
	// destructive says it cannot be taken back, so a surface can say so on the button.
	destructive bool
	// item is the same thing in parts, for a surface that lays it out.
	item ConfirmItem
}

// The kinds of action a ConfirmItem can be, which a surface names in its own words.
const (
	actionCreate = "create"
	actionStart  = "start"
	actionStop   = "stop"
	actionDelete = "delete"
	actionUpdate = "update"
	actionClose  = "close"
	// Deleting one conversation or plan, and deleting several at once, whose target is how
	// many.
	actionDeleteChat  = "delete_conversation"
	actionDeleteChats = "delete_conversations"
	actionDeletePlan  = "delete_plan"
	actionDeletePlans = "delete_plans"
)

// What kind of money an account holds, where that is known.
const (
	moneyReal    = "real"
	moneyTestnet = "testnet"
)

// ConfirmItem is one action waiting for an answer, in parts: what it does, to what, on which
// account and with what kind of money. All of it comes from Go, never from the model.
type ConfirmItem struct {
	Action      string   `json:"action,omitempty"`
	Target      string   `json:"target"`
	Account     string   `json:"account,omitempty"`
	Money       string   `json:"money,omitempty"`
	Details     []string `json:"details,omitempty"`
	Destructive bool     `json:"destructive,omitempty"`
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
	ID    string   `json:"id"`
	Lines []string `json:"lines"`
	// Destructive says one of them cannot be taken back.
	Destructive bool `json:"destructive"`
	// Items are the same lines in parts.
	Items []ConfirmItem `json:"items"`
}

// Reply is what a turn came to.
type Reply struct {
	Text string
	// Confirm is what now waits for an answer, if anything does.
	Confirm *Confirmation
	// Failed says the text is why the model could not answer, not an answer.
	Failed bool
	// Changed says a skill changed something, so what shows it is out of date.
	Changed bool
	// Links are places a page can go next, from Go rather than the model.
	Links []Link
	// Proposals are what the turn suggests for forms on the page it was asked from.
	Proposals []Proposal

	// The rest is what Text says, in parts, for a surface that lays it out itself.

	// Results are the confirmed actions this turn ran, and how each went.
	Results []ActionResult
	// Dropped is what waited for an answer and went without one.
	Dropped *Dropped
	// Ended says why the turn ended without an answer from the model, one of the End kinds.
	Ended string
	// Failure is why the model could not answer, when Ended is EndFailed.
	Failure *agent.Failure
	// Said is the model's own words.
	Said string
}

// Why a turn ended without an answer from the model.
const (
	EndStopped     = "stopped"
	EndTimeout     = "timeout"
	EndSteps       = "steps"
	EndEmpty       = "empty"
	EndUnavailable = "unavailable"
	EndFailed      = "failed"
)

// Dropped is what waited for an answer and went without one: cancelled, or replaced by
// another message, or out of time.
type Dropped struct {
	Expired bool          `json:"expired"`
	Items   []ConfirmItem `json:"items"`
}

// Link is a place in the web app worth going to, with Go's words for it. Telegram has no
// use for them.
type Link struct {
	To    string `json:"to"`
	Label string `json:"label"`
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
	confirmation := &Confirmation{ID: a.pending.id, Lines: make([]string, len(a.pending.actions)), Items: a.pending.items()}
	for i, action := range a.pending.actions {
		confirmation.Lines[i] = action.summary
		confirmation.Destructive = confirmation.Destructive || action.destructive
	}
	return confirmation
}

// items is the batch in parts.
func (p *pendingBatch) items() []ConfirmItem {
	items := make([]ConfirmItem, len(p.actions))
	for i, action := range p.actions {
		items[i] = action.itemOf()
	}
	return items
}

// itemOf is an action in parts. One held without them is named by its summary.
func (action pendingAction) itemOf() ConfirmItem {
	item := action.item
	if item.Target == "" {
		item.Target = action.summary
	}
	item.Destructive = action.destructive
	return item
}

// hold adds a high-risk action to what this turn is holding, and returns the tool result
// that tells the model the system will ask. The same action held twice counts once.
func (a *Agent) hold(summary string, run func() (string, error)) map[string]any {
	return a.holdAction(pendingAction{summary: summary, run: run})
}

// holdItem holds an action that a surface can lay out in parts.
func (a *Agent) holdItem(item ConfirmItem, summary string, run func() (string, error)) map[string]any {
	return a.holdAction(pendingAction{summary: summary, run: run, item: item, destructive: item.Destructive})
}

// confirmItem starts an item with what is known of the account it acts on.
func (a *Agent) confirmItem(action, target, accountID string, details ...string) ConfirmItem {
	item := ConfirmItem{Action: action, Target: target, Details: details, Destructive: action == actionDelete}
	accounts, err := a.accounts()
	if err != nil {
		return item
	}
	for _, e := range accounts {
		if e.ID == accountID {
			item.Account, item.Money = e.Name, moneyReal
			if e.Testnet {
				item.Money = moneyTestnet
			}
		}
	}
	return item
}

// patchLines is a config patch as what it sets, one key to a line, by its path.
func patchLines(patch map[string]any) []string {
	var lines []string
	var walk func(path string, value any)
	walk = func(path string, value any) {
		inner, ok := value.(map[string]any)
		if !ok {
			text, err := json.Marshal(value)
			if err != nil {
				return
			}
			lines = append(lines, path+": "+truncate(strings.Trim(string(text), `"`), 200))
			return
		}
		for key, nested := range inner {
			walk(strings.TrimPrefix(path+"."+key, "."), nested)
		}
	}
	walk("", patch)
	sort.Strings(lines)
	return lines
}

func (a *Agent) holdAction(held pendingAction) map[string]any {
	if a.pending == nil {
		a.pending = &pendingBatch{id: uuid.NewString()[:8]}
	}
	a.pending.expires = time.Now().Add(pendingTTL)
	found := false
	for i := range a.pending.actions {
		if a.pending.actions[i].summary == held.summary {
			a.pending.actions[i], found = held, true
		}
	}
	if !found {
		a.pending.actions = append(a.pending.actions, held)
	}
	return map[string]any{
		"status":  "needs_confirmation",
		"summary": held.summary,
		"instruction": "Nothing has been executed. The system itself shows the user this summary with " +
			"confirm and cancel buttons once you finish, so do not repeat it or ask them to reply. " +
			"Do not call the tool again for this action, and never say it is done.",
	}
}

// How a line of runPending opens, which SplitResults reads back.
const (
	doneZh   = "✅ 已执行："
	doneEn   = "✅ Done: "
	failedZh = "❌ 执行失败："
	failedEn = "❌ Failed: "
)

// runPending executes everything in a confirmed batch, one after another, and says how
// each went, in words and in parts. One failing does not stop the rest: stopping several
// traders is worth doing even when one of them refuses.
func runPending(p *pendingBatch, zh bool) (string, []ActionResult) {
	lines := make([]string, 0, len(p.actions))
	results := make([]ActionResult, 0, len(p.actions))
	for _, action := range p.actions {
		result, err := action.run()
		item := action.itemOf()
		switch {
		case err != nil && zh:
			lines = append(lines, failedZh+action.summary+"\n"+err.Error())
		case err != nil:
			lines = append(lines, failedEn+action.summary+"\n"+err.Error())
		case zh:
			lines = append(lines, joinResult(doneZh+action.summary, result))
		default:
			lines = append(lines, joinResult(doneEn+action.summary, result))
		}
		if err != nil {
			results = append(results, ActionResult{Text: action.summary, Note: err.Error(), Item: &item})
		} else {
			results = append(results, ActionResult{OK: true, Text: action.summary, Note: result, Item: &item})
		}
	}
	return strings.Join(lines, "\n"), results
}

// ActionResult is how one confirmed action went, for a surface that lays it out.
type ActionResult struct {
	OK   bool   `json:"ok"`
	Text string `json:"text"`
	// Note is what came with it: why it failed, or a warning about what it did.
	Note string `json:"note,omitempty"`
	// Item is the action in parts, where the turn that ran it says so.
	Item *ConfirmItem `json:"item,omitempty"`
}

// SplitResults reads what runPending wrote back into results, and leaves whatever else the
// text says. It reads the words rather than a mark, so a reply of the model's that merely
// opens with one stays a reply.
func SplitResults(text string) (string, []ActionResult) {
	var rest []string
	var results []ActionResult
	for _, line := range strings.Split(text, "\n") {
		done, isDone := cutAny(line, doneZh, doneEn)
		failed, isFailed := cutAny(line, failedZh, failedEn)
		switch {
		case isDone:
			results = append(results, ActionResult{OK: true, Text: done})
		case isFailed:
			results = append(results, ActionResult{Text: failed})
		case len(results) > 0:
			last := &results[len(results)-1]
			last.Note = strings.TrimSpace(last.Note + "\n" + line)
		default:
			rest = append(rest, line)
		}
	}
	return strings.TrimSpace(strings.Join(rest, "\n")), results
}

func cutAny(line string, prefixes ...string) (string, bool) {
	for _, prefix := range prefixes {
		if after, ok := strings.CutPrefix(line, prefix); ok {
			return after, true
		}
	}
	return "", false
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
	a.rejoin()
	if a.pending == nil || a.pending.id != id {
		return Reply{}, false
	}
	p := a.takePending()
	zh := a.zh("")
	said := pressedCancel
	if confirmed {
		said = pressedConfirm
	}
	a.record("user", agentMessage(said))
	switch {
	case !confirmed:
		reply := a.finish(said, status(zh, "↩️ 已取消：\n"+p.summary(), "↩️ Cancelled:\n"+p.summary()), "")
		reply.Dropped = &Dropped{Items: p.items()}
		return reply, true
	case p.expired():
		reply := a.finish(said, expiredText(p, zh), "")
		reply.Dropped = &Dropped{Expired: true, Items: p.items()}
		return reply, true
	}
	text, results := runPending(p, zh)
	reply := a.finish(said, text, "")
	reply.Results = results
	return reply, true
}

func expiredText(p *pendingBatch, zh bool) string {
	return status(zh, "⌛ 确认已超时，操作未执行：\n"+p.summary(), "⌛ Confirmation expired, nothing was executed:\n"+p.summary())
}

// CancelPending drops what is waiting for an answer, and reports whether anything was.
func (a *Agent) CancelPending() (Reply, bool) {
	p := a.takePending()
	if p == nil {
		return Reply{}, false
	}
	zh := a.zh("")
	reply := a.finish(cancelledMark, status(zh, "↩️ 已取消：\n"+p.summary(), "↩️ Cancelled:\n"+p.summary()), "")
	reply.Dropped = &Dropped{Items: p.items()}
	return reply, true
}
