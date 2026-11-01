package store

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
type AgentModelStore struct {
	db *gorm.DB
}

// AgentModel model configuration
type AgentModel struct {
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

func (AgentModel) TableName() string { return "agent.models" }

// NewAgentModelStore creates a new AgentModelStore
func NewAgentModelStore(db *gorm.DB) *AgentModelStore {
	return &AgentModelStore{db: db}
}

// List retrieves user's model list
func (s *AgentModelStore) List(userID string) ([]*AgentModel, error) {
	var models []*AgentModel
	err := s.db.Where("user_id = ?", userID).Order("created_at, id").Find(&models).Error
	if err != nil {
		return nil, err
	}
	return models, nil
}

// ListEndpoints says where each model is reached. Its key stays unread and so is not
// decrypted.
func (s *AgentModelStore) ListEndpoints(userID string) ([]*AgentModel, error) {
	var models []*AgentModel
	err := s.db.Select("id", "provider", "base_url").Where("user_id = ?", userID).Order("created_at, id").Find(&models).Error
	if err != nil {
		return nil, err
	}
	return models, nil
}

// Get retrieves a single model
func (s *AgentModelStore) Get(userID, modelID string) (*AgentModel, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID cannot be empty")
	}
	if uuid.Validate(modelID) != nil {
		return nil, gorm.ErrRecordNotFound
	}

	var model AgentModel
	if err := s.db.Where("user_id = ? AND id = ?", userID, modelID).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// GetByID retrieves a model by ID only
func (s *AgentModelStore) GetByID(modelID string) (*AgentModel, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID cannot be empty")
	}
	if uuid.Validate(modelID) != nil {
		return nil, gorm.ErrRecordNotFound
	}

	var model AgentModel
	err := s.db.Where("id = ?", modelID).First(&model).Error
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// GetDefault retrieves the default enabled model
func (s *AgentModelStore) GetDefault(userID string) (*AgentModel, error) {
	model, err := s.firstEnabledUsable(userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("please configure an available model in the system first")
	}
	return model, err
}

func (s *AgentModelStore) firstEnabledUsable(userID string) (*AgentModel, error) {
	var models []AgentModel
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

func hasUsableAPIKey(model AgentModel) bool {
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
func (s *AgentModelStore) Find(userID, key string) (*AgentModel, error) {
	if uuid.Validate(key) == nil {
		return s.Get(userID, key)
	}
	var model AgentModel
	if err := s.db.Where("user_id = ? AND provider = ?", userID, key).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

// Save stores a model's settings, under key as in Find, and creates the model named
// name the first time its provider is saved. A blank apiKey keeps the saved key.
func (s *AgentModelStore) Save(userID, key, name string, enabled bool, apiKey, customAPIURL, customModelName string) error {
	existing, err := s.Find(userID, key)
	if errors.Is(err, gorm.ErrRecordNotFound) && uuid.Validate(key) != nil {
		return s.db.Create(&AgentModel{
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
func (s *AgentModelStore) Delete(userID, id string) error {
	if uuid.Validate(id) != nil {
		return fmt.Errorf("agent model not found: id=%s, userID=%s", id, userID)
	}
	result := s.db.Where("user_id = ? AND id = ?", userID, id).Delete(&AgentModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent model not found: id=%s, userID=%s", id, userID)
	}
	logger.Infof("🗑️ Deleted model: id=%s, userID=%s", id, userID)
	return nil
}
