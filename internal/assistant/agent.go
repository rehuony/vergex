package assistant

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"vergex/internal/agent"
	"vergex/internal/logger"
	"vergex/internal/security/auth"
)

const maxIterations = 10

// Agent is a stateful agent for one chat, wherever that is reached from. The model sees
// only the task-level skills in skills.go; high-risk steps wait in pending until the user
// confirms them.
type Agent struct {
	api          *apiClient
	getClient    func() agent.Client
	memory       *Memory
	systemPrompt string
	userID       string
	pending      *pendingBatch
	// tools are the skills on offer where this chat is reached from.
	tools []agent.Tool
	// links are where the turn under way points, until its reply carries them.
	links []Link
	// page is what the turn under way says the user has open, and proposals are what it
	// suggests for the forms on it.
	page      string
	proposals []Proposal
	// language is the one chosen with /lang, empty until one is.
	language func() string
	// lastZh is the language of the last message, for what answers a button.
	lastZh bool

	history        History
	key            string
	conversationID atomic.Value // string
}

// New creates an Agent for one chat session.
func New(apiPort int, token func() (string, error), userID string, getClient func() agent.Client, systemPrompt string, language func() string) *Agent {
	return &Agent{
		api:          newAPIClient(apiPort, token),
		getClient:    getClient,
		memory:       NewMemory(getClient),
		systemPrompt: systemPrompt,
		userID:       userID,
		language:     language,
		tools:        skillTools,
	}
}

// zh says whether what Go writes should be in Chinese: the language chosen with /lang, or
// until one is, the language of the message being answered.
func (a *Agent) zh(message string) bool {
	switch a.language() {
	case "zh":
		return true
	case "en":
		return false
	}
	if message != "" {
		a.lastZh = PrefersChinese(message)
	}
	return a.lastZh
}

func agentMessage(content string) agent.Message { return agent.Message{Content: content} }

// addLink says where a page could go next, for the reply this turn comes to.
func (a *Agent) addLink(to, label string) { a.links = append(a.links, Link{To: to, Label: label}) }

// languageName is a /lang setting as the model reads it.
func languageName(lang string) string {
	switch lang {
	case "zh":
		return "Simplified Chinese"
	case "en":
		return "English"
	}
	return ""
}

// CallToken creates a JWT for one of a skill's calls to the local API. userID must match
// the actual registered user's ID, so what a skill changes is theirs.
func CallToken(userID string) (string, error) {
	return auth.GenerateJWT(userID, "assistant@internal")
}

func status(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// buildAccountContext summarizes models, accounts and traders
// (plus balance and statistics of running traders) for the first turn of a
// conversation, so the model can refer to existing resources by name.
func (a *Agent) buildAccountContext() string {
	var sb strings.Builder
	sb.WriteString("[Current Account State]\n\n")

	sb.WriteString("## Models\n")
	if models, err := a.models(); err != nil {
		sb.WriteString("  unavailable: " + err.Error() + "\n")
	} else {
		for _, m := range models {
			sb.WriteString(fmt.Sprintf("  • %s  id=%s  provider=%s  [%s, key %s]\n", m.Name, m.ID, m.Provider,
				status(m.Enabled, "enabled", "disabled"), status(m.HasAPIKey, "set", "missing")))
		}
	}

	sb.WriteString("\n## Accounts\n")
	if accounts, err := a.accounts(); err != nil {
		sb.WriteString("  unavailable: " + err.Error() + "\n")
	} else {
		for _, e := range accounts {
			sb.WriteString(fmt.Sprintf("  • %s  id=%s  type=%s\n", e.label(), e.ID, e.Venue))
		}
	}

	sb.WriteString("\n## Traders\n")
	traders, err := a.traders()
	if err != nil {
		sb.WriteString("  unavailable: " + err.Error() + "\n")
	}
	for _, t := range traders {
		sb.WriteString(fmt.Sprintf("  • %s  id=%s  kind=%s  [%s]\n", t.Name, t.TraderID, orDash(t.Kind), status(t.Running, "RUNNING", "stopped")))
	}
	for _, t := range traders {
		if !t.Running {
			continue
		}
		base := "/api/traders/" + url.PathEscape(t.TraderID)
		for _, part := range []struct{ label, path string }{{"Account", "/account"}, {"Statistics", "/statistics"}} {
			raw, err := a.api.raw("GET", base+part.path, nil)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("\n%s [%s]: %s\n", part.label, t.Name, truncate(string(raw), 1500)))
		}
	}
	return sb.String()
}

