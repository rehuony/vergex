package provider

import (
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultMiniMaxBaseURL = "https://api.minimax.io/v1"
	DefaultMiniMaxModel   = "MiniMax-M2.7"
)

func init() {
	agent.RegisterProvider(agent.ProviderMiniMax, func(opts ...agent.ClientOption) agent.Client {
		return NewMiniMaxClientWithOptions(opts...)
	})
}

type MiniMaxClient struct {
	*agent.BaseClient
}

func (c *MiniMaxClient) Base() *agent.BaseClient { return c.BaseClient }

// NewMiniMaxClientWithOptions creates MiniMax client (supports options pattern)
func NewMiniMaxClientWithOptions(opts ...agent.ClientOption) agent.Client {
	minimaxOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderMiniMax),
		agent.WithModel(DefaultMiniMaxModel),
		agent.WithBaseURL(DefaultMiniMaxBaseURL),
	}

	allOpts := append(minimaxOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	minimaxClient := &MiniMaxClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = minimaxClient
	return minimaxClient
}

func (c *MiniMaxClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [agent] MiniMax using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [agent] MiniMax using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [agent] MiniMax using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [agent] MiniMax using default Model: %s", c.Model)
	}
}

// MiniMax uses standard OpenAI-compatible API with Bearer auth
func (c *MiniMaxClient) SetAuthHeader(reqHeaders http.Header) {
	c.BaseClient.SetAuthHeader(reqHeaders)
}
