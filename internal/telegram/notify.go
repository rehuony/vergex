package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// maxMessageRunes keeps a message under Telegram's limit of 4096 UTF-16 units.
const maxMessageRunes = 3500

var notifyClient = &http.Client{Timeout: 15 * time.Second}

// ErrNotBound means no bot token or no chat is set up to send to.
var ErrNotBound = errors.New("Telegram is not connected to a chat")

// Notify sends plain text to the bound chat, split when it is too long. It does
// not depend on the running bot, so it works while the bot is reloading.
func Notify(ctx context.Context, configs ConfigStore, text string) error {
	cfg, err := configs.Get()
	if err != nil || cfg.BotToken == "" || cfg.ChatID == 0 {
		return ErrNotBound
	}
	for _, part := range splitMessage(text) {
		if err := sendMessage(ctx, string(cfg.BotToken), cfg.ChatID, part); err != nil {
			return err
		}
	}
	return nil
}

func sendMessage(ctx context.Context, token string, chatID int64, text string) error {
	body, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
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
		return fmt.Errorf("Telegram refused the message: %s", reply.Description)
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

// splitMessage cuts text at line breaks into parts Telegram will accept, and cuts a line
// too long for one message: a single long error must not stop everything behind it.
func splitMessage(text string) []string {
	var parts []string
	var part strings.Builder
	for _, line := range strings.Split(text, "\n") {
		for _, piece := range splitLine(line) {
			if part.Len() > 0 && utf16Len(part.String())+utf16Len(piece)+1 > maxMessageRunes {
				parts = append(parts, part.String())
				part.Reset()
			}
			if part.Len() > 0 {
				part.WriteByte('\n')
			}
			part.WriteString(piece)
		}
	}
	if part.Len() > 0 {
		parts = append(parts, part.String())
	}
	return parts
}

// splitLine cuts a line into pieces that each fit a message.
func splitLine(line string) []string {
	if utf16Len(line) <= maxMessageRunes {
		return []string{line}
	}
	var pieces []string
	var piece []rune
	size := 0
	for _, r := range line {
		width := utf16Len(string(r))
		if size+width > maxMessageRunes {
			pieces = append(pieces, string(piece))
			piece, size = piece[:0], 0
		}
		piece = append(piece, r)
		size += width
	}
	return append(pieces, string(piece))
}
