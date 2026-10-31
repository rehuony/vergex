package store

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// TelegramConfig stores the Telegram bot binding (single row, always ID=1)
type TelegramConfig struct {
	ID       uint      `gorm:"primaryKey"`
	BotToken string    `gorm:"column:bot_token"`
	ChatID   int64     `gorm:"column:chat_id"`
	Username string    `gorm:"column:username"` // @username for display
	BoundAt  time.Time `gorm:"column:bound_at"`
	ModelID  string    `gorm:"column:model_id;default:''"` // AI model used for Telegram replies
	Language string    `gorm:"column:language;default:''"` // "zh" or "en"; empty = not chosen yet
	// MutedAlerts lists the alert kinds turned off, comma separated. Everything
	// not listed is on, so a new kind starts on.
	MutedAlerts string `gorm:"column:muted_alerts;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AlertMuted reports whether the user turned off this kind of alert.
func (tc TelegramConfig) AlertMuted(kind string) bool {
	for _, muted := range strings.Split(tc.MutedAlerts, ",") {
		if muted == kind {
			return true
		}
	}
	return false
}

// TelegramAlertMark remembers what the alert watcher has already told the user:
// a time for a stream of events, or that a standing problem was reported. It is
// kept in the database so a restart repeats nothing.
type TelegramAlertMark struct {
	Key string `gorm:"column:key;primaryKey"`
	At  int64  `gorm:"column:at;not null"` // Unix milliseconds
}

func (TelegramAlertMark) TableName() string { return "telegram_alert_marks" }

// String returns a safe string representation of TelegramConfig with the token masked.
func (tc TelegramConfig) String() string {
	token := "***"
	if tc.BotToken == "" {
		token = "<not set>"
	}
	return fmt.Sprintf("TelegramConfig{ID:%d, ChatID:%d, Username:%q, BotToken:%s, BoundAt:%v}",
		tc.ID, tc.ChatID, tc.Username, token, tc.BoundAt)
}

// TelegramConfigStore defines the interface for Telegram bot binding operations
type TelegramConfigStore interface {
	Get() (*TelegramConfig, error)                 // Get current config (may not exist)
	SaveToken(botToken string) error               // Save bot token only (Web UI sets this)
	Save(botToken, modelID string) error           // Save bot token + selected AI model
	BindUser(chatID int64, username string) error  // Called on first /start
	IsBound() (bool, error)                        // Check if any user is bound
	GetBoundChatID() (int64, error)                // Get bound chat ID (0 if not bound)
	Unbind() error                                 // Remove binding
	SetLanguage(lang string) error                 // Set UI language ("en" or "zh")
	GetLanguage() string                           // Get UI language; returns "en" if not set
	SetMutedAlerts(kinds []string) error           // Replace the alert kinds turned off
	AlertMark(key string) (int64, bool)            // Read an alert mark
	SetAlertMark(key string, at int64) error       // Create or move an alert mark
	DeleteAlertMark(key string) error              // Forget an alert mark
	AlertMarkKeys(prefix string) ([]string, error) // Marks whose key starts with prefix
}

type telegramConfigStore struct {
	db *gorm.DB
	mu sync.RWMutex
}

// NewTelegramConfigStore creates a new TelegramConfigStore
func NewTelegramConfigStore(db *gorm.DB) TelegramConfigStore {
	return &telegramConfigStore{db: db}
}

func (s *telegramConfigStore) initTables() error {
	return s.db.AutoMigrate(&TelegramConfig{}, &TelegramAlertMark{})
}

func (s *telegramConfigStore) Get() (*TelegramConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var cfg TelegramConfig
	if err := s.db.First(&cfg, 1).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *telegramConfigStore) SaveToken(botToken string) error {
	return s.Save(botToken, "")
}

func (s *telegramConfigStore) Save(botToken, modelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cfg TelegramConfig
	result := s.db.First(&cfg, 1)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	cfg.ID = 1
	cfg.BotToken = botToken
	cfg.ModelID = modelID
	return s.db.Save(&cfg).Error
}

func (s *telegramConfigStore) BindUser(chatID int64, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cfg TelegramConfig
	result := s.db.First(&cfg, 1)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	cfg.ID = 1
	cfg.ChatID = chatID
	cfg.Username = username
	cfg.BoundAt = time.Now()
	return s.db.Save(&cfg).Error
}

func (s *telegramConfigStore) IsBound() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var cfg TelegramConfig
	if err := s.db.First(&cfg, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return cfg.ChatID != 0, nil
}

func (s *telegramConfigStore) GetBoundChatID() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var cfg TelegramConfig
	if err := s.db.First(&cfg, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return cfg.ChatID, nil
}

func (s *telegramConfigStore) Unbind() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Model(&TelegramConfig{}).Where("id = 1").Updates(map[string]interface{}{
		"chat_id":  0,
		"username": "",
	}).Error
}

func (s *telegramConfigStore) SetLanguage(lang string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cfg TelegramConfig
	result := s.db.First(&cfg, 1)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	cfg.ID = 1
	cfg.Language = lang
	return s.db.Save(&cfg).Error
}

func (s *telegramConfigStore) GetLanguage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var cfg TelegramConfig
	if err := s.db.First(&cfg, 1).Error; err != nil {
		return "en" // default: English
	}
	if cfg.Language == "" {
		return "en"
	}
	return cfg.Language
}

func (s *telegramConfigStore) SetMutedAlerts(kinds []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cfg TelegramConfig
	result := s.db.First(&cfg, 1)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	cfg.ID = 1
	cfg.MutedAlerts = strings.Join(kinds, ",")
	return s.db.Save(&cfg).Error
}

func (s *telegramConfigStore) AlertMark(key string) (int64, bool) {
	var mark TelegramAlertMark
	if err := s.db.Where(`"key" = ?`, key).First(&mark).Error; err != nil {
		return 0, false
	}
	return mark.At, true
}

func (s *telegramConfigStore) SetAlertMark(key string, at int64) error {
	return s.db.Save(&TelegramAlertMark{Key: key, At: at}).Error
}

func (s *telegramConfigStore) DeleteAlertMark(key string) error {
	return s.db.Where(`"key" = ?`, key).Delete(&TelegramAlertMark{}).Error
}

func (s *telegramConfigStore) AlertMarkKeys(prefix string) ([]string, error) {
	var keys []string
	err := s.db.Model(&TelegramAlertMark{}).Where(`"key" LIKE ?`, prefix+"%").Pluck(`"key"`, &keys).Error
	return keys, err
}
