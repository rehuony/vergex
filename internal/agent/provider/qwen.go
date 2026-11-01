package provider

import (
	"net/http"

	"vergex/internal/agent"
)

const (
	DefaultQwenBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	DefaultQwenModel   = "qwen3-max"
)

func init() {
	agent.RegisterProvider(agent.ProviderQwen, func(opts ...agent.ClientOption) agent.Client {
		return NewQwenClientWithOptions(opts...)
	})
}

type QwenClient struct {
	*agent.BaseClient
}

func (c *QwenClient) Base() *agent.BaseClient { return c.BaseClient }

// NewQwenClientWithOptions creates Qwen client (supports options pattern)
func NewQwenClientWithOptions(opts ...agent.ClientOption) agent.Client {
	qwenOpts := []agent.ClientOption{
		agent.WithProvider(agent.ProviderQwen),
		agent.WithModel(DefaultQwenModel),
		agent.WithBaseURL(DefaultQwenBaseURL),
	}

	allOpts := append(qwenOpts, opts...)
	baseClient := agent.NewClient(allOpts...).(*agent.BaseClient)

	qwenClient := &QwenClient{
		BaseClient: baseClient,
	}

	baseClient.Hooks = qwenClient
	return qwenClient
}

func (qwenClient *QwenClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	qwenClient.APIKey = apiKey

	if customURL != "" {
		qwenClient.BaseURL = customURL
		qwenClient.Log.Infof("🔧 [agent] Qwen using custom BaseURL: %s", customURL)
	} else {
		qwenClient.Log.Infof("🔧 [agent] Qwen using default BaseURL: %s", qwenClient.BaseURL)
	}
	if customModel != "" {
		qwenClient.Model = customModel
		qwenClient.Log.Infof("🔧 [agent] Qwen using custom Model: %s", customModel)
	} else {
		qwenClient.Log.Infof("🔧 [agent] Qwen using default Model: %s", qwenClient.Model)
	}
}

func (qwenClient *QwenClient) SetAuthHeader(reqHeaders http.Header) {
	qwenClient.BaseClient.SetAuthHeader(reqHeaders)
}
