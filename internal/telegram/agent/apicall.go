package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"vergex/internal/logger"
)

// apiClient calls this server's own REST API with the bot user's JWT. Skills
// use it with fixed paths, so the LLM never chooses an endpoint and every
// request goes through the same validation and risk caps as the web UI.
type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// apiError is a non-2xx API response.
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.Status, truncate(e.Body, 500))
}

func newAPIClient(port int, token string) *apiClient {
	return &apiClient{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// do sends one request. When out is non-nil the JSON response is decoded into it.
func (c *apiClient) do(method, path string, body, out any) error {
	raw, err := c.raw(method, path, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unexpected response from %s %s: %w", method, path, err)
	}
	return nil
}

// raw sends one request and returns the response body.
func (c *apiClient) raw(method, path string, body any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	logger.Infof("Agent skill call: %s %s -> %d", method, path, resp.StatusCode)
	if resp.StatusCode >= 400 {
		return nil, &apiError{Status: resp.StatusCode, Body: string(data)}
	}
	if len(data) == 0 {
		data = []byte("{}")
	}
	return data, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
