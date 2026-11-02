package agent

import (
	"context"
	"net/url"
	"strings"
)

// The features a call can belong to.
const (
	FeatureTrader   = "trader"
	FeatureFutures  = "futures"
	FeatureTelegram = "telegram"
	// FeatureTest marks the one-off calls of a model test or a trader dry run.
	FeatureTest = "test"
)

// TagUsage attributes a client's calls to a feature and, where it has one, what
// they belong to within it, such as a trader's ID.
func TagUsage(client Client, feature, sourceID string) {
	if embedder, ok := client.(ClientEmbedder); ok {
		embedder.Base().UsageFeature = feature
		embedder.Base().UsageSource = sourceID
	}
}

type conversationKey struct{}

// WithConversation marks the calls made with ctx as part of a conversation.
func WithConversation(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, conversationKey{}, id)
}

func conversationID(ctx context.Context) string {
	id, _ := ctx.Value(conversationKey{}).(string)
	return id
}

// BaseHost returns the host a base URL points at. It leaves out the scheme, path
// and any credentials written into the URL, so it is safe to store and show.
func BaseHost(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return parsed.Host
}

// ProviderHost returns the host a model's calls go to: its custom URL if it has
// one, otherwise the provider's default.
func ProviderHost(provider, customURL string) string {
	if host := BaseHost(customURL); host != "" {
		return host
	}
	if embedder, ok := NewClientByProvider(provider).(ClientEmbedder); ok {
		return BaseHost(embedder.Base().BaseURL)
	}
	return ""
}

// openAIUsage is the usage block of OpenAI-compatible responses. Cached
// prompt tokens are reported as prompt_tokens_details.cached_tokens (OpenAI,
// Qwen, Kimi, Grok) or prompt_cache_hit_tokens (DeepSeek).
type openAIUsage struct {
	PromptTokens         int `json:"prompt_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	TotalTokens          int `json:"total_tokens"`
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails  *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) tokenUsage() *TokenUsage {
	if u.TotalTokens <= 0 {
		return nil
	}
	cached := u.PromptCacheHitTokens
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > cached {
		cached = u.PromptTokensDetails.CachedTokens
	}
	return &TokenUsage{
		PromptTokens:       u.PromptTokens,
		CachedPromptTokens: cached,
		CompletionTokens:   u.CompletionTokens,
		TotalTokens:        u.TotalTokens,
	}
}
