package agent

import (
	"sync"
	"time"
	"vergex/internal/agent"
	"vergex/internal/logger"
)

// Manager holds one Agent per Telegram chat ID.
// Messages for the same chat are serialized (OpenClaw Lane Queue pattern).
type Manager struct {
	mu           sync.Mutex
	agents       map[int64]*Agent
	lanes        map[int64]chan struct{}
	apiPort      int
	botToken     string
	userID       string
	getClient    func() agent.Client
	systemPrompt string
	history      History
}

// NewManager creates a Manager.
// userEmail is the registered email shown to the user when they ask "who am I".
// userID is the internal DB UUID used for API authentication.
// history keeps chats across restarts; nil keeps them in memory only.
func NewManager(apiPort int, botToken, userEmail, userID string, getClient func() agent.Client, history History) *Manager {
	return &Manager{
		agents:       make(map[int64]*Agent),
		lanes:        make(map[int64]chan struct{}),
		apiPort:      apiPort,
		botToken:     botToken,
		userID:       userID,
		getClient:    getClient,
		systemPrompt: BuildAgentPrompt(userEmail),
		history:      history,
	}
}

// Run processes a message for the given chat ID.
// If the same chat is already processing a message, this call blocks until it completes
// or the lane wait times out (60 s), whichever comes first.
// onChunk is optional — when set, model reply chunks are forwarded progressively (SSE streaming).
func (m *Manager) Run(chatID int64, userMessage string, onChunk func(string)) string {
	a, lane := m.getOrCreate(chatID)
	select {
	case lane <- struct{}{}:
	case <-time.After(60 * time.Second):
		logger.Warnf("Agent: lane wait timeout for chat %d — previous message still processing", chatID)
		return "Previous message is still being processed. Please wait a moment and try again."
	}
	defer func() { <-lane }()
	return a.Run(userMessage, onChunk)
}

// Reset begins a new conversation for the given chat (called on /start). A chat with
// no agent yet gets one, so a restart cannot bring the old conversation back.
func (m *Manager) Reset(chatID int64) {
	a, _ := m.getOrCreate(chatID)
	a.ResetMemory()
}

func (m *Manager) getOrCreate(chatID int64) (*Agent, chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[chatID]
	if !ok {
		a = New(m.apiPort, m.botToken, m.userID, m.getClient, m.systemPrompt)
		a.resume(m.history, chatID)
		m.agents[chatID] = a
	}
	lane, ok := m.lanes[chatID]
	if !ok {
		lane = make(chan struct{}, 1) // binary semaphore: one message at a time per chat
		m.lanes[chatID] = lane
	}
	return a, lane
}
