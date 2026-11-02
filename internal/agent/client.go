package agent

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

	// retryableErrors are failures of the connection, for what has no status to go by. An
	// answer with a status is judged by that: see StatusError.
	retryableErrors = []string{
		"EOF",
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"no such host",
		"stream error",   // HTTP/2 stream error
		"INTERNAL_ERROR", // Server internal error
		"upstream_empty_output",
	}

	// CallCallback is called after each request to a model, failed ones included.
	CallCallback func(report CallReport)
)

// TokenUsage is the token counts of one response.
type TokenUsage struct {
	PromptTokens       int // includes CachedPromptTokens
	CachedPromptTokens int
	CompletionTokens   int
	TotalTokens        int
}

// CallReport describes one request to a model, whether it worked or not.
type CallReport struct {
	Provider       string
	Model          string
	BaseURL        string
	Feature        string // BaseClient.UsageFeature of the reporting client
	SourceID       string // BaseClient.UsageSource of the reporting client (e.g. trader ID)
	ConversationID string // from WithConversation; empty outside a conversation
	TokenUsage
	Duration     time.Duration
	FinishReason string
	Error        string // empty when the call worked; never holds the API key
}

// BaseClient model API configuration
type BaseClient struct {
	Provider   string
	APIKey     string
	BaseURL    string
	Model      string
	UseFullURL bool // Whether to use full URL (without appending /chat/completions)
	MaxTokens  int  // Maximum tokens for model response

	HTTPClient *http.Client // Exported for sub-packages
	Log        Logger       // Exported for sub-packages
	Cfg        *Config      // Exported for sub-packages

	// UsageFeature and UsageSource tag the calls this client reports, so cost
	// records can be attributed: see TagUsage.
	UsageFeature string
	UsageSource  string

	// Hooks are used to implement dynamic dispatch (polymorphism)
	// When provider.DeepSeekClient embeds BaseClient, Hooks point to DeepSeekClient
	// This way methods called in Call() are automatically dispatched to the overridden version
	Hooks ClientHooks
}

