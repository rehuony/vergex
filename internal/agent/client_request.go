package agent

import (
	"fmt"
	"time"
)

// ============================================================
// Builder Pattern API (Advanced Features)
// ============================================================

// CallWithRequest calls model API using Request object (supports advanced features)
func (client *BaseClient) CallWithRequest(req *Request) (string, error) {
	if client.APIKey == "" {
		return "", fmt.Errorf("model API key not set, please call SetAPIKey first")
	}

	// If Model is not set in Request, use BaseClient's Model
	if req.Model == "" {
		req.Model = client.Model
	}

	// Fixed retry flow
	var lastErr error
	maxRetries := client.Cfg.MaxRetries

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			client.Log.Warnf("⚠️  model API call failed, retrying (%d/%d)...", attempt, maxRetries)
		}

		// Call single request
		result, err := client.callWithRequest(req)
		if err == nil {
			if attempt > 1 {
				client.Log.Infof("✓ model API retry succeeded")
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

// CallWithRequestFull calls the model API and returns both text content and tool calls.
func (client *BaseClient) CallWithRequestFull(req *Request) (*Response, error) {
	if client.APIKey == "" {
		return nil, fmt.Errorf("model API key not set, please call SetAPIKey first")
	}
	if req.Model == "" {
		req.Model = client.Model
	}

	var lastErr error
	maxRetries := client.Cfg.MaxRetries
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			client.Log.Warnf("⚠️  model API call failed, retrying (%d/%d)...", attempt, maxRetries)
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

// callWithRequestFull single call that returns Response (content + tool calls).
func (client *BaseClient) callWithRequestFull(req *Request) (*Response, error) {
	client.Log.Debugf("[%s] Messages count: %d", client.String(), len(req.Messages))
	return client.send(contextFromRequest(req), client.Hooks.BuildRequestBodyFromRequest(req))
}

// callWithRequest single model API call (using Request object)
func (client *BaseClient) callWithRequest(req *Request) (string, error) {
	response, err := client.callWithRequestFull(req)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// BuildRequestBodyFromRequest builds request body from Request object
func (client *BaseClient) BuildRequestBodyFromRequest(req *Request) map[string]any {
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
			// If not set in Request, use BaseClient's configuration
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
		// If not set in Request, use BaseClient's MaxTokens
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

	return requestBody
}
