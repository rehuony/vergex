package assistant

import (
	"context"
	"sync"
	"vergex/internal/agent"
)

// Manager holds one Agent per chat, named by a key its surface chooses. The caller runs a
// chat's turns one at a time.
type Manager struct {
	mu           sync.Mutex
	agents       map[string]*Agent
	apiPort      int
	token        func() (string, error)
	userID       string
	getClient    func() agent.Client
	systemPrompt string
	history      History
	language     func() string
	tools        []agent.Tool
}

// NewManager creates a Manager.
// userEmail is the registered email shown to the user when they ask "who am I".
// userID is the internal DB UUID used for API authentication.
// history keeps chats across restarts; nil keeps them in memory only.
// language is the one chosen with /lang, empty until one is.
func NewManager(apiPort int, token func() (string, error), userEmail, userID string, getClient func() agent.Client, history History, language func() string, surface Surface) *Manager {
	return &Manager{
		agents:       make(map[string]*Agent),
		apiPort:      apiPort,
		token:        token,
		userID:       userID,
		getClient:    getClient,
		systemPrompt: BuildAgentPrompt(userEmail, surface),
		history:      history,
		language:     language,
		tools:        surface.Tools,
	}
}

// Run processes a message for the given chat. progress hears how it is going, and may be nil.
func (m *Manager) Run(ctx context.Context, key, userMessage string, progress *Progress) Reply {
	return m.getOrCreate(key).Run(ctx, userMessage, progress)
}

// Answer resolves what is waiting for confirmation from its button. It reports false when
// id is no longer what is waiting.
func (m *Manager) Answer(key, id string, confirmed bool) (Reply, bool) {
	return m.getOrCreate(key).Answer(id, confirmed)
}

// CancelPending drops what is waiting for confirmation. It reports false when nothing is.
func (m *Manager) CancelPending(key string) (Reply, bool) {
	return m.getOrCreate(key).CancelPending()
}

// Pending says what a chat is waiting for an answer to, if anything is. It starts no chat.
func (m *Manager) Pending(key string) *Confirmation {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.agents[key]
	if !ok || a.pending == nil || a.pending.expired() {
		return nil
	}
	return a.confirmation()
}

// Forget drops what is held in memory for a chat that no longer exists.
func (m *Manager) Forget(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, key)
}

// ForgetAll drops what is held in memory for every chat.
func (m *Manager) ForgetAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents = make(map[string]*Agent)
}

// Holdings and Traders read for the commands, without the model. See commands.go.
func (m *Manager) Holdings(key string) ([]Holding, error) { return m.getOrCreate(key).Holdings() }
func (m *Manager) Traders(key string) ([]TraderState, error) {
	return m.getOrCreate(key).TraderStates()
}

// Tidy does what can wait until a reply is out.
func (m *Manager) Tidy(key string) { m.getOrCreate(key).Tidy() }

// Reset begins a new conversation for the given chat (called on /start). A chat with
// no agent yet gets one, so a restart cannot bring the old conversation back.
func (m *Manager) Reset(key string) {
	m.getOrCreate(key).ResetMemory()
}

func (m *Manager) getOrCreate(key string) *Agent {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[key]
	if !ok {
		a = New(m.apiPort, m.token, m.userID, m.getClient, m.systemPrompt, m.language)
		a.tools = m.tools
		a.resume(m.history, key)
		m.agents[key] = a
	}
	return a
}
