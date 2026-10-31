package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ProviderCustom = "custom"

	DefaultTemperature = 0.5
)

var (
	DefaultTimeout = 120 * time.Second

	MaxRetryTimes = 3

	retryableErrors = []string{
		"EOF",
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"no such host",
		"stream error",   // HTTP/2 stream error
		"INTERNAL_ERROR", // Server internal error
		"status 429",     // Rate limit / upstream gateway throttling
		"rate_limit_error",
		"upstream_empty_output",
		"status 502", // Bad Gateway
		"status 503", // Service Unavailable
		"status 520", // Cloudflare origin error
		"status 524", // Cloudflare timeout
	}

	// TokenUsageCallback is called after each AI request with token usage info
	TokenUsageCallback func(usage TokenUsage)
)

// TokenUsage represents token usage from AI API response
type TokenUsage struct {
	Provider           string
	Model              string
	BaseURL            string
	Owner              string // Client.UsageOwner of the reporting client (e.g. trader ID)
	PromptTokens       int    // includes CachedPromptTokens
	CachedPromptTokens int
	CompletionTokens   int
	TotalTokens        int
}

// Client AI API configuration
type Client struct {
	Provider   string
	APIKey     string
	BaseURL    string
	Model      string
	UseFullURL bool // Whether to use full URL (without appending /chat/completions)
	MaxTokens  int  // Maximum tokens for AI response

	HTTPClient *http.Client // Exported for sub-packages
	Log        Logger       // Exported for sub-packages
	Cfg        *Config      // Exported for sub-packages

	// UsageOwner tags token usage reported by this client (e.g. a trader ID)
	// so cost records can be attributed.
	UsageOwner string

	// Hooks are used to implement dynamic dispatch (polymorphism)
	// When provider.DeepSeekClient embeds Client, Hooks point to DeepSeekClient
	// This way methods called in Call() are automatically dispatched to the overridden version
	Hooks ClientHooks
}

// New creates default client (backward compatible)
//
// Deprecated: Recommend using NewClient(...opts) for better flexibility
func New() AIClient {
	return NewClient()
}

// NewClient creates client (supports options pattern)
//
// Usage examples:
//
//	// Basic usage (backward compatible)
//	client := llm.NewClient()
//
//	// Custom logger
//	client := llm.NewClient(llm.WithLogger(customLogger))
//
//	// Custom timeout
//	client := llm.NewClient(llm.WithTimeout(60*time.Second))
//
//	// Combine multiple options
//	client := llm.NewClient(
//	    llm.WithDeepSeekConfig("sk-xxx"),
//	    llm.WithLogger(customLogger),
//	    llm.WithTimeout(60*time.Second),
//	)
func NewClient(opts ...ClientOption) AIClient {
	// 1. Create default config
	cfg := DefaultConfig()

	// 2. Apply user options
	for _, opt := range opts {
		opt(cfg)
	}

	// 3. Create client instance
	client := &Client{
		Provider:   cfg.Provider,
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		MaxTokens:  cfg.MaxTokens,
		UseFullURL: cfg.UseFullURL,
		HTTPClient: cfg.HTTPClient,
		Log:        cfg.Logger,
		Cfg:        cfg,
	}

	// 4. Set default Provider (if not set)
	if client.Provider == "" {
		client.Provider = ProviderDeepSeek
		client.BaseURL = DefaultDeepSeekBaseURL
		client.Model = DefaultDeepSeekModel
	}

	// 5. Set hooks to point to self
	client.Hooks = client

	return client
}

// SetCustomAPI sets custom OpenAI-compatible API
func (client *Client) SetAPIKey(apiKey, apiURL, customModel string) {
	client.Provider = ProviderCustom
	client.APIKey = apiKey

	// Check if URL ends with #, if so use full URL (without appending /chat/completions)
	if strings.HasSuffix(apiURL, "#") {
		client.BaseURL = strings.TrimSuffix(apiURL, "#")
		client.UseFullURL = true
	} else {
		client.BaseURL = apiURL
		client.UseFullURL = false
	}

	client.Model = customModel
}

