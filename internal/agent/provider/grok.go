package provider

import (
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultGrokBaseURL = "https://api.x.ai/v1"
	DefaultGrokModel   = "grok-3-latest"
)

func init() {
	agent.RegisterProvider(agent.ProviderGrok, func(opts ...agent.ClientOption) agent.Client {
		return NewGrokClientWithOptions(opts...)
	})
}

type GrokClient struct {
	*agent.BaseClient
}

func (c *GrokClient) Base() *agent.BaseClient { return c.BaseClient }

// NewGrokClientWithOptions creates Grok client (supports options pattern)
func NewGrokClientWithOptions(opts ...agent.ClientOption) agent.Client {
	grokOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderGrok),
		agent.WithModel(DefaultGrokModel),
		agent.WithBaseURL(DefaultGrokBaseURL),
	}

	allOpts := append(grokOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	grokClient := &GrokClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = grokClient
	return grokClient
}

func (c *GrokClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [agent] Grok using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [agent] Grok using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [agent] Grok using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [agent] Grok using default Model: %s", c.Model)
	}
}

// Grok uses standard OpenAI-compatible API with Bearer auth
func (c *GrokClient) SetAuthHeader(reqHeaders http.Header) {
	c.BaseClient.SetAuthHeader(reqHeaders)
}
