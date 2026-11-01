package provider

import (
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
	DefaultOpenAIModel   = "gpt-5.4"
)

func init() {
	agent.RegisterProvider(agent.ProviderOpenAI, func(opts ...agent.ClientOption) agent.Client {
		return NewOpenAIClientWithOptions(opts...)
	})
}

type OpenAIClient struct {
	*agent.BaseClient
}

func (c *OpenAIClient) Base() *agent.BaseClient { return c.BaseClient }

// NewOpenAIClientWithOptions creates OpenAI client (supports options pattern)
func NewOpenAIClientWithOptions(opts ...agent.ClientOption) agent.Client {
	openaiOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderOpenAI),
		agent.WithModel(DefaultOpenAIModel),
		agent.WithBaseURL(DefaultOpenAIBaseURL),
	}

	allOpts := append(openaiOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	openaiClient := &OpenAIClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = openaiClient
	return openaiClient
}

func (c *OpenAIClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [agent] OpenAI using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [agent] OpenAI using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [agent] OpenAI using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [agent] OpenAI using default Model: %s", c.Model)
	}
}

// OpenAI uses standard Bearer auth
func (c *OpenAIClient) SetAuthHeader(reqHeaders http.Header) {
	c.BaseClient.SetAuthHeader(reqHeaders)
}