func (client *Client) SetTimeout(timeout time.Duration) {
	client.HTTPClient.Timeout = timeout
}

// CallWithMessages template method - fixed retry flow (cannot be overridden)
func (client *Client) CallWithMessages(systemPrompt, userPrompt string) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("AI API key not set, please call SetAPIKey first")
	}

	// Fixed retry flow
	var lastErr error
	maxRetries := client.Cfg.MaxRetries

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			client.Log.Warnf("⚠️  AI API call failed, retrying (%d/%d)...", attempt, maxRetries)
		}

		// Call the fixed single-call flow
		result, err := client.Hooks.Call(systemPrompt, userPrompt)
		if err == nil {
			if attempt > 1 {
				client.Log.Infof("✓ AI API retry succeeded")
			}
			return result, nil
		}

		lastErr = err
		// Check if error is retryable via hooks (supports custom retry strategy)
		if !client.Hooks.IsRetryableError(err) {
			return "", err
		}

		// Wait before retry
		if attempt < maxRetries {
			waitTime := client.Cfg.RetryWaitBase * time.Duration(attempt)
			client.Log.Infof("⏳ Waiting %v before retry...", waitTime)
			if err := sleepWithContext(context.Background(), waitTime); err != nil {
				return "", err
			}
		}
	}

	return "", fmt.Errorf("still failed after %d retries: %w", maxRetries, lastErr)
}

func (client *Client) SetAuthHeader(reqHeader http.Header) {
	reqHeader.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
}

// modelSupportsCustomTemperature reports whether the target model accepts a
// non-default temperature. OpenAI reasoning models (gpt-5/6 families, o-series)
// reject any value other than the default and fail the whole request with
// "unsupported_value", so temperature must be omitted for them.
func modelSupportsCustomTemperature(model string) bool {
	m := strings.ToLower(model)
	return !strings.HasPrefix(m, "gpt-5") &&
		!strings.HasPrefix(m, "gpt-6") &&
		!strings.HasPrefix(m, "o1") &&
		!strings.HasPrefix(m, "o3") &&
		!strings.HasPrefix(m, "o4")
}

func (client *Client) BuildRequestBody(systemPrompt, userPrompt string) map[string]any {
	// Build messages array
	messages := []map[string]string{}

	// If system prompt exists, add system message
	if systemPrompt != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	// Add user message
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": userPrompt,
	})

	// Guard: truncate messages if they would exceed the model's context window
	if client.Cfg.MaxContext > 0 {
		truncated, removed := truncateMessages(messages, client.Cfg.MaxContext, client.MaxTokens)
		if removed > 0 {
			client.Log.Warnf("⚠️  [%s] Context guard: truncated %d oldest messages to fit within %d token limit",
				client.String(), removed, client.Cfg.MaxContext)
			messages = truncated
		}
	}

	// Build request body
	requestBody := map[string]interface{}{
		"model":    client.Model,
		"messages": messages,
	}
	if modelSupportsCustomTemperature(client.Model) {
		requestBody["temperature"] = client.Cfg.Temperature
	}
	// OpenAI newer models use max_completion_tokens instead of max_tokens
	if client.Provider == ProviderOpenAI {
		requestBody["max_completion_tokens"] = client.MaxTokens
	} else {
		requestBody["max_tokens"] = client.MaxTokens
	}
	return requestBody
}

// MarshalRequestBody can be used to marshal the request body and can be overridden
func (client *Client) MarshalRequestBody(requestBody map[string]any) ([]byte, error) {
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize request: %w", err)
	}
	return jsonData, nil
}

func (client *Client) ParseResponse(body []byte) (string, error) {
	r, err := client.ParseResponseFull(body)
	if err != nil {
		return "", err
	}
	return r.Content, nil
}

