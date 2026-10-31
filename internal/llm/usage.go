package llm

import (
	"net/url"
	"strings"
)

// Usage owners for AI calls that belong to no trader. A trader's calls carry its ID.
const (
	OwnerFrontier = "frontier"
	OwnerTelegram = "telegram"
	// OwnerTest marks the one-off calls of a model test or a strategy test.
	OwnerTest = "test"
)

// TagUsage attributes a client's token usage to owner.
func TagUsage(client AIClient, owner string) {
	if embedder, ok := client.(ClientEmbedder); ok {
		embedder.BaseClient().UsageOwner = owner
	}
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
	if embedder, ok := NewAIClientByProvider(provider).(ClientEmbedder); ok {
		return BaseHost(embedder.BaseClient().BaseURL)
	}
	return ""
}
