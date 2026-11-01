package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"vergex/internal/agent"
	"vergex/internal/agent/provider"
	"vergex/internal/config"
	"vergex/internal/logger"
	"vergex/internal/security"
	"vergex/internal/security/crypto"
	"vergex/internal/store"

	"github.com/gin-gonic/gin"
)

// SafeModelConfig Safe model configuration structure (does not contain sensitive information)
type SafeModelConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Enabled         bool   `json:"enabled"`
	HasAPIKey       bool   `json:"has_api_key"`
	CustomAPIURL    string `json:"customApiUrl"`    // Custom API URL (usually not sensitive)
	CustomModelName string `json:"customModelName"` // Custom model name (not sensitive)
}

// ModelConfigUpdate is a single model's update payload. It is a named type
// (rather than an inline anonymous struct) so the log-sanitizer in utils.go is
// guaranteed to stay in sync with this shape — a mismatch there is what let
// plaintext credentials reach the logs previously.
type ModelConfigUpdate struct {
	Enabled         bool   `json:"enabled"`
	APIKey          string `json:"api_key"`
	CustomAPIURL    string `json:"custom_api_url"`
	CustomModelName string `json:"custom_model_name"`
}

type UpdateModelConfigRequest struct {
	Models map[string]ModelConfigUpdate `json:"models"`
}

// handleGetModelConfigs Get model configurations
func (s *Server) handleGetModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	logger.Infof("🔍 Querying model configs for user %s", userID)
	models, err := s.store.AgentModel().List(userID)
	if err != nil {
		logger.Infof("❌ Failed to get model configs: %v", err)
		SafeInternalError(c, "Failed to get model configs", err)
		return
	}

	// If no models in database, return default models
	if len(models) == 0 {
		logger.Infof("⚠️ No models in database, returning defaults")
		c.JSON(http.StatusOK, defaultModelConfigs())
		return
	}

	logger.Infof("✅ Found %d model configs", len(models))

	// Convert to safe response structure, remove sensitive information
	safeModels := make([]SafeModelConfig, 0, len(models))
	for _, model := range models {
		if !store.IsVisibleAgentModel(model) {
			continue
		}
		safeModel := SafeModelConfig{
			ID:              model.ID,
			Name:            model.Name,
			Provider:        model.Provider,
			Enabled:         model.Enabled,
			HasAPIKey:       model.APIKey != "",
			CustomAPIURL:    model.CustomAPIURL,
			CustomModelName: model.CustomModelName,
		}

		safeModels = append(safeModels, safeModel)
	}

	if len(safeModels) == 0 {
		logger.Infof("⚠️ No visible models in database, returning defaults")
		c.JSON(http.StatusOK, defaultModelConfigs())
		return
	}

	c.JSON(http.StatusOK, safeModels)
}

// readSensitiveJSON decodes a request body that carries credentials: plain JSON,
// or an encrypted payload when transport encryption is on. It writes the error
// response itself and returns false when the body cannot be read.
func (s *Server) readSensitiveJSON(c *gin.Context, out any) bool {
	userID := c.GetString("user_id")
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return false
	}

	if !config.Get().TransportEncryption {
		if err := json.Unmarshal(bodyBytes, out); err != nil {
			logger.Infof("❌ Failed to parse plain JSON request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return false
		}
		return true
	}

	var encryptedPayload crypto.EncryptedPayload
	if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
		logger.Infof("❌ Failed to parse encrypted payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format, encrypted transmission required"})
		return false
	}
	if encryptedPayload.WrappedKey == "" {
		logger.Infof("❌ Detected unencrypted request (UserID: %s)", userID)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "This endpoint only supports encrypted transmission, please use encrypted client",
			"code":    "ENCRYPTION_REQUIRED",
			"message": "Encrypted transmission is required for security reasons",
		})
		return false
	}
	decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
	if err != nil {
		logger.Infof("❌ Failed to decrypt request (UserID: %s): %v", userID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decrypt data"})
		return false
	}
	if err := json.Unmarshal([]byte(decrypted), out); err != nil {
		logger.Infof("❌ Failed to parse decrypted data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse decrypted data"})
		return false
	}
	return true
}

