package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The roles of a message. RoleContext is data sent with a turn that the user did
// not type, kept apart from their words.
const (
	RoleSystem    = "system"
	RoleContext   = "context"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// AgentConversation is one exchange with a model: a Frontier session, a Telegram chat.
type AgentConversation struct {
	ID           string            `gorm:"primaryKey"`
	UserID       string            `gorm:"column:user_id"`
	Feature      string            `gorm:"column:feature"`
	SourceID     string            `gorm:"column:source_id;serializer:nulltext"`
	AgentModelID string            `gorm:"column:agent_model_id;serializer:nulltext"`
	Title        string            `gorm:"column:title;serializer:nulltext"`
	Context      map[string]string `gorm:"column:context;serializer:json"`
	CreatedAt    time.Time         `gorm:"autoCreateTime"`
	UpdatedAt    time.Time         `gorm:"autoUpdateTime"`
}

func (AgentConversation) TableName() string { return "agent.conversations" }

// AgentMessage is one thing said in a conversation. ToolCalls holds the calls the
// model asked for, as JSON.
type AgentMessage struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"-"`
	ConversationID string    `gorm:"column:conversation_id" json:"-"`
	Seq            int       `gorm:"column:seq" json:"-"`
	Role           string    `gorm:"column:role" json:"role"`
	Content        string    `gorm:"column:content" json:"content"`
	Thinking       string    `gorm:"column:thinking;serializer:nulltext" json:"-"`
	ToolCalls      string    `gorm:"column:tool_calls;serializer:nulltext" json:"-"`
	ToolCallID     string    `gorm:"column:tool_call_id;serializer:nulltext" json:"-"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"-"`
}

func (AgentMessage) TableName() string { return "agent.messages" }

// AgentConversationStore keeps conversations with models.
type AgentConversationStore struct {
	db *gorm.DB
}

// NewAgentConversationStore creates a new AgentConversationStore
func NewAgentConversationStore(db *gorm.DB) *AgentConversationStore {
	return &AgentConversationStore{db: db}
}

// WithContext returns the store with its queries bound to ctx, for a caller that must
// not wait on the database for long.
func (s *AgentConversationStore) WithContext(ctx context.Context) *AgentConversationStore {
	return &AgentConversationStore{db: s.db.WithContext(ctx)}
}

// Open creates a conversation, or finds it when its ID already exists. An ID that
// belongs to another user or feature is refused.
func (s *AgentConversationStore) Open(conversation *AgentConversation) error {
	if uuid.Validate(conversation.ID) != nil {
		return errors.New("invalid conversation ID")
	}
	if conversation.Context == nil {
		conversation.Context = map[string]string{}
	}
	var existing AgentConversation
	err := s.db.Where("id = ?", conversation.ID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(conversation).Error
	}
	if err != nil {
		return err
	}
	if existing.UserID != conversation.UserID || existing.Feature != conversation.Feature {
		return errors.New("conversation belongs to something else")
	}
	return nil
}

// Append adds messages after those already in the conversation.
func (s *AgentConversationStore) Append(conversationID string, messages ...AgentMessage) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var conversation AgentConversation
		// The lock keeps the numbering in order when two writers meet, and lets a row
		// that points at this conversation be saved meanwhile.
		if err := tx.Raw(`SELECT id FROM agent.conversations WHERE id = ? FOR NO KEY UPDATE`, conversationID).Scan(&conversation).Error; err != nil {
			return err
		}
		if conversation.ID == "" {
			return gorm.ErrRecordNotFound
		}
		var last int
		if err := tx.Raw(`SELECT COALESCE(MAX(seq), 0) FROM agent.messages WHERE conversation_id = ?`, conversationID).Scan(&last).Error; err != nil {
			return err
		}
		for i := range messages {
			message := messages[i]
			message.ConversationID = conversationID
			message.Seq = last + i + 1
			message.Content = storableText(message.Content)
			message.Thinking = storableText(message.Thinking)
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE agent.conversations SET updated_at = now() WHERE id = ?`, conversationID).Error
	})
}

// List returns a user's latest conversations in a feature, without their messages.
func (s *AgentConversationStore) List(userID, feature string, limit int) ([]AgentConversation, error) {
	var conversations []AgentConversation
	err := s.db.Where("user_id = ? AND feature = ?", userID, feature).Order("updated_at DESC").Limit(limit).Find(&conversations).Error
	return conversations, err
}

// Get returns one of a user's conversations.
func (s *AgentConversationStore) Get(userID, id string) (*AgentConversation, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var conversation AgentConversation
	if err := s.db.Where("user_id = ? AND id = ?", userID, id).First(&conversation).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

// Latest returns the conversation last active for a source within a feature.
func (s *AgentConversationStore) Latest(userID, feature, sourceID string) (*AgentConversation, error) {
	var conversation AgentConversation
	err := s.db.Where("user_id = ? AND feature = ? AND source_id = ?", userID, feature, sourceID).
		Order("updated_at DESC").First(&conversation).Error
	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

// Messages returns everything in a conversation, in order.
func (s *AgentConversationStore) Messages(conversationID string) ([]AgentMessage, error) {
	var messages []AgentMessage
	err := s.db.Where("conversation_id = ?", conversationID).Order("seq").Find(&messages).Error
	return messages, err
}

// Transcript returns what the user and the model said to each other, in order: no
// context, tool traffic or steps on the way to an answer. A positive limit keeps the
// latest messages.
func (s *AgentConversationStore) Transcript(conversationID string, limit int) ([]AgentMessage, error) {
	var messages []AgentMessage
	query := s.db.Where("conversation_id = ? AND role IN (?, ?) AND tool_calls IS NULL AND content <> ''", conversationID, RoleUser, RoleAssistant).
		Order("seq DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}
