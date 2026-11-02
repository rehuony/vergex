// Package pricing estimates model call costs from token usage using the public
// models.dev price catalog (USD per million tokens).
//
// The catalog is cached in core.settings, loaded at startup and refreshed in the
// background every 24 hours. A failed download keeps the previous cache, so
// cost estimation never blocks trading. Models that cannot be matched are
// reported as unpriced instead of guessing a value.
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"vergex/internal/database"
	"vergex/internal/logger"
	"vergex/internal/service"
)

const (
	// SourceURL is the models.dev catalog endpoint.
	SourceURL = "https://models.dev/api.json"
	// RefreshInterval is how often the catalog is re-downloaded.
	RefreshInterval = 24 * time.Hour
	// settingsModule is the row in core.settings holding the cached catalog.
	settingsModule = "agent_pricing"

	longContextThreshold = 200_000
	maxCatalogBytes      = 64 << 20
)

// Cost is a model's price in USD per million tokens.
type Cost struct {
	Input       float64  `json:"input"`
	Output      float64  `json:"output"`
	CacheRead   *float64 `json:"cache_read,omitempty"`
	LongContext *Cost    `json:"context_over_200k,omitempty"`
}

// Usage is the token usage of one model call.
type Usage struct {
	PromptTokens       int // includes cached prompt tokens
	CachedPromptTokens int
	CompletionTokens   int
}

// Estimate returns the USD cost of a call priced at c.
func (c Cost) Estimate(u Usage) float64 {
	price := c
	if u.PromptTokens > longContextThreshold && c.LongContext != nil {
		price = *c.LongContext
	}
	cached := u.CachedPromptTokens
	if cached < 0 {
		cached = 0
	}
	if cached > u.PromptTokens {
		cached = u.PromptTokens
	}
	cacheRead := price.Input
	if price.CacheRead != nil {
		cacheRead = *price.CacheRead
	}
	total := float64(u.PromptTokens-cached)*price.Input +
		float64(cached)*cacheRead +
		float64(u.CompletionTokens)*price.Output
	return total / 1e6
}

// Catalog maps a models.dev provider id to its model prices.
type Catalog map[string]map[string]Cost

type cachedCatalog struct {
	FetchedAt time.Time `json:"fetched_at"`
	Providers Catalog   `json:"providers"`
}

// providerRoute maps a native provider to models.dev provider ids. The first
// rule whose host matches the base URL wins; an empty base URL uses the
// default id. A base URL on an unknown host is a custom endpoint and stays
// unpriced.
type providerRoute struct {
	defaultID string
	hosts     map[string]string // host -> models.dev provider id
}

var providerRoutes = map[string]providerRoute{
	"deepseek": {defaultID: "deepseek", hosts: map[string]string{"api.deepseek.com": "deepseek"}},
	"openai":   {defaultID: "openai", hosts: map[string]string{"api.openai.com": "openai"}},
	"claude":   {defaultID: "anthropic", hosts: map[string]string{"api.anthropic.com": "anthropic"}},
	"qwen": {defaultID: "alibaba-cn", hosts: map[string]string{
		"dashscope.aliyuncs.com":      "alibaba-cn",
		"dashscope-intl.aliyuncs.com": "alibaba",
	}},
	"gemini": {defaultID: "google", hosts: map[string]string{"generativelanguage.googleapis.com": "google"}},
	"grok":   {defaultID: "xai", hosts: map[string]string{"api.x.ai": "xai"}},
	"kimi": {defaultID: "moonshotai", hosts: map[string]string{
		"api.moonshot.ai": "moonshotai",
		"api.moonshot.cn": "moonshotai-cn",
	}},
	"minimax": {defaultID: "minimax", hosts: map[string]string{
		"api.minimax.io":   "minimax",
		"api.minimaxi.com": "minimax-cn",
		"api.minimax.cn":   "minimax-cn",
	}},
}

// catalogProviderIDs lists every models.dev provider id kept in the cache.
func catalogProviderIDs() map[string]bool {
	ids := make(map[string]bool)
	for _, route := range providerRoutes {
		ids[route.defaultID] = true
		for _, id := range route.hosts {
			ids[id] = true
		}
	}
	return ids
}

