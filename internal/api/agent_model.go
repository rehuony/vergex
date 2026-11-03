package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vergex/internal/agent"
	"vergex/internal/agent/provider"
	"vergex/internal/config"
	"vergex/internal/logger"
	"vergex/internal/security"
	"vergex/internal/security/crypto"

	"github.com/gin-gonic/gin"
)

// newModelClient is what makes a model's client, replaced in tests.
var newModelClient = func(provider string) agent.Client { return agent.NewClientByProvider(provider) }

// modelClient makes a client for one of the user's models, its calls counted to a feature.
// name is another of that connection's models, or blank for the one it is set to.
func (s *Server) modelClient(userID, modelID, name, feature string) (*agent.Model, agent.Client, error) {
	model, err := s.stores.Models.Get(userID, modelID)
	if err != nil || !model.Enabled || model.APIKey == "" {
		return nil, nil, errors.New("Select a configured and enabled model owned by the current user")
	}
	client := newModelClient(model.Provider)
	if client == nil {
		return nil, nil, errors.New("This model provider is not registered in Vergex")
	}
	if name == "" {
		name = model.ModelName
	}
	client.SetAPIKey(string(model.APIKey), model.BaseURL, name)
	client.SetTimeout(90 * time.Second)
	agent.TagUsage(client, feature, "")
	return model, client, nil
}

// SafeModelConfig Safe model configuration structure (does not contain sensitive information)
type SafeModelConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Enabled      bool   `json:"enabled"`
	HasAPIKey    bool   `json:"has_api_key"`
	BaseURL      string `json:"base_url"`      // Custom API URL (usually not sensitive)
	ModelName    string `json:"model_name"`    // Custom model name (not sensitive)
	DefaultModel string `json:"default_model"` // What it calls with no custom name
}

