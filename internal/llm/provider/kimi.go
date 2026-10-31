package provider

import (
	"net/http"

	"vergex/internal/llm"
)

const (
	DefaultKimiBaseURL = "https://api.moonshot.ai/v1" // Global endpoint (use api.moonshot.cn for China)
	DefaultKimiModel   = "moonshot-v1-auto"
)

func init() {
	llm.RegisterProvider(llm.ProviderKimi, func(opts ...llm.ClientOption) llm.AIClient {
		return NewKimiClientWithOptions(opts...)
	})
}

type KimiClient struct {
	*llm.Client
}

func (c *KimiClient) BaseClient() *llm.Client { return c.Client }

// NewKimiClientWithOptions creates Kimi client (supports options pattern)
func NewKimiClientWithOptions(opts ...llm.ClientOption) llm.AIClient {
	kimiOpts := []llm.ClientOption{
		llm.WithProvider(llm.ProviderKimi),
		llm.WithModel(DefaultKimiModel),
		llm.WithBaseURL(DefaultKimiBaseURL),
		llm.WithTemperature(1.0), // Kimi K2.5 only allows temperature=1
	}

	allOpts := append(kimiOpts, opts...)
	baseClient := llm.NewClient(allOpts...).(*llm.Client)

	kimiClient := &KimiClient{
		Client: baseClient,
	}

	baseClient.Hooks = kimiClient
	return kimiClient
}

func (c *KimiClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	c.APIKey = apiKey

	if customURL != "" {
		c.BaseURL = customURL
		c.Log.Infof("🔧 [LLM] Kimi using custom BaseURL: %s", customURL)
	} else {
		c.Log.Infof("🔧 [LLM] Kimi using default BaseURL: %s", c.BaseURL)
	}
	if customModel != "" {
		c.Model = customModel
		c.Log.Infof("🔧 [LLM] Kimi using custom Model: %s", customModel)
	} else {
		c.Log.Infof("🔧 [LLM] Kimi using default Model: %s", c.Model)
	}
}

// Kimi uses standard OpenAI-compatible API
func (c *KimiClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
