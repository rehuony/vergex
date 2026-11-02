package agent

import (
	"context"
	"sync"
	"vergex/internal/agent"
)

// Manager holds one Agent per Telegram chat ID. The caller runs a chat's turns one at a time.
type Manager struct {
	mu           sync.Mutex
	agents       map[int64]*Agent
	apiPort      int
	token        func() (string, error)
	userID       string
	getClient    func() agent.Client
	systemPrompt string
	history      History
	language     func() string
}

// NewManager creates a Manager.
// userEmail is the registered email shown to the user when they ask "who am I".
// userID is the internal DB UUID used for API authentication.
// history keeps chats across restarts; nil keeps them in memory only.
// language is the one chosen with /lang, empty until one is.
func NewManager(apiPort int, token func() (string, error), userEmail, userID string, getClient func() agent.Client, history History, language func() string) *Manager {
	return &Manager{
		agents:       make(map[int64]*Agent),
		apiPort:      apiPort,
		token:        token,
		userID:       userID,
		getClient:    getClient,
		systemPrompt: BuildAgentPrompt(userEmail),
		history:      history,
		language:     language,
	}
}

// Run processes a message for the given chat ID. progress hears how it is going, and may
// be nil.
func (m *Manager) Run(ctx context.Context, chatID int64, userMessage string, progress *Progress) Reply {
	return m.getOrCreate(chatID).Run(ctx, userMessage, progress)
}

// Answer resolves what is waiting for confirmation from its button. It reports false when
// id is no longer what is waiting.
func (m *Manager) Answer(chatID int64, id string, confirmed bool) (Reply, bool) {
	return m.getOrCreate(chatID).Answer(id, confirmed)
}

// CancelPending drops what is waiting for confirmation. It reports false when nothing is.
func (m *Manager) CancelPending(chatID int64) (Reply, bool) {
	return m.getOrCreate(chatID).CancelPending()
}

// Status, Positions and Traders answer without the model. See commands.go.
func (m *Manager) Status(chatID int64) string    { return m.getOrCreate(chatID).StatusText() }
func (m *Manager) Positions(chatID int64) string { return m.getOrCreate(chatID).PositionsText() }
func (m *Manager) Traders(chatID int64) string   { return m.getOrCreate(chatID).TradersText() }

// Tidy does what can wait until a reply is out.
func (m *Manager) Tidy(chatID int64) { m.getOrCreate(chatID).Tidy() }

// Reset begins a new conversation for the given chat (called on /start). A chat with
// no agent yet gets one, so a restart cannot bring the old conversation back.
func (m *Manager) Reset(chatID int64) {
	m.getOrCreate(chatID).ResetMemory()
}

func (m *Manager) getOrCreate(chatID int64) *Agent {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[chatID]
	if !ok {
		a = New(m.apiPort, m.token, m.userID, m.getClient, m.systemPrompt, m.language)
		a.resume(m.history, chatID)
		m.agents[chatID] = a
	}
	return a
}