// ParseResponseFull parses the OpenAI-format response body and returns both
// the text content and any tool calls.
func (client *Client) ParseResponseFull(body []byte) (*LLMResponse, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content          string     `json:"content"`
				ReasoningContent string     `json:"reasoning_content"`
				ToolCalls        []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage openAIUsage `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("API returned empty response")
	}

	client.ReportUsage(result.Usage.tokenUsage())

	msg := result.Choices[0].Message
	return &LLMResponse{
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        msg.ToolCalls,
	}, nil
}

func (client *Client) BuildUrl() string {
	if client.UseFullURL {
		return client.BaseURL
	}
	return fmt.Sprintf("%s/chat/completions", client.BaseURL)
}

func (client *Client) BuildRequest(url string, jsonData []byte) (*http.Request, error) {
	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("fail to build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set auth header via hooks (supports overriding)
	client.Hooks.SetAuthHeader(req.Header)

	return req, nil
}

func contextFromRequest(req *Request) context.Context {
	if req != nil && req.Ctx != nil {
		return req.Ctx
	}
	return context.Background()
}

func (client *Client) buildHTTPRequestWithContext(ctx context.Context, url string, jsonData []byte) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	httpReq, err := client.Hooks.BuildRequest(url, jsonData)
	if err != nil {
		return nil, err
	}
	return httpReq.WithContext(ctx), nil
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Call single AI API call (fixed flow, cannot be overridden)
func (client *Client) Call(systemPrompt, userPrompt string) (string, error) {
	// Print current AI configuration
	client.Log.Infof("📡 [%s] Request AI Server: BaseURL: %s", client.String(), client.BaseURL)
	client.Log.Debugf("[%s] UseFullURL: %v", client.String(), client.UseFullURL)

	// Step 1: Build request body (via hooks for dynamic dispatch)
	requestBody := client.Hooks.BuildRequestBody(systemPrompt, userPrompt)

	// Step 2: Serialize request body (via hooks for dynamic dispatch)
	jsonData, err := client.Hooks.MarshalRequestBody(requestBody)
	if err != nil {
		return "", err
	}

	// Step 3: Build URL (via hooks for dynamic dispatch)
	url := client.Hooks.BuildUrl()
	client.Log.Infof("📡 [LLM %s] Request URL: %s", client.String(), url)

	// Step 4: Create HTTP request (fixed logic)
	req, err := client.Hooks.BuildRequest(url, jsonData)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Step 5: Send HTTP request (fixed logic)
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Step 6: Read response body (fixed logic)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Step 7: Check HTTP status code (fixed logic)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned error (status %d): %s", resp.StatusCode, string(body))
	}

	// Step 8: Parse response (via hooks for dynamic dispatch)
	result, err := client.Hooks.ParseResponse(body)
	if err != nil {
		return "", fmt.Errorf("fail to parse AI server response: %w", err)
	}

	return result, nil
}

func (client *Client) String() string {
	return fmt.Sprintf("[Provider: %s, Model: %s]",
		client.Provider, client.Model)
}

// BaseClient returns the underlying *Client (satisfies ClientEmbedder interface).
func (c *Client) BaseClient() *Client { return c }

// IsRetryableError determines if error is retryable (network errors, timeouts, etc.)
func (client *Client) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var policy interface{ Retryable() bool }
	if errors.As(err, &policy) && !policy.Retryable() {
		return false
	}
	errStr := err.Error()
	// Network errors, timeouts, EOF, etc. can be retried
	for _, retryable := range client.Cfg.RetryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}
	return false
}

// ReportUsage fires TokenUsageCallback with the given token counts, tagged
// with this client's provider, model, base URL and usage owner.
// No-op if usage is nil or callback is unset.
func (client *Client) ReportUsage(usage *TokenUsage) {
	if usage == nil || TokenUsageCallback == nil || usage.TotalTokens <= 0 {
		return
	}
	TokenUsageCallback(TokenUsage{
		Provider:           client.Provider,
		Model:              client.Model,
		BaseURL:            client.BaseURL,
		Owner:              client.UsageOwner,
		PromptTokens:       usage.PromptTokens,
		CachedPromptTokens: usage.CachedPromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		TotalTokens:        usage.TotalTokens,
	})
}
