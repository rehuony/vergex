package provider

import (
	"net/http"

	"vergex/internal/llm"
)

const (
	DefaultMiniMaxBaseURL = "https://api.minimax.io/v1"
	DefaultMiniMaxModel   = "MiniMax-M2.7"
)

func init() {
	llm.RegisterProvider(llm.ProviderMiniMax, func(opts ...llm.ClientOption) llm.AIClient {
		return NewMiniMaxClientWithOptions(opts...)
	})
}

type MiniMaxClient struct {
	*llm.Client
}

func (c *MiniMaxClient) BaseClient() *llm.Client { return c.Client }

// NewMiniMaxClientWithOptions creates MiniMax client (supports options pattern)
func NewMiniMaxClientWithOptions(opts ...llm.ClientOption) llm.AIClient {
	minimaxOpts := []llm.ClientOption{
		llm.WithProvider(llm.ProviderMiniMax),
		llm.WithModel(DefaultMiniMaxModel),
		llm.WithBaseURL(DefaultMiniMaxBaseURL),
	}

	allOpts := append(minimaxOpts, opts...)
	baseClient := llm.NewClient(allOpts...).(*llm.Client)

	minimaxClient := &MiniMaxClient{
		Client: baseClient,
	}

	baseClient.Hooks = minimaxClient
	return minimaxClient
}

func (c *MiniMaxClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [LLM] MiniMax using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [LLM] MiniMax using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [LLM] MiniMax using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [LLM] MiniMax using default Model: %s", c.Model)
	}
}

// MiniMax uses standard OpenAI-compatible API with Bearer auth
func (c *MiniMaxClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
