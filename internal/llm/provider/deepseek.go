package provider

import (
	"net/http"

	"vergex/internal/llm"
)

func init() {
	llm.RegisterProvider(llm.ProviderDeepSeek, func(opts ...llm.ClientOption) llm.AIClient {
		return NewDeepSeekClientWithOptions(opts...)
	})
}

type DeepSeekClient struct {
	*llm.Client
}

func (c *DeepSeekClient) BaseClient() *llm.Client { return c.Client }

// NewDeepSeekClientWithOptions creates DeepSeek client (supports options pattern)
func NewDeepSeekClientWithOptions(opts ...llm.ClientOption) llm.AIClient {
	deepseekOpts := []llm.ClientOption{
		llm.WithProvider(llm.ProviderDeepSeek),
		llm.WithModel(llm.DefaultDeepSeekModel),
		llm.WithBaseURL(llm.DefaultDeepSeekBaseURL),
	}

	allOpts := append(deepseekOpts, opts...)
	baseClient := llm.NewClient(allOpts...).(*llm.Client)

	dsClient := &DeepSeekClient{
		Client: baseClient,
	}

	baseClient.Hooks = dsClient
	return dsClient
}

func (dsClient *DeepSeekClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	dsClient.APIKey = apiKey

	if customURL != "" {
		dsClient.BaseURL = customURL
		dsClient.Log.Infof("🔧 [LLM] DeepSeek using custom BaseURL: %s", customURL)
	} else {
		dsClient.Log.Infof("🔧 [LLM] DeepSeek using default BaseURL: %s", dsClient.BaseURL)
	}
	if customModel != "" {
		dsClient.Model = customModel
		dsClient.Log.Infof("🔧 [LLM] DeepSeek using custom Model: %s", customModel)
	} else {
		dsClient.Log.Infof("🔧 [LLM] DeepSeek using default Model: %s", dsClient.Model)
	}
}

func (dsClient *DeepSeekClient) SetAuthHeader(reqHeaders http.Header) {
	dsClient.Client.SetAuthHeader(reqHeaders)
}
