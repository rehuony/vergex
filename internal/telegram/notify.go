package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var notifyClient = &http.Client{Timeout: 15 * time.Second}

// ErrNotBound means no bot token or no chat is set up to send to.
var ErrNotBound = errors.New("Telegram is not connected to a chat")

// Notify sends a message to the bound chat, in as many parts as it takes. It does not depend
// on the running bot, so it works while the bot is reloading.
func Notify(ctx context.Context, configs ConfigStore, m *message) error {
	for _, part := range m.parts() {
		if err := notifyPart(ctx, configs, part, m.silent); err != nil {
			return err
		}
	}
	return nil
}

// notifyPart sends one part to the bound chat, as HTML and failing that as plain text.
func notifyPart(ctx context.Context, configs ConfigStore, part messagePart, silent bool) error {
	cfg, err := configs.Get()
	if err != nil || cfg.BotToken == "" || cfg.ChatID == 0 {
		return ErrNotBound
	}
	err = sendMessage(ctx, string(cfg.BotToken), cfg.ChatID, part.HTML, "HTML", silent)
	var refused *refusal
	if errors.As(err, &refused) && refused.status == http.StatusBadRequest {
		err = sendMessage(ctx, string(cfg.BotToken), cfg.ChatID, part.Plain, "", silent)
	}
	return err
}

// refusal is Telegram saying no to a message.
type refusal struct {
	status      int
	description string
}

func (r *refusal) Error() string { return "Telegram refused the message: " + r.description }

func sendMessage(ctx context.Context, token string, chatID int64, text, parseMode string, silent bool) error {
	request := map[string]any{"chat_id": chatID, "text": text, "disable_notification": silent, "link_preview_options": map[string]any{"is_disabled": true}}
	if parseMode != "" {
		request["parse_mode"] = parseMode
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(apiEndpoint, token, "sendMessage"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := notifyClient.Do(req)
	if err != nil {
		// The URL holds the bot token, so the transport error is not passed on.
		return errors.New("could not reach Telegram")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var reply struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&reply)
		return &refusal{status: resp.StatusCode, description: reply.Description}
	}
	return nil
}

// ErrBadToken means Telegram does not recognise a bot token.
var ErrBadToken = errors.New("Telegram does not recognise this bot token")

// GetMe asks Telegram which bot a token belongs to and returns its username. The token is
// in the URL, so a transport error is not passed on.
func GetMe(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(apiEndpoint, token, "getMe"), nil)
	if err != nil {
		return "", errors.New("could not reach Telegram")
	}
	resp, err := notifyClient.Do(req)
	if err != nil {
		return "", errors.New("could not reach Telegram")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		return "", ErrBadToken
	}
	var reply struct {
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil || reply.Result.Username == "" {
		return "", errors.New("could not reach Telegram")
	}
	return reply.Result.Username, nil
}

// ClearMenu takes the command menu off a chat that is no longer bound.
func ClearMenu(ctx context.Context, token string, chatID int64) {
	body, err := json.Marshal(map[string]any{"scope": map[string]any{"type": "chat", "chat_id": chatID}})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(apiEndpoint, token, "deleteMyCommands"), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if resp, err := notifyClient.Do(req); err == nil {
		resp.Body.Close()
	}
}