// Progress is who hears how a turn is going.
type Progress struct {
	// Stage hears what is under way, in words for the user.
	Stage func(string)
	// Text hears the reply as it is written.
	Text func(string)
}

func (p *Progress) stage(stage string) {
	if p != nil && p.Stage != nil {
		p.Stage(stage)
	}
}

func (p *Progress) text(text string) {
	if p != nil && p.Text != nil {
		p.Text(text)
	}
}

// Run processes one user message.
//
// A pending high-risk action is resolved first, deterministically: an explicit
// confirmation executes it without consulting the model; any other message
// cancels it and is then handled normally. Otherwise the model loops over skill
// calls until it returns a plain-text reply. On the first message of a
// conversation the account state is injected.
// progress hears what is under way, and may be nil. Ending ctx ends the turn: a turn stopped
// by the user drops what it was holding, one that ran out of time still shows it.
func (a *Agent) Run(ctx context.Context, userMessage string, progress *Progress) Reply {
	a.rejoin()
	a.record("user", agent.Message{Content: userMessage})
	zh := a.zh(userMessage)
	notice := ""
	var dropped *Dropped
	if p := a.takePending(); p != nil {
		switch {
		case isConfirmation(userMessage) && p.expired():
			reply := a.finish(userMessage, expiredText(p, zh), "")
			reply.Dropped = &Dropped{Expired: true, Items: p.items()}
			return reply
		case isConfirmation(userMessage):
			logger.Infof("Agent: user confirmed pending actions: %s", p.summary())
			progress.stage(status(zh, "正在执行…", "Running it…"))
			text, results := runPending(p, zh)
			reply := a.finish(userMessage, text, "")
			reply.Results = results
			return reply
		default:
			notice = status(zh, "↩️ 已取消待确认操作：\n"+p.summary(), "↩️ Cancelled the pending actions:\n"+p.summary())
			dropped = &Dropped{Items: p.items()}
		}
	}
	// Whatever the turn comes to, it also says what it dropped.
	ended := func(reply Reply, why string) Reply {
		reply.Dropped, reply.Ended = dropped, why
		return reply
	}

	client := a.getClient()
	if client == nil {
		return ended(Reply{Text: status(zh, "助手不可用。请在网页上配置模型", "Assistant unavailable. Please configure a model in the web app")}, EndUnavailable)
	}

	// Build initial user message: prepend account state on first turn, history on subsequent turns.
	histCtx := a.memory.BuildContext()
	var firstUserContent string
	if histCtx == "" {
		firstUserContent = a.buildAccountContext() + "\n[User Message]\n" + userMessage
	} else {
		firstUserContent = histCtx + "\n---\nUser: " + userMessage
	}
	a.page = pageOf(ctx)
	firstUserContent += pageContext(ctx)
	if notice != "" {
		firstUserContent += "\n[System: the previously pending action was cancelled because the user did not confirm it.]"
	}
	if name := languageName(a.language()); name != "" {
		firstUserContent += "\n[System: the user's language setting is " + name + ". Reply in the language of their message, and in that setting only when the message does not show one.]"
	}

	a.record(roleContext, agent.Message{Content: firstUserContent})

	turnMsgs := []agent.Message{agent.NewUserMessage(firstUserContent)}
	builder := func() *agent.RequestBuilder {
		b := agent.NewRequestBuilder().WithSystemPrompt(a.systemPrompt).AddConversationHistory(turnMsgs)
		for _, t := range a.tools {
			b.AddTool(t)
		}
		return b.WithToolChoice("auto")
	}

	progress.stage(status(zh, "正在思考…", "Thinking…"))
	for i := 0; i < maxIterations; i++ {
		if ctx.Err() != nil {
			return ended(a.interrupted(ctx, userMessage, notice, zh))
		}
		req, err := builder().Build()
		if err != nil {
			logger.Errorf("Agent: failed to build request: %v", err)
			break
		}
		req.Ctx = agent.WithConversation(ctx, a.conversation())

		resp, err := agent.Stream(client, req, progress.text)
		if ctx.Err() != nil {
			return ended(a.interrupted(ctx, userMessage, notice, zh))
		}
		if err != nil {
			logger.Errorf("Agent: model call failed (iteration %d): %v", i+1, err)
			failure := agent.Classify(client, err)
			reply := ended(Reply{Text: "❌ " + failure.Describe(zh), Confirm: a.confirmation(), Failed: true, Changed: a.api.takeWrote()}, EndFailed)
			reply.Failure = failure
			return reply
		}

		// No tool calls → model returned a final text reply. With nothing held, an empty one
		// would leave the user looking at nothing.
		if len(resp.ToolCalls) == 0 {
			content := strings.TrimSpace(resp.Content)
			if content == "" && a.pending == nil {
				return ended(a.finish(userMessage, joinNotice(notice, status(zh,
					"⚠️ 模型没有返回内容。请重试，或在设置里检查模型",
					"⚠️ The model returned nothing. Try again, or check the model in Settings")), resp.ReasoningContent), EndEmpty)
			}
			reply := ended(a.finish(userMessage, joinNotice(notice, content), resp.ReasoningContent), "")
			reply.Said = content
			return reply
		}

		a.record("assistant", agent.Message{Content: resp.Content, ReasoningContent: resp.ReasoningContent, ToolCalls: resp.ToolCalls})
		turnMsgs = append(turnMsgs, agent.Message{Role: "assistant", ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			logger.Infof("Agent: iter=%d skill=%s id=%s", i+1, tc.Function.Name, tc.ID)
			progress.stage(stageOf(tc.Function.Name, tc.Function.Arguments, zh))
			result := agent.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    a.runSkill(tc.Function.Name, tc.Function.Arguments),
			}
			a.record("tool", result)
			turnMsgs = append(turnMsgs, result)
		}
	}

	// Safety: max iterations reached. Nothing says the work got done, so neither does this.
	logger.Warnf("Agent: max iterations (%d) reached for message: %q", maxIterations, userMessage)
	return ended(a.finish(userMessage, joinNotice(notice, status(zh,
		"⚠️ 这件事的步骤太多，我没有做完。可以拆开来问，或在网页上查看",
		"⚠️ That took too many steps and I did not finish it. Try asking in smaller parts, or look in the web app")), ""), EndSteps)
}