// ModelConfigUpdate is a single model's update payload. It is a named type
// (rather than an inline anonymous struct) so the log-sanitizer in utils.go is
// guaranteed to stay in sync with this shape — a mismatch there is what let
// plaintext credentials reach the logs previously.
type ModelConfigUpdate struct {
	Enabled   bool   `json:"enabled"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	ModelName string `json:"model_name"`
}

type UpdateModelConfigRequest struct {
	Models map[string]ModelConfigUpdate `json:"models"`
}

// handleGetModelConfigs Get model configurations
func (s *Server) handleGetModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	logger.Infof("🔍 Querying model configs for user %s", userID)
	models, err := s.stores.Models.List(userID)
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
		if !agent.IsVisibleModel(model) {
			continue
		}
		safeModel := SafeModelConfig{
			ID:        model.ID,
			Name:      model.Name,
			Provider:  model.Provider,
			Enabled:   model.Enabled,
			HasAPIKey: model.APIKey != "",
			BaseURL:   model.BaseURL,
			ModelName: model.ModelName,
		}
		if native, ok := findNativeProvider(model.Provider); ok {
			safeModel.DefaultModel = native.DefaultModel
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
		// SSRF protection: validate base_url before storing
		if modelData.BaseURL != "" {
			cleanURL := strings.TrimSuffix(modelData.BaseURL, "#")
			if err := security.ValidateURL(cleanURL); err != nil {
				logger.Warnf("Invalid base_url for model %s: %v", key, err)
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid base_url for model %s: URL must be a valid HTTPS endpoint", key)})
				return
			}
		}

		// Find traders using this model BEFORE updating. Only a provider can be
		// new: anything else must already exist.
		name := ""
		if existing, err := s.stores.Models.Find(userID, key); err == nil {
			traders, _ := s.stores.Traders.ListByAgentModelID(userID, existing.ID)
			for _, t := range traders {
				tradersToReload[t.ID] = true
			}
		} else if native, isProvider := findNativeProvider(key); isProvider {
			name = native.Name
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unknown model %s", key)})
			return
		}

		err := s.stores.Models.Save(userID, key, name, modelData.Enabled, modelData.APIKey, modelData.BaseURL, modelData.ModelName)
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
	err := s.traderManager.LoadUserTradersFromStore(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
		// Don't return error here since model config was successfully updated to database
	}

	logger.Infof("✓ model config updated: %+v", SanitizeModelConfigForLog(req.Models))
	c.JSON(http.StatusOK, gin.H{"message": "Model configuration updated"})
}

// handleDeleteModel removes a model and its key. A trader in use still needs it, so that
// refuses, naming them.
func (s *Server) handleDeleteModel(c *gin.Context) {
	userID := c.GetString("user_id")
	model, err := s.stores.Models.Get(userID, c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Model")
		return
	}
	traders, err := s.stores.Traders.ListByAgentModelID(userID, model.ID)
	if err != nil {
		SafeInternalError(c, "Delete the model", err)
		return
	}
	if len(traders) > 0 {
		names := make([]string, len(traders))
		for i, trader := range traders {
			names[i] = trader.Name
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":        "These traders use this model: " + strings.Join(names, ", "),
			"error_key":    "connections.inUse",
			"error_params": gin.H{"names": strings.Join(names, ", ")},
		})
		return
	}
	if err := s.stores.Models.Delete(userID, model.ID); err != nil {
		SafeInternalError(c, "Delete the model", err)
		return
	}
	// Telegram may have been set to it, and keeps its clients until it reloads.
	s.reloadTelegram()
	c.JSON(http.StatusOK, gin.H{"deleted": model.ID})
}

// TestModelConfigRequest holds the model form's current values. A blank
// api_key falls back to the key saved for agent_model_id.
type TestModelConfigRequest struct {
	AgentModelID string `json:"agent_model_id"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	ModelName    string `json:"model_name"`
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

	providerID, apiKey, baseURL, refusal := s.modelFormValues(userID, req)
	if refusal != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": refusal})
		return
	}
	modelName := strings.TrimSpace(req.ModelName)
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
	client.SetAPIKey(apiKey, baseURL, strings.TrimSpace(req.ModelName))
	started := time.Now()
	if _, err := client.CallWithMessages("You check that an API connection works.", "Reply with OK."); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "model": modelName, "error": modelTestError(err, apiKey)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "model": modelName, "latency_ms": time.Since(started).Milliseconds()})
}

// modelFormValues reads what the model form says: which provider, with the key typed in or
// else the saved one, and the endpoint. What is wrong with them comes back as refusal.
func (s *Server) modelFormValues(userID string, req TestModelConfigRequest) (providerID, apiKey, baseURL, refusal string) {
	apiKey = strings.TrimSpace(req.APIKey)
	if saved, err := s.stores.Models.Get(userID, req.AgentModelID); err == nil {
		providerID = saved.Provider
		if apiKey == "" {
			apiKey = string(saved.APIKey)
		}
	} else if p, ok := findNativeProvider(req.AgentModelID); ok {
		providerID = p.ID
	} else {
		return "", "", "", "Unknown model"
	}
	if apiKey == "" {
		return "", "", "", "Enter an API key first"
	}
	baseURL = strings.TrimSpace(req.BaseURL)
	if baseURL != "" {
		if err := security.ValidateURL(strings.TrimSuffix(baseURL, "#")); err != nil {
			return "", "", "", "Base URL must be a valid HTTPS endpoint"
		}
	}
	return providerID, apiKey, baseURL, ""
}

// modelListTimeout bounds asking a provider what it offers.
const modelListTimeout = 15 * time.Second

// newModelListClient builds the client that asks a provider what it offers; tests replace it.
var newModelListClient = func(providerID string) agent.Client {
	return agent.NewClientByProvider(providerID, agent.WithTimeout(modelListTimeout), agent.WithMaxRetries(1))
}