// NewClient creates client (supports options pattern)
//
// Usage examples:
//
//	// Basic usage
//	client := agent.NewClient()
//
//	// Custom logger
//	client := agent.NewClient(agent.WithLogger(customLogger))
//
//	// Custom timeout
//	client := agent.NewClient(agent.WithTimeout(60*time.Second))
//
//	// Combine multiple options
//	client := agent.NewClient(
//	    agent.WithDeepSeekConfig("sk-xxx"),
//	    agent.WithLogger(customLogger),
//	    agent.WithTimeout(60*time.Second),
//	)
func NewClient(opts ...ClientOption) Client {
	// 1. Create default config
	cfg := DefaultConfig()

	// 2. Apply user options
	for _, opt := range opts {
		opt(cfg)
	}

	// 3. Create client instance
	client := &BaseClient{
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
func (client *BaseClient) SetAPIKey(apiKey, apiURL, customModel string) {
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

func (client *BaseClient) SetTimeout(timeout time.Duration) {
	client.HTTPClient.Timeout = timeout
}

// CallWithMessages template method - fixed retry flow (cannot be overridden)
func (client *BaseClient) CallWithMessages(systemPrompt, userPrompt string) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("model API key not set, please call SetAPIKey first")
	}

	return retrying(client, context.Background(), func() (string, error) {
		return client.Hooks.Call(systemPrompt, userPrompt)
	})
}

func (client *BaseClient) SetAuthHeader(reqHeader http.Header) {
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

func (client *BaseClient) BuildRequestBody(systemPrompt, userPrompt string) map[string]any {
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
func (client *BaseClient) MarshalRequestBody(requestBody map[string]any) ([]byte, error) {
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize request: %w", err)
	}
	return jsonData, nil
}

// ParseResponseFull parses the OpenAI-format response body and returns both
// the text content and any tool calls.
func (client *BaseClient) ParseResponseFull(body []byte) (*Response, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content          string     `json:"content"`
				ReasoningContent string     `json:"reasoning_content"`
				ToolCalls        []ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage openAIUsage `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("API returned empty response")
	}

	msg := result.Choices[0].Message
	return &Response{
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        msg.ToolCalls,
		Usage:            result.Usage.tokenUsage(),
		FinishReason:     result.Choices[0].FinishReason,
	}, nil
}

func (client *BaseClient) BuildUrl() string {
	if client.UseFullURL {
		return client.BaseURL
	}
	return strings.TrimRight(client.BaseURL, "/") + "/chat/completions"
}

func (client *BaseClient) BuildRequest(url string, jsonData []byte) (*http.Request, error) {
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

func (client *BaseClient) buildHTTPRequestWithContext(ctx context.Context, url string, jsonData []byte) (*http.Request, error) {
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

// Call single model API call (fixed flow, cannot be overridden)
func (client *BaseClient) Call(systemPrompt, userPrompt string) (string, error) {
	response, err := client.send(context.Background(), client.Hooks.BuildRequestBody(systemPrompt, userPrompt))
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// send makes one request to the model and reports it, whether it worked or not.
func (client *BaseClient) send(ctx context.Context, requestBody map[string]any) (*Response, error) {
	started := time.Now()
	response, err := client.exchange(ctx, requestBody)
	var usage *TokenUsage
	finishReason := ""
	if response != nil {
		usage, finishReason = response.Usage, response.FinishReason
	}
	client.report(ctx, started, usage, finishReason, err)
	return response, err
}

func (client *BaseClient) exchange(ctx context.Context, requestBody map[string]any) (*Response, error) {
	jsonData, err := client.Hooks.MarshalRequestBody(requestBody)
	if err != nil {
		return nil, err
	}

	url := client.Hooks.BuildUrl()
	client.Log.Infof("📡 [agent %s] Request URL: %s", client.String(), url)
	httpReq, err := client.buildHTTPRequestWithContext(ctx, url, jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newStatusError(resp, body)
	}

	response, err := client.Hooks.ParseResponseFull(body)
	if err != nil {
		return nil, fmt.Errorf("fail to parse model server response: %w", err)
	}
	return response, nil
}

func (client *BaseClient) String() string {
	return fmt.Sprintf("[Provider: %s, Model: %s]",
		client.Provider, client.Model)
}

// BaseClient returns the underlying *BaseClient (satisfies ClientEmbedder interface).
func (c *BaseClient) Base() *BaseClient { return c }

// IsRetryableError determines if error is retryable (network errors, timeouts, etc.)
func (client *BaseClient) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var broken streamBroken
	if errors.As(err, &broken) {
		return false
	}
	var refused *StatusError
	if errors.As(err, &refused) {
		return refused.Retryable()
	}
	// A call that ran out of time, or was stopped, has already had its chance.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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

// report fires CallCallback for one request, tagged with this client's provider,
// model, base URL, feature and source.
func (client *BaseClient) report(ctx context.Context, started time.Time, usage *TokenUsage, finishReason string, err error) {
	if CallCallback == nil {
		return
	}
	report := CallReport{
		Provider:       client.Provider,
		Model:          client.Model,
		BaseURL:        client.BaseURL,
		Feature:        client.UsageFeature,
		SourceID:       client.UsageSource,
		ConversationID: conversationID(ctx),
		Duration:       time.Since(started),
		FinishReason:   finishReason,
	}
	if usage != nil {
		report.TokenUsage = *usage
	}
	if err != nil {
		report.Error = client.safeError(err)
	}
	CallCallback(report)
}

// maxCallError bounds the error kept with a call: a provider's error body can be long.
const maxCallError = 500

// safeError is an error as it can be stored: without the API key, which a provider
// may repeat back, and without bytes PostgreSQL refuses in text.
func (client *BaseClient) safeError(err error) string {
	message := err.Error()
	if client.APIKey != "" {
		message = strings.ReplaceAll(message, client.APIKey, "***")
	}
	message = strings.ToValidUTF8(strings.ReplaceAll(message, "\x00", ""), "")
	if runes := []rune(message); len(runes) > maxCallError {
		message = string(runes[:maxCallError]) + "…"
	}
	return message
}
