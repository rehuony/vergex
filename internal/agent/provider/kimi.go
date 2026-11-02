package provider

import (
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultKimiBaseURL = "https://api.moonshot.ai/v1" // Global endpoint (use api.moonshot.cn for China)
	DefaultKimiModel   = "kimi-k3.1-pro"
)

func init() {
	agent.RegisterProvider(agent.ProviderKimi, func(opts ...agent.ClientOption) agent.Client {
		return NewKimiClientWithOptions(opts...)
	})
}

type KimiClient struct {
	*agent.BaseClient
}

func (c *KimiClient) Base() *agent.BaseClient { return c.BaseClient }

// NewKimiClientWithOptions creates Kimi client (supports options pattern)
func NewKimiClientWithOptions(opts ...agent.ClientOption) agent.Client {
	kimiOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderKimi),
		agent.WithModel(DefaultKimiModel),
		agent.WithBaseURL(DefaultKimiBaseURL),
		agent.WithTemperature(1.0), // Kimi K2.5 only allows temperature=1
	}

	allOpts := append(kimiOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	kimiClient := &KimiClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = kimiClient
	return kimiClient
}

func (c *KimiClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [agent] Kimi using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [agent] Kimi using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [agent] Kimi using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [agent] Kimi using default Model: %s", c.Model)
	}
}

// Kimi uses standard OpenAI-compatible API
func (c *KimiClient) SetAuthHeader(reqHeaders http.Header) {
	c.BaseClient.SetAuthHeader(reqHeaders)
}
