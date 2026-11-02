package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// notChatModel matches what providers list beside their chat models, which a chat cannot call.
var notChatModel = regexp.MustCompile(`(?i)embed|whisper|tts|transcri|dall-e|image|moderation|realtime|audio|sora|veo|aqa|rerank|speech`)

// ListModels asks a provider which models the client's key can chat with, from the /models
// endpoint beside its chat endpoint. A client on a full URL has no such endpoint to find, so
// the answer is empty. It costs no tokens.
func ListModels(ctx context.Context, client Client) ([]string, error) {
	embedder, ok := client.(ClientEmbedder)
	if !ok {
		return nil, errors.New("this client cannot list models")
	}
	base := embedder.Base()
	if base.UseFullURL || strings.HasSuffix(base.BaseURL, "#") {
		return nil, nil
	}
	url := strings.TrimRight(base.BaseURL, "/") + "/models"
	if base.Provider == ProviderClaude {
		// It answers 20 at a time unless asked for more.
		url += "?limit=1000"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to build request: %w", err)
	}
	base.Hooks.SetAuthHeader(req.Header)
	resp, err := base.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fail to send request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("fail to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned error (status %d): %s", resp.StatusCode, body)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("fail to parse response: %w", err)
	}
	names := make([]string, 0, len(list.Data))
	for _, model := range list.Data {
		// Gemini names its models as resources on some of its endpoints.
		name := strings.TrimPrefix(model.ID, "models/")
		if name != "" && !notChatModel.MatchString(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}
