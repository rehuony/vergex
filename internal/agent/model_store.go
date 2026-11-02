package agent

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"vergex/internal/logger"
	"vergex/internal/security/crypto"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentModelStore model storage
type ModelStore struct {
	db *gorm.DB
}

// AgentModel model configuration
type Model struct {
	ID              string                 `gorm:"primaryKey" json:"id"`
	UserID          string                 `gorm:"column:user_id" json:"user_id"`
	Name            string                 `gorm:"column:name" json:"name"`
	Provider        string                 `gorm:"column:provider" json:"provider"`
	Enabled         bool                   `gorm:"column:enabled" json:"enabled"`
	APIKey          crypto.EncryptedString `gorm:"column:api_key" json:"apiKey"`
	CustomAPIURL    string                 `gorm:"column:base_url" json:"customApiUrl"`
	CustomModelName string                 `gorm:"column:model_name" json:"customModelName"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

func (Model) TableName() string { return "agent.models" }

// NewAgentModelStore creates a new AgentModelStore
func NewModelStore(db *gorm.DB) *ModelStore {
	return &ModelStore{db: db}
}

// List retrieves user's model list
func (s *ModelStore) List(userID string) ([]*Model, error) {
	var models []*Model
	err := s.db.Where("user_id = ?", userID).Order("created_at, id").Find(&models).Error
	if err != nil {
		return nil, err
	}
	return models, nil
}

// ListEndpoints says where each model is reached. Its key stays unread and so is not
// decrypted.
func (s *ModelStore) ListEndpoints(userID string) ([]*Model, error) {
	var models []*Model
	err := s.db.Select("id", "provider", "base_url").Where("user_id = ?", userID).Order("created_at, id").Find(&models).Error
	if err != nil {
		return nil, err
	}
	return models, nil
}

// Get retrieves a single model
func (s *ModelStore) Get(userID, modelID string) (*Model, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID cannot be empty")
	}
	if uuid.Validate(modelID) != nil {
		return nil, gorm.ErrRecordNotFound
	}

	var model Model
	if err := s.db.Where("user_id = ? AND id = ?", userID, modelID).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// GetByID retrieves a model by ID only
func (s *ModelStore) GetByID(modelID string) (*Model, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID cannot be empty")
	}
	if uuid.Validate(modelID) != nil {
		return nil, gorm.ErrRecordNotFound
	}

	var model Model
	err := s.db.Where("id = ?", modelID).First(&model).Error
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// GetDefault retrieves the default enabled model
func (s *ModelStore) GetDefault(userID string) (*Model, error) {
	model, err := s.firstEnabledUsable(userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("please configure an available model in the system first")
	}
	return model, err
}

func (s *ModelStore) firstEnabledUsable(userID string) (*Model, error) {
	var models []Model
	err := s.db.Where("user_id = ? AND enabled = ? AND api_key != ''", userID, true).
		Order("updated_at DESC, id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	for i := range models {
		if hasUsableAPIKey(models[i]) {
			return &models[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func hasUsableAPIKey(model Model) bool {
	if strings.TrimSpace(string(model.APIKey)) != "" {
		return true
	}
	envKeyByProvider := map[string]string{
		"deepseek": "DEEPSEEK_API_KEY",
		"openai":   "OPENAI_API_KEY",
		"claude":   "ANTHROPIC_API_KEY",
		"gemini":   "GEMINI_API_KEY",
		"grok":     "XAI_API_KEY",
		"kimi":     "MOONSHOT_API_KEY",
		"minimax":  "MINIMAX_API_KEY",
		"qwen":     "DASHSCOPE_API_KEY",
	}
	envKey := envKeyByProvider[strings.ToLower(strings.TrimSpace(model.Provider))]
	return envKey != "" && strings.TrimSpace(os.Getenv(envKey)) != ""
}

// Find returns a user's model by its id or, until the model exists under one, by
// the provider name the settings form holds for it.
func (s *ModelStore) Find(userID, key string) (*Model, error) {
	if uuid.Validate(key) == nil {
		return s.Get(userID, key)
	}
	var model Model
	if err := s.db.Where("user_id = ? AND provider = ?", userID, key).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// Save stores a model's settings, under key as in Find, and creates the model named
// name the first time its provider is saved. A blank apiKey keeps the saved key.
func (s *ModelStore) Save(userID, key, name string, enabled bool, apiKey, customAPIURL, customModelName string) error {
	existing, err := s.Find(userID, key)
	if errors.Is(err, gorm.ErrRecordNotFound) && uuid.Validate(key) != nil {
		return s.db.Create(&Model{
			ID:              uuid.NewString(),
			UserID:          userID,
			Name:            name,
			Provider:        key,
			Enabled:         enabled,
			APIKey:          crypto.EncryptedString(apiKey),
			CustomAPIURL:    customAPIURL,
			CustomModelName: customModelName,
		}).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]any{
		"enabled":    enabled,
		"base_url":   customAPIURL,
		"model_name": customModelName,
		"updated_at": time.Now().UTC(),
	}
	if apiKey != "" {
		updates["api_key"] = crypto.EncryptedString(apiKey)
	}
	return s.db.Model(existing).Updates(updates).Error
}

// Delete removes a user-owned model configuration.
func (s *ModelStore) Delete(userID, id string) error {
	if uuid.Validate(id) != nil {
		return fmt.Errorf("agent model not found: id=%s, userID=%s", id, userID)
	}
	result := s.db.Where("user_id = ? AND id = ?", userID, id).Delete(&Model{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent model not found: id=%s, userID=%s", id, userID)
	}
	logger.Infof("🗑️ Deleted model: id=%s, userID=%s", id, userID)
	return nil
}
