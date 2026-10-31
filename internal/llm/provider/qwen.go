package provider

import (
	"net/http"

	"vergex/internal/llm"
)

const (
	DefaultQwenBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	DefaultQwenModel   = "qwen3-max"
)

func init() {
	llm.RegisterProvider(llm.ProviderQwen, func(opts ...llm.ClientOption) llm.AIClient {
		return NewQwenClientWithOptions(opts...)
	})
}

type QwenClient struct {
	*llm.Client
}

func (c *QwenClient) BaseClient() *llm.Client { return c.Client }

// NewQwenClientWithOptions creates Qwen client (supports options pattern)
func NewQwenClientWithOptions(opts ...llm.ClientOption) llm.AIClient {
	qwenOpts := []llm.ClientOption{
		llm.WithProvider(llm.ProviderQwen),
		llm.WithModel(DefaultQwenModel),
		llm.WithBaseURL(DefaultQwenBaseURL),
	}

	allOpts := append(qwenOpts, opts...)
	baseClient := llm.NewClient(allOpts...).(*llm.Client)

	qwenClient := &QwenClient{
		Client: baseClient,
	}

	baseClient.Hooks = qwenClient
	return qwenClient
}

func (qwenClient *QwenClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	qwenClient.APIKey = apiKey

	if customURL != "" {
		qwenClient.BaseURL = customURL
		qwenClient.Log.Infof("🔧 [LLM] Qwen using custom BaseURL: %s", customURL)
	} else {
		qwenClient.Log.Infof("🔧 [LLM] Qwen using default BaseURL: %s", qwenClient.BaseURL)
	}
	if customModel != "" {
		qwenClient.Model = customModel
		qwenClient.Log.Infof("🔧 [LLM] Qwen using custom Model: %s", customModel)
	} else {
		qwenClient.Log.Infof("🔧 [LLM] Qwen using default Model: %s", qwenClient.Model)
	}
}

func (qwenClient *QwenClient) SetAuthHeader(reqHeaders http.Header) {
	qwenClient.Client.SetAuthHeader(reqHeaders)
}
