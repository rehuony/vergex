package agent

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"vergex/internal/agent"
	"vergex/internal/logger"
	"vergex/internal/security/auth"
)

const maxIterations = 10

// Agent is a stateful agent for one Telegram chat. The model sees only the
// task-level skills in skills.go; high-risk steps wait in pending until the
// user confirms them in their next message.
type Agent struct {
	api          *apiClient
	getClient    func() agent.Client
	memory       *Memory
	systemPrompt string
	userID       string
	pending      *pendingAction

	history        History
	chatID         int64
	conversationID atomic.Value // string
}

// New creates an Agent for one chat session.
func New(apiPort int, botToken, userID string, getClient func() agent.Client, systemPrompt string) *Agent {
	return &Agent{
		api:          newAPIClient(apiPort, botToken),
		getClient:    getClient,
		memory:       NewMemory(getClient()),
		systemPrompt: systemPrompt,
		userID:       userID,
	}
}

// GenerateBotToken creates a long-lived JWT for the bot's internal API calls.
// userID must match the actual registered user's ID so bot-made changes
// are visible in the frontend (shared user namespace).
func GenerateBotToken(userID string) (string, error) {
	return auth.GenerateJWT(userID, "bot@internal")
}

func status(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// buildAccountContext summarizes models, exchanges, strategies and traders
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

	sb.WriteString("\n## Exchanges\n")
	if exchanges, err := a.exchanges(); err != nil {
		sb.WriteString("  unavailable: " + err.Error() + "\n")
	} else {
		for _, e := range exchanges {
			sb.WriteString(fmt.Sprintf("  • %s  id=%s  type=%s\n", e.label(), e.ID, e.ExchangeType))
		}
	}

	sb.WriteString("\n## Strategies\n")
	if strategies, err := a.strategies(); err != nil {
		sb.WriteString("  unavailable: " + err.Error() + "\n")
	} else {
		for _, s := range strategies {
			sb.WriteString(fmt.Sprintf("  • %s  id=%s%s\n", s.Name, s.ID, status(s.IsActive, "  [active]", "")))
		}
	}

	sb.WriteString("\n## Traders\n")
	traders, err := a.traders()
	if err != nil {
		sb.WriteString("  unavailable: " + err.Error() + "\n")
	}
	for _, t := range traders {
		sb.WriteString(fmt.Sprintf("  • %s  id=%s  strategy=%s  [%s]\n", t.Name, t.TraderID, orDash(t.StrategyName), status(t.IsRunning, "RUNNING", "stopped")))
	}
	for _, t := range traders {
		if !t.IsRunning {
			continue
		}
		q := "?trader_id=" + url.QueryEscape(t.TraderID)
		for _, part := range []struct{ label, path string }{{"Account", "/api/account"}, {"Statistics", "/api/statistics"}} {
			raw, err := a.api.raw("GET", part.path+q, nil)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("\n%s [%s]: %s\n", part.label, t.Name, truncate(string(raw), 1500)))
		}
	}
	return sb.String()
}

// Run processes one user message.
//
// A pending high-risk action is resolved first, deterministically: an explicit
// confirmation executes it without consulting the model; any other message
// cancels it and is then handled normally. Otherwise the model loops over skill
// calls until it returns a plain-text reply. On the first message of a
// conversation the account state is injected.
// onChunk is optional; when set it is called once with the complete final reply text.
func (a *Agent) Run(userMessage string, onChunk func(string)) string {
	a.record("user", agent.Message{Content: userMessage})
	zh := prefersChinese(userMessage)
	notice := ""
	if p := a.takePending(); p != nil {
		switch {
		case isConfirmation(userMessage) && p.expired():
			notice = status(zh, "⌛ 确认已超时，操作未执行："+p.summary, "⌛ Confirmation expired, nothing was executed: "+p.summary)
			return a.finish(userMessage, notice, "", onChunk)
		case isConfirmation(userMessage):
			logger.Infof("Agent: user confirmed pending action: %s", p.summary)
			return a.finish(userMessage, runPending(p, zh), "", onChunk)
		default:
			notice = status(zh, "已取消待确认操作："+p.summary, "Cancelled the pending action: "+p.summary)
		}
	}

	client := a.getClient()
	if client == nil {
		return "Assistant unavailable. Please configure a model in the Web UI."
	}

	// Build initial user message: prepend account state on first turn, history on subsequent turns.
	histCtx := a.memory.BuildContext()
	var firstUserContent string
	if histCtx == "" {
		firstUserContent = a.buildAccountContext() + "\n[User Message]\n" + userMessage
	} else {
		firstUserContent = histCtx + "\n---\nUser: " + userMessage
	}
	if notice != "" {
		firstUserContent += "\n[System: the previously pending action was cancelled because the user did not confirm it.]"
	}

	a.record(roleContext, agent.Message{Content: firstUserContent})

	turnMsgs := []agent.Message{agent.NewUserMessage(firstUserContent)}
	builder := func() *agent.RequestBuilder {
		b := agent.NewRequestBuilder().WithSystemPrompt(a.systemPrompt).AddConversationHistory(turnMsgs)
		for _, t := range skillTools {
			b.AddTool(t)
		}
		return b.WithToolChoice("auto")
	}

	for i := 0; i < maxIterations; i++ {
		req, err := builder().Build()
		if err != nil {
			logger.Errorf("Agent: failed to build request: %v", err)
			break
		}
		req.Ctx = agent.WithConversation(context.Background(), a.conversation())

		resp, err := client.CallWithRequestFull(req)
		if err != nil {
			logger.Errorf("Agent: model call failed (iteration %d): %v", i+1, err)
			return "Assistant temporarily unavailable. Please try again."
		}

		// No tool calls → model returned a final text reply.
		if len(resp.ToolCalls) == 0 {
			return a.finish(userMessage, joinNotice(notice, strings.TrimSpace(resp.Content)), resp.ReasoningContent, onChunk)
		}

		// Tool call iteration — show thinking indicator.
		if onChunk != nil {
			onChunk("⏳")
		}

		a.record("assistant", agent.Message{Content: resp.Content, ReasoningContent: resp.ReasoningContent, ToolCalls: resp.ToolCalls})
		turnMsgs = append(turnMsgs, agent.Message{Role: "assistant", ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			logger.Infof("Agent: iter=%d skill=%s id=%s", i+1, tc.Function.Name, tc.ID)
			result := agent.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    a.runSkill(tc.Function.Name, tc.Function.Arguments),
			}
			a.record("tool", result)
			turnMsgs = append(turnMsgs, result)
		}
	}

	// Safety: max iterations reached.
	logger.Warnf("Agent: max iterations (%d) reached for message: %q", maxIterations, userMessage)
	return a.finish(userMessage, joinNotice(notice, "Operation completed. Please check your account for the latest status."), "", onChunk)
}

func joinNotice(notice, reply string) string {
	if notice == "" {
		return reply
	}
	return notice + "\n\n" + reply
}

// finish records the exchange and delivers the reply.
func (a *Agent) finish(userMessage, reply, thinking string, onChunk func(string)) string {
	if onChunk != nil {
		onChunk(reply)
	}
	a.record("assistant", agent.Message{Content: reply, ReasoningContent: thinking})
	a.memory.Add("user", userMessage)
	a.memory.Add("assistant", reply)
	return reply
}

// ResetMemory clears conversation history and any pending action and begins a new
// conversation (called on /start).
func (a *Agent) ResetMemory() {
	a.pending = nil
	a.memory.ResetFull()
	if a.history != nil {
		a.conversationID.Store(a.history.Start(a.chatID))
	}
}