// answerModelList asks a provider what it offers and answers with the names, or with the
// provider's own short message. It costs no tokens, so it records no usage.
func answerModelList(c *gin.Context, providerID, apiKey, baseURL string) {
	client := newModelListClient(providerID)
	if client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown model provider"})
		return
	}
	client.SetAPIKey(apiKey, baseURL, "")
	ctx, cancel := context.WithTimeout(c.Request.Context(), modelListTimeout)
	defer cancel()
	names, err := agent.ListModels(ctx, client)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": modelTestError(err, apiKey)})
		return
	}
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"models": names})
}

// handleSavedModelList says what a saved model's provider offers, with its saved key.
func (s *Server) handleSavedModelList(c *gin.Context) {
	saved, err := s.stores.Models.Get(c.GetString("user_id"), c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Model")
		return
	}
	answerModelList(c, saved.Provider, string(saved.APIKey), saved.BaseURL)
}

// handleModelFormList says what a provider offers with the model form's current values.
func (s *Server) handleModelFormList(c *gin.Context) {
	var req TestModelConfigRequest
	if !s.readSensitiveJSON(c, &req) {
		return
	}
	providerID, apiKey, baseURL, refusal := s.modelFormValues(c.GetString("user_id"), req)
	if refusal != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": refusal})
		return
	}
	answerModelList(c, providerID, apiKey, baseURL)
}

// modelTestError turns a provider error into one short line: the provider's own
// message where the body is JSON, never the API key.
func modelTestError(err error, apiKey string) string {
	return agent.ProviderMessage(err, apiKey)
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
			"id": p.ID, "name": p.Name, "provider": p.ID, "default_model": p.DefaultModel, "default_base_url": p.DefaultBaseURL,
		})
	}

	c.JSON(http.StatusOK, supportedModels)
}

// nativeProvider describes a directly connected model provider (user brings their own API key).
type nativeProvider struct {
	ID             string
	Name           string
	DefaultModel   string
	DefaultBaseURL string
}

// nativeProviders is the list of agent providers offered in the model config UI.
// Default model names mirror the defaults used by the agent provider clients.
var nativeProviders = []nativeProvider{
	{ID: agent.ProviderDeepSeek, Name: "DeepSeek", DefaultModel: agent.DefaultDeepSeekModel, DefaultBaseURL: agent.DefaultDeepSeekBaseURL},
	{ID: agent.ProviderQwen, Name: "Qwen", DefaultModel: provider.DefaultQwenModel, DefaultBaseURL: provider.DefaultQwenBaseURL},
	{ID: agent.ProviderOpenAI, Name: "OpenAI", DefaultModel: provider.DefaultOpenAIModel, DefaultBaseURL: provider.DefaultOpenAIBaseURL},
	{ID: agent.ProviderClaude, Name: "Claude", DefaultModel: provider.DefaultClaudeModel, DefaultBaseURL: provider.DefaultClaudeBaseURL},
	{ID: agent.ProviderGemini, Name: "Google Gemini", DefaultModel: provider.DefaultGeminiModel, DefaultBaseURL: provider.DefaultGeminiBaseURL},
	{ID: agent.ProviderGrok, Name: "Grok (xAI)", DefaultModel: provider.DefaultGrokModel, DefaultBaseURL: provider.DefaultGrokBaseURL},
	{ID: agent.ProviderKimi, Name: "Kimi (Moonshot)", DefaultModel: provider.DefaultKimiModel, DefaultBaseURL: provider.DefaultKimiBaseURL},
	{ID: agent.ProviderMiniMax, Name: "MiniMax", DefaultModel: provider.DefaultMiniMaxModel, DefaultBaseURL: provider.DefaultMiniMaxBaseURL},
}

// defaultModelConfigs returns the unconfigured placeholder entries shown when a user has no models yet.
func defaultModelConfigs() []SafeModelConfig {
	models := make([]SafeModelConfig, 0, len(nativeProviders))
	for _, p := range nativeProviders {
		models = append(models, SafeModelConfig{ID: p.ID, Name: p.Name, Provider: p.ID, DefaultModel: p.DefaultModel})
	}
	return models
}
