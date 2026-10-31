package agent

import (
	"fmt"
	"net/url"
	"strings"
	"vergex/internal/auth"
	"vergex/internal/llm"
	"vergex/internal/logger"
	"vergex/internal/telegram/session"
)

const maxIterations = 10

// Agent is a stateful AI agent for one Telegram chat. The LLM sees only the
// task-level skills in skills.go; high-risk steps wait in pending until the
// user confirms them in their next message.
type Agent struct {
	api          *apiClient
	getLLM       func() llm.AIClient
	memory       *session.Memory
	systemPrompt string
	userID       string
	pending      *pendingAction
}

// New creates an Agent for one chat session.
func New(apiPort int, botToken, userID string, getLLM func() llm.AIClient, systemPrompt string) *Agent {
	return &Agent{
		api:          newAPIClient(apiPort, botToken),
		getLLM:       getLLM,
		memory:       session.NewMemory(getLLM()),
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
// conversation, so the LLM can refer to existing resources by name.
func (a *Agent) buildAccountContext() string {
	var sb strings.Builder
	sb.WriteString("[Current Account State]\n\n")

	sb.WriteString("## AI Models\n")
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
			sb.WriteString(fmt.Sprintf("  • %s  id=%s  type=%s  [%s]\n", e.label(), e.ID, e.ExchangeType, status(e.Enabled, "enabled", "disabled")))
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
// confirmation executes it without consulting the LLM; any other message
// cancels it and is then handled normally. Otherwise the LLM loops over skill
// calls until it returns a plain-text reply. On the first message of a
// conversation the account state is injected.
// onChunk is optional; when set it is called once with the complete final reply text.
func (a *Agent) Run(userMessage string, onChunk func(string)) string {
	zh := prefersChinese(userMessage)
	notice := ""
	if p := a.takePending(); p != nil {
		switch {
		case isConfirmation(userMessage) && p.expired():
			notice = status(zh, "⌛ 确认已超时，操作未执行："+p.summary, "⌛ Confirmation expired, nothing was executed: "+p.summary)
			return a.finish(userMessage, notice, onChunk)
		case isConfirmation(userMessage):
			logger.Infof("Agent: user confirmed pending action: %s", p.summary)
			return a.finish(userMessage, runPending(p, zh), onChunk)
		default:
			notice = status(zh, "已取消待确认操作："+p.summary, "Cancelled the pending action: "+p.summary)
		}
	}

	client := a.getLLM()
	if client == nil {
		return "AI assistant unavailable. Please configure an AI model in the Web UI."
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

	turnMsgs := []llm.Message{llm.NewUserMessage(firstUserContent)}
	builder := func() *llm.RequestBuilder {
		b := llm.NewRequestBuilder().WithSystemPrompt(a.systemPrompt).AddConversationHistory(turnMsgs)
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

		resp, err := client.CallWithRequestFull(req)
		if err != nil {
			logger.Errorf("Agent: LLM call failed (iteration %d): %v", i+1, err)
			return "AI assistant temporarily unavailable. Please try again."
		}

		// No tool calls → LLM returned a final text reply.
		if len(resp.ToolCalls) == 0 {
			return a.finish(userMessage, joinNotice(notice, strings.TrimSpace(resp.Content)), onChunk)
		}

		// Tool call iteration — show thinking indicator.
		if onChunk != nil {
			onChunk("⏳")
		}

		turnMsgs = append(turnMsgs, llm.Message{Role: "assistant", ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			logger.Infof("Agent: iter=%d skill=%s id=%s", i+1, tc.Function.Name, tc.ID)
			turnMsgs = append(turnMsgs, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    a.runSkill(tc.Function.Name, tc.Function.Arguments),
			})
		}
	}

	// Safety: max iterations reached.
	logger.Warnf("Agent: max iterations (%d) reached for message: %q", maxIterations, userMessage)
	return a.finish(userMessage, joinNotice(notice, "Operation completed. Please check your account for the latest status."), onChunk)
}

func joinNotice(notice, reply string) string {
	if notice == "" {
		return reply
	}
	return notice + "\n\n" + reply
}

// finish records the exchange in memory and delivers the reply.
func (a *Agent) finish(userMessage, reply string, onChunk func(string)) string {
	if onChunk != nil {
		onChunk(reply)
	}
	a.memory.Add("user", userMessage)
	a.memory.Add("assistant", reply)
	return reply
}

// ResetMemory clears conversation history and any pending action (called on /start).
func (a *Agent) ResetMemory() {
	a.pending = nil
	a.memory.ResetFull()
}
