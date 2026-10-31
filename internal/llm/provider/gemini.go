package provider

import (
	"net/http"

	"vergex/internal/llm"
)

const (
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	DefaultGeminiModel   = "gemini-3.1-pro"
)

func init() {
	llm.RegisterProvider(llm.ProviderGemini, func(opts ...llm.ClientOption) llm.AIClient {
		return NewGeminiClientWithOptions(opts...)
	})
}

type GeminiClient struct {
	*llm.Client
}

func (c *GeminiClient) BaseClient() *llm.Client { return c.Client }

// NewGeminiClientWithOptions creates Gemini client (supports options pattern)
func NewGeminiClientWithOptions(opts ...llm.ClientOption) llm.AIClient {
	geminiOpts := []llm.ClientOption{
		llm.WithProvider(llm.ProviderGemini),
		llm.WithModel(DefaultGeminiModel),
		llm.WithBaseURL(DefaultGeminiBaseURL),
	}

	allOpts := append(geminiOpts, opts...)
	baseClient := llm.NewClient(allOpts...).(*llm.Client)

	geminiClient := &GeminiClient{
		Client: baseClient,
	}

	baseClient.Hooks = geminiClient
	return geminiClient
}

func (c *GeminiClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [LLM] Gemini using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [LLM] Gemini using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [LLM] Gemini using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [LLM] Gemini using default Model: %s", c.Model)
	}
}

// Gemini OpenAI-compatible API uses standard Bearer auth
func (c *GeminiClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
