package provider

import (
	"net/http"

	"vergex/internal/agent"
)

func init() {
	agent.RegisterProvider(agent.ProviderDeepSeek, func(opts ...agent.ClientOption) agent.Client {
		return NewDeepSeekClientWithOptions(opts...)
	})
}

type DeepSeekClient struct {
	*agent.BaseClient
}

func (c *DeepSeekClient) Base() *agent.BaseClient { return c.BaseClient }

// NewDeepSeekClientWithOptions creates DeepSeek client (supports options pattern)
func NewDeepSeekClientWithOptions(opts ...agent.ClientOption) agent.Client {
	deepseekOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderDeepSeek),
		agent.WithModel(agent.DefaultDeepSeekModel),
		agent.WithBaseURL(agent.DefaultDeepSeekBaseURL),
	}

	allOpts := append(deepseekOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	dsClient := &DeepSeekClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = dsClient
	return dsClient
}

func (dsClient *DeepSeekClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	dsClient.APIKey = apiKey

	if customURL != "" {
		dsClient.BaseURL = customURL
		dsClient.Log.Infof("🔧 [agent] DeepSeek using custom BaseURL: %s", customURL)
	} else {
		dsClient.Log.Infof("🔧 [agent] DeepSeek using default BaseURL: %s", dsClient.BaseURL)
	}
	if customModel != "" {
		dsClient.Model = customModel
		dsClient.Log.Infof("🔧 [agent] DeepSeek using custom Model: %s", customModel)
	} else {
		dsClient.Log.Infof("🔧 [agent] DeepSeek using default Model: %s", dsClient.Model)
	}
}

func (dsClient *DeepSeekClient) SetAuthHeader(reqHeaders http.Header) {
	dsClient.BaseClient.SetAuthHeader(reqHeaders)
}