// ResolveProvider returns the models.dev provider id for a native provider
// and base URL, or false when the endpoint is custom or unknown.
func ResolveProvider(provider, baseURL string) (string, bool) {
	route, ok := providerRoutes[strings.ToLower(strings.TrimSpace(provider))]
	if !ok {
		return "", false
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return route.defaultID, true
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", false
	}
	id, ok := route.hosts[strings.ToLower(u.Hostname())]
	return id, ok
}

// Lookup finds the price of model on the given native provider endpoint.
func (c Catalog) Lookup(provider, baseURL, model string) (Cost, bool) {
	providerID, ok := ResolveProvider(provider, baseURL)
	if !ok {
		return Cost{}, false
	}
	models := c[providerID]
	model = strings.TrimSpace(model)
	if cost, ok := models[model]; ok {
		return cost, true
	}
	for id, cost := range models {
		if strings.EqualFold(id, model) {
			return cost, true
		}
	}
	return Cost{}, false
}

// parseCatalog extracts the prices of the supported providers from a raw
// models.dev api.json document.
func parseCatalog(data []byte) (Catalog, error) {
	var raw map[string]struct {
		Models map[string]struct {
			Cost *Cost `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode models.dev catalog: %w", err)
	}
	catalog := make(Catalog)
	for id := range catalogProviderIDs() {
		provider, ok := raw[id]
		if !ok {
			continue
		}
		models := make(map[string]Cost)
		for modelID, model := range provider.Models {
			if model.Cost != nil {
				models[modelID] = *model.Cost
			}
		}
		catalog[id] = models
	}
	if len(catalog) == 0 {
		return nil, fmt.Errorf("models.dev catalog has none of the supported providers")
	}
	return catalog, nil
}

// Service holds the current catalog and keeps it fresh.
type Service struct {
	mu        sync.RWMutex
	catalog   Catalog
	fetchedAt time.Time
	db        *gorm.DB
	sourceURL string
	client    *http.Client
}

// NewService creates a service caching the catalog in the database.
func NewService(db *gorm.DB) *Service {
	return &Service{
		db:        db,
		sourceURL: SourceURL,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Lookup finds a model price in the current catalog.
func (s *Service) Lookup(provider, baseURL, model string) (Cost, bool) {
	if s == nil {
		return Cost{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.catalog.Lookup(provider, baseURL, model)
}

// Estimate returns the estimated cost of a call, or false when unpriced.
func (s *Service) Estimate(provider, baseURL, model string, usage Usage) (float64, bool) {
	cost, ok := s.Lookup(provider, baseURL, model)
	if !ok {
		return 0, false
	}
	return cost.Estimate(usage), true
}

// LoadCache loads the cached catalog, if present.
func (s *Service) LoadCache() error {
	var cache cachedCatalog
	found, err := database.LoadSetting(s.db, settingsModule, &cache)
	if err != nil || !found {
		return err
	}
	s.mu.Lock()
	s.catalog = cache.Providers
	s.fetchedAt = cache.FetchedAt
	s.mu.Unlock()
	return nil
}

// Refresh downloads the catalog and replaces the cache. On failure the
// current catalog is kept.
func (s *Service) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.sourceURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download models.dev catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download models.dev catalog: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes))
	if err != nil {
		return fmt.Errorf("read models.dev catalog: %w", err)
	}
	catalog, err := parseCatalog(data)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	s.catalog = catalog
	s.fetchedAt = now
	s.mu.Unlock()

	return database.PatchSetting(s.db, settingsModule, map[string]any{"fetched_at": now, "providers": catalog})
}

func (s *Service) age() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.fetchedAt.IsZero() {
		return RefreshInterval
	}
	return time.Since(s.fetchedAt)
}

// Start loads the cache and refreshes the catalog in the background until
// ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	if err := s.LoadCache(); err != nil {
		logger.Warnf("⚠️ Failed to load model pricing cache: %v", err)
	}
	service.Report(service.AgentPricing, service.Running, "")
	go func() {
		wait := RefreshInterval - s.age()
		if wait < 0 {
			wait = 0
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if err := s.Refresh(ctx); err != nil {
				logger.Warnf("⚠️ Failed to refresh model pricing from models.dev (keeping previous prices): %v", err)
				service.Report(service.AgentPricing, service.Degraded, "Keeping the previous prices")
				wait = time.Hour
				continue
			}
			logger.Infof("💲 model pricing catalog refreshed from models.dev")
			service.Report(service.AgentPricing, service.Running, "")
			wait = RefreshInterval
		}
	}()
}
