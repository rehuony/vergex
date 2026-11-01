package provider

import (
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	DefaultGeminiModel   = "gemini-3.1-pro"
)

func init() {
	agent.RegisterProvider(agent.ProviderGemini, func(opts ...agent.ClientOption) agent.Client {
		return NewGeminiClientWithOptions(opts...)
	})
}

type GeminiClient struct {
	*agent.BaseClient
}

func (c *GeminiClient) Base() *agent.BaseClient { return c.BaseClient }

// NewGeminiClientWithOptions creates Gemini client (supports options pattern)
func NewGeminiClientWithOptions(opts ...agent.ClientOption) agent.Client {
	geminiOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderGemini),
		agent.WithModel(DefaultGeminiModel),
		agent.WithBaseURL(DefaultGeminiBaseURL),
	}

	allOpts := append(geminiOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	geminiClient := &GeminiClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = geminiClient
	return geminiClient
}

func (c *GeminiClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [agent] Gemini using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [agent] Gemini using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [agent] Gemini using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [agent] Gemini using default Model: %s", c.Model)
	}
}

// Gemini OpenAI-compatible API uses standard Bearer auth
func (c *GeminiClient) SetAuthHeader(reqHeaders http.Header) {
	c.BaseClient.SetAuthHeader(reqHeaders)
}
