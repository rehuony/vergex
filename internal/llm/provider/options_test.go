package provider

import (
	"testing"

	"vergex/internal/llm"
)

func TestOptionsWithDeepSeekClient(t *testing.T) {
	logger := llm.NewNoopLogger()

	client := NewDeepSeekClientWithOptions(
		llm.WithAPIKey("sk-deepseek-key"),
		llm.WithLogger(logger),
		llm.WithMaxTokens(5000),
	)

	dsClient := client.(*DeepSeekClient)

	// Verify DeepSeek default values
	if dsClient.Provider != llm.ProviderDeepSeek {
		t.Error("Provider should be DeepSeek")
	}

	if dsClient.BaseURL != llm.DefaultDeepSeekBaseURL {
		t.Error("BaseURL should be DeepSeek default")
	}

	if dsClient.Model != llm.DefaultDeepSeekModel {
		t.Error("Model should be DeepSeek default")
	}

	// Verify custom options
	if dsClient.APIKey != "sk-deepseek-key" {
		t.Error("APIKey should be set from options")
	}

	if dsClient.Log != logger {
		t.Error("Log should be set from options")
	}

	if dsClient.MaxTokens != 5000 {
		t.Error("MaxTokens should be 5000")
	}
}

func TestOptionsWithQwenClient(t *testing.T) {
	logger := llm.NewNoopLogger()

	client := NewQwenClientWithOptions(
		llm.WithAPIKey("sk-qwen-key"),
		llm.WithLogger(logger),
		llm.WithMaxTokens(6000),
	)

	qwenClient := client.(*QwenClient)

	// Verify Qwen default values
	if qwenClient.Provider != llm.ProviderQwen {
		t.Error("Provider should be Qwen")
	}

	if qwenClient.BaseURL != llm.DefaultQwenBaseURL {
		t.Error("BaseURL should be Qwen default")
	}

	if qwenClient.Model != llm.DefaultQwenModel {
		t.Error("Model should be Qwen default")
	}

	// Verify custom options
	if qwenClient.APIKey != "sk-qwen-key" {
		t.Error("APIKey should be set from options")
	}

	if qwenClient.Log != logger {
		t.Error("Log should be set from options")
	}

	if qwenClient.MaxTokens != 6000 {
		t.Error("MaxTokens should be 6000")
	}
}
