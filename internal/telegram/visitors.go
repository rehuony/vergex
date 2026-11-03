package telegram

import (
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

// visitorTextRunes is how much of someone's last message is kept: enough to see what they were
// after.
const visitorTextRunes = 200

// maxVisitors is how many are kept, newest first, so someone trying many accounts cannot grow
// the table for long.
var maxVisitors = 500

// Visitor is someone who tried to use the bot and was refused.
type Visitor struct {
	ChatID    int64  `gorm:"column:chat_id;primaryKey" json:"chat_id"`
	SenderID  int64  `gorm:"column:sender_id;primaryKey" json:"sender_id"`
	ChatType  string `gorm:"column:chat_type" json:"chat_type"`
	ChatTitle string `gorm:"column:chat_title" json:"chat_title,omitempty"`
	Username  string `gorm:"column:username" json:"username,omitempty"`
	Name      string `gorm:"column:name" json:"name,omitempty"`
	Attempts  int64  `gorm:"column:attempts" json:"attempts"`
	LastText  string `gorm:"column:last_text" json:"last_text,omitempty"`
	FirstAt   int64  `gorm:"column:first_at;serializer:unixms" json:"first_at"`
	LastAt    int64  `gorm:"column:last_at;serializer:unixms" json:"last_at"`
}

func (Visitor) TableName() string { return "telegram.visitors" }

// newVisitor reads who sent a message and what they said.
func newVisitor(m *tgbotapi.Message) Visitor {
	visitor := Visitor{ChatID: m.Chat.ID, ChatType: m.Chat.Type, ChatTitle: m.Chat.Title, LastText: visitorText(m)}
	if m.From != nil {
		visitor.SenderID, visitor.Username = m.From.ID, m.From.UserName
		visitor.Name = strings.TrimSpace(m.From.FirstName + " " + m.From.LastName)
	}
	return visitor
}

// visitorText is what is kept of a message. Whatever follows /start is a binding code, live or
// not, as is a code sent on its own, and there is no reason to keep one.
func visitorText(m *tgbotapi.Message) string {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	if command, _ := parseCommand(text); command == "start" {
		return "/start"
	}
	if bindCodeShape.MatchString(text) {
		return ""
	}
	if runes := []rune(text); len(runes) > visitorTextRunes {
		return string(runes[:visitorTextRunes])
	}
	return text
}

// VisitorStore keeps who was refused, for Settings to show.
type VisitorStore struct {
	db *gorm.DB
}

// NewVisitorStore opens it on a migrated connection.
func NewVisitorStore(db *gorm.DB) *VisitorStore {
	return &VisitorStore{db: db}
}

// Record adds an attempt, and lets go of whoever has been quiet longest once there are too many.
func (s *VisitorStore) Record(visitor Visitor, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Exec(`
			INSERT INTO telegram.visitors (chat_id, sender_id, chat_type, chat_title, username, name, last_text, first_at, last_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (chat_id, sender_id) DO UPDATE SET
				chat_type = EXCLUDED.chat_type,
				chat_title = EXCLUDED.chat_title,
				username = EXCLUDED.username,
				name = EXCLUDED.name,
				attempts = telegram.visitors.attempts + 1,
				last_text = EXCLUDED.last_text,
				last_at = EXCLUDED.last_at`,
			visitor.ChatID, visitor.SenderID, visitor.ChatType, visitor.ChatTitle, visitor.Username, visitor.Name, visitor.LastText, now.UTC(), now.UTC(),
		).Error
		if err != nil {
			return err
		}
		return tx.Exec(`
			DELETE FROM telegram.visitors WHERE (chat_id, sender_id) IN (
				SELECT chat_id, sender_id FROM telegram.visitors ORDER BY last_at DESC OFFSET ?
			)`, maxVisitors).Error
	})
}

// List returns who was refused, the latest first.
func (s *VisitorStore) List() ([]Visitor, error) {
	visitors := []Visitor{}
	err := s.db.Order("last_at DESC").Limit(maxVisitors).Find(&visitors).Error
	return visitors, err
}

// Forget removes a chat's attempts: whoever binds it is no longer a stranger.
func (s *VisitorStore) Forget(chatID int64) error {
	return s.db.Where("chat_id = ?", chatID).Delete(&Visitor{}).Error
}