// interrupted ends a turn whose context ended, and says which way. Whatever a stopped turn
// was holding goes with it, since being told to stop is no time to ask for a confirmation.
func (a *Agent) interrupted(ctx context.Context, userMessage, notice string, zh bool) (Reply, string) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return a.finish(userMessage, joinNotice(notice, status(zh,
			"⌛ 这件事超过了时限，我没有做完。已经执行的操作不会撤销",
			"⌛ That ran out of time and I did not finish it. Anything already done stays done")), ""), EndTimeout
	}
	a.pending = nil
	return a.finish(userMessage, joinNotice(notice, status(zh,
		"🛑 已停止。已经执行的操作不会撤销",
		"🛑 Stopped. Anything already done stays done")), ""), EndStopped
}

func joinNotice(notice, reply string) string {
	if notice == "" {
		return reply
	}
	return notice + "\n\n" + reply
}

// finish records the exchange and delivers the reply, with whatever now waits for an
// answer. That goes on record too, so the model knows of it on the next turn.
func (a *Agent) finish(userMessage, text, thinking string) Reply {
	reply := Reply{Text: text, Confirm: a.confirmation(), Changed: a.api.takeWrote(), Links: a.links, Proposals: a.proposals}
	a.links, a.proposals = nil, nil
	said := text
	if a.pending != nil {
		said = strings.TrimSpace(said + "\n\n" + waitingMark + "\n" + a.pending.summary() + "]")
	}
	a.record("assistant", agent.Message{Content: said, ReasoningContent: thinking})
	a.memory.Add("user", userMessage)
	a.memory.Add("assistant", said)
	return reply
}

// Tidy does what can wait until a reply is out.
func (a *Agent) Tidy() { a.memory.Tidy(a.conversation()) }

// ResetMemory clears conversation history and any pending action and begins a new
// conversation (called on /start).
func (a *Agent) ResetMemory() {
	a.pending = nil
	a.memory.ResetFull()
	if a.history != nil {
		a.conversationID.Store(a.history.Start(a.key))
	}
}
