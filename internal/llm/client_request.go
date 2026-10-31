package llm

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// ============================================================
// Builder Pattern API (Advanced Features)
// ============================================================

// CallWithRequest calls AI API using Request object (supports advanced features)
func (client *Client) CallWithRequest(req *Request) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("AI API key not set, please call SetAPIKey first")
	}

	// If Model is not set in Request, use Client's Model
	if req.Model == "" {
		req.Model = client.Model
	}

	// Fixed retry flow
	var lastErr error
	maxRetries := client.Cfg.MaxRetries

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			client.Log.Warnf("⚠️  AI API call failed, retrying (%d/%d)...", attempt, maxRetries)
		}

		// Call single request
		result, err := client.callWithRequest(req)
		if err == nil {
			if attempt > 1 {
				client.Log.Infof("✓ AI API retry succeeded")
			}
			return result, nil
		}

		lastErr = err
		// Check if error is retryable
		if !client.Hooks.IsRetryableError(err) {
			return "", err
		}

		// Wait before retry
		if attempt < maxRetries {
			waitTime := client.Cfg.RetryWaitBase * time.Duration(attempt)
			client.Log.Infof("⏳ Waiting %v before retry...", waitTime)
			if err := sleepWithContext(contextFromRequest(req), waitTime); err != nil {
				return "", err
			}
		}
	}

	return "", fmt.Errorf("still failed after %d retries: %w", maxRetries, lastErr)
}

// CallWithRequestFull calls the AI API and returns both text content and tool calls.
func (client *Client) CallWithRequestFull(req *Request) (*LLMResponse, error) {
	if client.APIKey == "" {
		return nil, fmt.Errorf("AI API key not set, please call SetAPIKey first")
	}
	if req.Model == "" {
		req.Model = client.Model
	}

	var lastErr error
	maxRetries := client.Cfg.MaxRetries
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			client.Log.Warnf("⚠️  AI API call failed, retrying (%d/%d)...", attempt, maxRetries)
		}
		result, err := client.callWithRequestFull(req)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !client.Hooks.IsRetryableError(err) {
			return nil, err
		}
		if attempt < maxRetries {
			waitTime := client.Cfg.RetryWaitBase * time.Duration(attempt)
			if err := sleepWithContext(contextFromRequest(req), waitTime); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("still failed after %d retries: %w", maxRetries, lastErr)
}

// callWithRequestFull single call that returns LLMResponse (content + tool calls).
func (client *Client) callWithRequestFull(req *Request) (*LLMResponse, error) {
	client.Log.Infof("📡 [%s] Request AI Server (full): BaseURL: %s", client.String(), client.BaseURL)

	requestBody := client.Hooks.BuildRequestBodyFromRequest(req)
	jsonData, err := client.Hooks.MarshalRequestBody(requestBody)
	if err != nil {
		return nil, err
	}

	url := client.Hooks.BuildUrl()
	httpReq, err := client.buildHTTPRequestWithContext(contextFromRequest(req), url, jsonData)
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
		return nil, fmt.Errorf("API returned error (status %d): %s", resp.StatusCode, string(body))
	}

	return client.Hooks.ParseResponseFull(body)
}

// callWithRequest single AI API call (using Request object)
func (client *Client) callWithRequest(req *Request) (string, error) {
	// Print current AI configuration
	client.Log.Infof("📡 [%s] Request AI Server with Builder: BaseURL: %s", client.String(), client.BaseURL)
	client.Log.Debugf("[%s] Messages count: %d", client.String(), len(req.Messages))

	requestBody := client.Hooks.BuildRequestBodyFromRequest(req)

	jsonData, err := client.Hooks.MarshalRequestBody(requestBody)
	if err != nil {
		return "", err
	}

	url := client.Hooks.BuildUrl()
	client.Log.Infof("📡 [LLM %s] Request URL: %s", client.String(), url)

	httpReq, err := client.buildHTTPRequestWithContext(contextFromRequest(req), url, jsonData)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned error (status %d): %s", resp.StatusCode, string(body))
	}

	result, err := client.Hooks.ParseResponse(body)
	if err != nil {
		return "", fmt.Errorf("fail to parse AI server response: %w", err)
	}

	return result, nil
}

// BuildRequestBodyFromRequest builds request body from Request object
func (client *Client) BuildRequestBodyFromRequest(req *Request) map[string]any {
	// Convert Message to API format — must use map[string]any to support
	// tool-call messages (tool_calls, tool_call_id fields).
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		m := map[string]any{"role": msg.Role}
		if len(msg.ToolCalls) > 0 {
			// Assistant message that contains tool invocations.
			// content must be null/omitted for OpenAI compatibility.
			m["tool_calls"] = msg.ToolCalls
		} else if msg.ToolCallID != "" {
			// Tool result message (role="tool").
			m["tool_call_id"] = msg.ToolCallID
			m["content"] = msg.Content
		} else {
			m["content"] = msg.Content
		}
		// DeepSeek thinking models require reasoning_content to be echoed back
		// in multi-turn conversations when present in assistant messages.
		if msg.ReasoningContent != "" {
			m["reasoning_content"] = msg.ReasoningContent
		}
		messages = append(messages, m)
	}

	// Guard: truncate messages if they would exceed the model's context window
	maxOut := client.MaxTokens
	if req.MaxTokens != nil {
		maxOut = *req.MaxTokens
	}
	if client.Cfg.MaxContext > 0 {
		truncated, removed := truncateMessagesAny(messages, client.Cfg.MaxContext, maxOut)
		if removed > 0 {
			client.Log.Warnf("⚠️  [%s] Context guard: truncated %d oldest messages to fit within %d token limit",
				client.String(), removed, client.Cfg.MaxContext)
			messages = truncated
		}
	}

	// Build basic request body
	requestBody := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
	}

	// Add optional parameters (only add non-nil parameters)
	if modelSupportsCustomTemperature(req.Model) {
		if req.Temperature != nil {
			requestBody["temperature"] = *req.Temperature
		} else {
			// If not set in Request, use Client's configuration
			requestBody["temperature"] = client.Cfg.Temperature
		}
	}

	// OpenAI newer models use max_completion_tokens instead of max_tokens
	tokenKey := "max_tokens"
	if client.Provider == ProviderOpenAI {
		tokenKey = "max_completion_tokens"
	}
	if req.MaxTokens != nil {
		requestBody[tokenKey] = *req.MaxTokens
	} else {
		// If not set in Request, use Client's MaxTokens
		requestBody[tokenKey] = client.MaxTokens
	}

	if req.TopP != nil {
		requestBody["top_p"] = *req.TopP
	}

	if req.FrequencyPenalty != nil {
		requestBody["frequency_penalty"] = *req.FrequencyPenalty
	}

	if req.PresencePenalty != nil {
		requestBody["presence_penalty"] = *req.PresencePenalty
	}

	if len(req.Stop) > 0 {
		requestBody["stop"] = req.Stop
	}

	if len(req.Tools) > 0 {
		requestBody["tools"] = req.Tools
	}

	if req.ToolChoice != "" {
		requestBody["tool_choice"] = req.ToolChoice
	}

	if req.Stream {
		requestBody["stream"] = true
	}

	return requestBody
}
