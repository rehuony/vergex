package agent

import (
	"context"
	"errors"
	"time"
	"vergex/internal/database"

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

// ChatFeatures are the features whose conversations are the user's own chats with a model,
// listed and deleted as conversations. A trader's are the record of its cycles, read with its
// decisions. A new feature people chat in joins this list, and is listed and deleted with
// the rest from then on.
var ChatFeatures = []string{FeatureFutures, FeatureAssistant, FeatureTelegram}

// IsChat says whether a feature is one people chat in.
func IsChat(feature string) bool {
	for _, chat := range ChatFeatures {
		if chat == feature {
			return true
		}
	}
	return false
}

// Conversation is one exchange with a model: a trader cycle, a Futures session, a Telegram
// chat, a conversation with the assistant in the web app.
type Conversation struct {
	ID           string `gorm:"primaryKey"`
	UserID       string `gorm:"column:user_id"`
	Feature      string `gorm:"column:feature"`
	AgentModelID string `gorm:"column:agent_model_id;serializer:nulltext"`
	Title        string `gorm:"column:title"`
	// Context is what the feature keeps about it, such as a Futures session's account
	// and symbol, or a Telegram chat's id.
	Context   map[string]string `gorm:"column:context;serializer:json"`
	CreatedAt time.Time         `gorm:"autoCreateTime"`
	UpdatedAt time.Time         `gorm:"autoUpdateTime"`
}

func (Conversation) TableName() string { return "agent.conversations" }

// ConversationMessage is one thing said in a conversation. ToolCalls holds the calls the
// model asked for, as JSON.
type ConversationMessage struct {
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

func (ConversationMessage) TableName() string { return "agent.messages" }

// AgentConversationStore keeps conversations with models.
type ConversationStore struct {
	db *gorm.DB
}

// NewAgentConversationStore creates a new AgentConversationStore
func NewConversationStore(db *gorm.DB) *ConversationStore {
	return &ConversationStore{db: db}
}

// WithContext returns the store with its queries bound to ctx, for a caller that must
// not wait on the database for long.
func (s *ConversationStore) WithContext(ctx context.Context) *ConversationStore {
	return &ConversationStore{db: s.db.WithContext(ctx)}
}

// Open creates a conversation, or finds it when its ID already exists. An ID that
// belongs to another user or feature is refused.
func (s *ConversationStore) Open(conversation *Conversation) error {
	if uuid.Validate(conversation.ID) != nil {
		return errors.New("invalid conversation ID")
	}
	if conversation.Context == nil {
		conversation.Context = map[string]string{}
	}
	var existing Conversation
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
func (s *ConversationStore) Append(conversationID string, messages ...ConversationMessage) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var conversation Conversation
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
			message.Content = database.StorableText(message.Content)
			message.Thinking = database.StorableText(message.Thinking)
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE agent.conversations SET updated_at = now() WHERE id = ?`, conversationID).Error
	})
}

// List returns a user's latest conversations in a feature, without their messages.
func (s *ConversationStore) List(userID, feature string, limit int) ([]Conversation, error) {
	var conversations []Conversation
	err := s.db.Where("user_id = ? AND feature = ?", userID, feature).Order("updated_at DESC").Limit(limit).Find(&conversations).Error
	return conversations, err
}

// ListChats returns a user's latest conversations in the features people chat in, or in one
// of them, without their messages.
func (s *ConversationStore) ListChats(userID, feature string, limit int) ([]Conversation, error) {
	features := ChatFeatures
	if feature != "" {
		features = []string{feature}
	}
	query := s.db.Where("user_id = ? AND feature IN ?", userID, features).Order("updated_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var conversations []Conversation
	err := query.Find(&conversations).Error
	return conversations, err
}

// FindChats returns those of some conversations that are a user's chats, without their
// messages. Any other ID is left out.
func (s *ConversationStore) FindChats(userID string, ids []string) ([]Conversation, error) {
	valid := validIDs(ids)
	if len(valid) == 0 {
		return nil, nil
	}
	var conversations []Conversation
	err := s.db.Where("user_id = ? AND id IN ? AND feature IN ?", userID, valid, ChatFeatures).Order("updated_at DESC").Find(&conversations).Error
	return conversations, err
}

func validIDs(ids []string) []string {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if uuid.Validate(id) == nil {
			valid = append(valid, id)
		}
	}
	return valid
}

// ListByContext returns a user's latest conversations in a feature whose context holds a
// value, such as those on one account, without their messages. A limit of 0 lists them all.
func (s *ConversationStore) ListByContext(userID, feature, key, value string, limit int) ([]Conversation, error) {
	query := s.db.Where("user_id = ? AND feature = ? AND context ->> ? = ?", userID, feature, key, value).Order("updated_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var conversations []Conversation
	err := query.Find(&conversations).Error
	return conversations, err
}

// LatestByContext returns the conversation last active in a feature whose context holds a
// value, such as a Telegram chat's, or gorm.ErrRecordNotFound.
func (s *ConversationStore) LatestByContext(userID, feature, key, value string) (*Conversation, error) {
	rows, err := s.ListByContext(userID, feature, key, value, 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &rows[0], nil
}

// Get returns one of a user's conversations.
func (s *ConversationStore) Get(userID, id string) (*Conversation, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var conversation Conversation
	if err := s.db.Where("user_id = ? AND id = ?", userID, id).First(&conversation).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

// Rename changes a conversation's name. A new name is not activity, so the conversation
// keeps its place in the list.
func (s *ConversationStore) Rename(conversation *Conversation, title string) error {
	return s.db.Model(conversation).UpdateColumn("title", title).Error
}

// Delete removes some of a user's chats, and says how many there were. What belongs to one
// goes with it in the database: what was said, its model calls and its entry calculators.
// A record that outlives it, such as a plan that ran, only loses the link, so a feature that
// keeps more of its own removes that first, in the same transaction. A trader's
// conversations are not chats and stay with its cycles.
func (s *ConversationStore) Delete(userID string, ids []string) (int64, error) {
	valid := validIDs(ids)
	if len(valid) == 0 {
		return 0, nil
	}
	result := s.db.Where("user_id = ? AND id IN ? AND feature IN ?", userID, valid, ChatFeatures).Delete(&Conversation{})
	return result.RowsAffected, result.Error
}

// Exists says whether a conversation is still there, for a chat that holds one in memory
// and must not go on with one deleted from elsewhere.
func (s *ConversationStore) Exists(id string) (bool, error) {
	var count int64
	err := s.db.Model(&Conversation{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

// MessagesBefore says how many messages are older than a time, and how much text they hold.
func (s *ConversationStore) MessagesBefore(cutoff time.Time) (count, textBytes int64, err error) {
	var row struct {
		Count int64 `gorm:"column:count"`
		Bytes int64 `gorm:"column:bytes"`
	}
	err = s.db.Model(&ConversationMessage{}).
		Select("COUNT(*) AS count, COALESCE(SUM(octet_length(content) + COALESCE(octet_length(thinking), 0) + COALESCE(octet_length(tool_calls::text), 0)), 0) AS bytes").
		Where("created_at < ?", cutoff).Scan(&row).Error
	return row.Count, row.Bytes, err
}

// DeleteMessagesBefore removes messages older than a time, a batch at a time so nothing
// writing to the table waits long. The conversations stay, listed with nothing in them.
func (s *ConversationStore) DeleteMessagesBefore(cutoff time.Time, batch int) (int64, error) {
	var deleted int64
	for {
		result := s.db.Exec("DELETE FROM agent.messages WHERE id IN (SELECT id FROM agent.messages WHERE created_at < ? ORDER BY id LIMIT ?)", cutoff, batch)
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if result.RowsAffected < int64(batch) {
			return deleted, nil
		}
	}
}

// MessageStorage says how much disk the messages take, their indexes included.
func (s *ConversationStore) MessageStorage() (int64, error) {
	var size int64
	err := s.db.Raw("SELECT pg_total_relation_size('agent.messages')").Scan(&size).Error
	return size, err
}

// Messages returns everything in a conversation, in order.
func (s *ConversationStore) Messages(conversationID string) ([]ConversationMessage, error) {
	var messages []ConversationMessage
	err := s.db.Where("conversation_id = ?", conversationID).Order("seq").Find(&messages).Error
	return messages, err
}

// Transcript returns what the user and the model said to each other, in order: no
// context, tool traffic or steps on the way to an answer. A positive limit keeps the
// latest messages.
func (s *ConversationStore) Transcript(conversationID string, limit int) ([]ConversationMessage, error) {
	var messages []ConversationMessage
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