// handleUpdateModelConfigs Update model configurations (supports both encrypted and plain text based on config)
func (s *Server) handleUpdateModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")

	var req UpdateModelConfigRequest
	if !s.readSensitiveJSON(c, &req) {
		return
	}

	// Update each model's configuration and track traders that need reload. The
	// key is the model's id, or a provider name until the model exists.
	tradersToReload := make(map[string]bool)
	for key, modelData := range req.Models {
		// SSRF protection: validate custom_api_url before storing
		if modelData.CustomAPIURL != "" {
			cleanURL := strings.TrimSuffix(modelData.CustomAPIURL, "#")
			if err := security.ValidateURL(cleanURL); err != nil {
				logger.Warnf("Invalid custom_api_url for model %s: %v", key, err)
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid custom_api_url for model %s: URL must be a valid HTTPS endpoint", key)})
				return
			}
		}

		// Find traders using this model BEFORE updating. Only a provider can be
		// new: anything else must already exist.
		name := ""
		if existing, err := s.store.AgentModel().Find(userID, key); err == nil {
			traders, _ := s.store.Trader().ListByAgentModelID(userID, existing.ID)
			for _, t := range traders {
				tradersToReload[t.ID] = true
			}
		} else if native, isProvider := findNativeProvider(key); isProvider {
			name = native.Name
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unknown model %s", key)})
			return
		}

		err := s.store.AgentModel().Save(userID, key, name, modelData.Enabled, modelData.APIKey, modelData.CustomAPIURL, modelData.CustomModelName)
		if err != nil {
			SafeInternalError(c, fmt.Sprintf("Update model %s", key), err)
			return
		}
	}

	// Remove affected traders from memory BEFORE reloading to pick up new config
	for traderID := range tradersToReload {
		logger.Infof("🔄 Removing trader %s from memory to reload with new model config", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	// Reload all traders for this user to make new config take effect immediately
	err := s.traderManager.LoadUserTradersFromStore(s.store, userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
		// Don't return error here since model config was successfully updated to database
	}

	logger.Infof("✓ model config updated: %+v", SanitizeModelConfigForLog(req.Models))
	c.JSON(http.StatusOK, gin.H{"message": "Model configuration updated"})
}

// TestModelConfigRequest holds the model form's current values. A blank
// api_key falls back to the key saved for agent_model_id.
type TestModelConfigRequest struct {
	AgentModelID    string `json:"agent_model_id"`
	APIKey          string `json:"api_key"`
	CustomAPIURL    string `json:"custom_api_url"`
	CustomModelName string `json:"custom_model_name"`
}

// modelTestTimeout bounds the single test request.
const modelTestTimeout = 30 * time.Second

// newModelTestClient builds the client a model test calls; tests replace it.
var newModelTestClient = func(providerID string) agent.Client {
	client := agent.NewClientByProvider(providerID, agent.WithTimeout(modelTestTimeout), agent.WithMaxRetries(1))
	agent.TagUsage(client, agent.FeatureTest, "")
	return client
}

// handleTestModelConfig sends one short prompt with the form's settings and
// reports whether the model answered. Nothing is saved; the call's tokens are
// recorded like any other model call.
func (s *Server) handleTestModelConfig(c *gin.Context) {
	userID := c.GetString("user_id")
	var req TestModelConfigRequest
	if !s.readSensitiveJSON(c, &req) {
		return
	}

	providerID := ""
	apiKey := strings.TrimSpace(req.APIKey)
	if saved, err := s.store.AgentModel().Get(userID, req.AgentModelID); err == nil {
		providerID = saved.Provider
		if apiKey == "" {
			apiKey = string(saved.APIKey)
		}
	} else if p, ok := findNativeProvider(req.AgentModelID); ok {
		providerID = p.ID
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown model"})
		return
	}
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter an API key first"})
		return
	}
	baseURL := strings.TrimSpace(req.CustomAPIURL)
	if baseURL != "" {
		if err := security.ValidateURL(strings.TrimSuffix(baseURL, "#")); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Base URL must be a valid HTTPS endpoint"})
			return
		}
	}
	modelName := strings.TrimSpace(req.CustomModelName)
	if modelName == "" {
		if p, ok := findNativeProvider(providerID); ok {
			modelName = p.DefaultModel
		}
	}

	client := newModelTestClient(providerID)
	if client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown model provider"})
		return
	}
	client.SetAPIKey(apiKey, baseURL, strings.TrimSpace(req.CustomModelName))
	started := time.Now()
	if _, err := client.CallWithMessages("You check that an API connection works.", "Reply with OK."); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "model": modelName, "error": modelTestError(err, apiKey)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "model": modelName, "latency_ms": time.Since(started).Milliseconds()})
}

var modelTestStatusPrefix = regexp.MustCompile(`^API returned error \(status (\d+)\): `)

// modelTestError turns a provider error into one short line: the provider's own
// message where the body is JSON, never the API key.
func modelTestError(err error, apiKey string) string {
	msg := strings.TrimPrefix(err.Error(), "still failed after 1 retries: ")
	if i := strings.Index(msg, "{"); i >= 0 {
		var body struct {
			Error   json.RawMessage `json:"error"`
			Message string          `json:"message"`
		}
		if json.Unmarshal([]byte(msg[i:]), &body) == nil {
			var nested struct {
				Message string `json:"message"`
			}
			var plain string
			switch {
			case json.Unmarshal(body.Error, &nested) == nil && nested.Message != "":
				msg = msg[:i] + nested.Message
			case json.Unmarshal(body.Error, &plain) == nil && plain != "":
				msg = msg[:i] + plain
			case body.Message != "":
				msg = msg[:i] + body.Message
			}
		}
	}
	msg = modelTestStatusPrefix.ReplaceAllString(msg, "HTTP $1: ")
	if apiKey != "" {
		msg = strings.ReplaceAll(msg, apiKey, "***")
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if runes := []rune(msg); len(runes) > 200 {
		msg = string(runes[:200]) + "…"
	}
	return msg
}

func findNativeProvider(id string) (nativeProvider, bool) {
	for _, p := range nativeProviders {
		if p.ID == id {
			return p, true
		}
	}
	return nativeProvider{}, false
}

// handleGetSupportedModels Get list of models supported by the system
func (s *Server) handleGetSupportedModels(c *gin.Context) {
	// Return static list of supported models with default versions
	supportedModels := make([]map[string]interface{}, 0, len(nativeProviders))
	for _, p := range nativeProviders {
		supportedModels = append(supportedModels, map[string]interface{}{
			"id": p.ID, "name": p.Name, "provider": p.ID, "defaultModel": p.DefaultModel,
		})
	}

	c.JSON(http.StatusOK, supportedModels)
}

// nativeProvider describes a directly connected model provider (user brings their own API key).
type nativeProvider struct {
	ID           string
	Name         string
	DefaultModel string
}

// nativeProviders is the list of agent providers offered in the model config UI.
// Default model names mirror the defaults used by the agent provider clients.
var nativeProviders = []nativeProvider{
	{ID: agent.ProviderDeepSeek, Name: "DeepSeek", DefaultModel: agent.DefaultDeepSeekModel},
	{ID: agent.ProviderQwen, Name: "Qwen", DefaultModel: provider.DefaultQwenModel},
	{ID: agent.ProviderOpenAI, Name: "OpenAI", DefaultModel: provider.DefaultOpenAIModel},
	{ID: agent.ProviderClaude, Name: "Claude", DefaultModel: provider.DefaultClaudeModel},
	{ID: agent.ProviderGemini, Name: "Google Gemini", DefaultModel: provider.DefaultGeminiModel},
	{ID: agent.ProviderGrok, Name: "Grok (xAI)", DefaultModel: provider.DefaultGrokModel},
	{ID: agent.ProviderKimi, Name: "Kimi (Moonshot)", DefaultModel: provider.DefaultKimiModel},
	{ID: agent.ProviderMiniMax, Name: "MiniMax", DefaultModel: provider.DefaultMiniMaxModel},
}

// defaultModelConfigs returns the unconfigured placeholder entries shown when a user has no models yet.
func defaultModelConfigs() []SafeModelConfig {
	models := make([]SafeModelConfig, 0, len(nativeProviders))
	for _, p := range nativeProviders {
		models = append(models, SafeModelConfig{ID: p.ID, Name: p.Name, Provider: p.ID})
	}
	return models
}
