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
	"unicode/utf8"
)

// maxMessageRunes keeps a message under Telegram's 4096 character limit.
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

// splitMessage cuts text at line breaks into parts Telegram will accept.
func splitMessage(text string) []string {
	var parts []string
	var part strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if part.Len() > 0 && utf8.RuneCountInString(part.String())+utf8.RuneCountInString(line)+1 > maxMessageRunes {
			parts = append(parts, part.String())
			part.Reset()
		}
		if part.Len() > 0 {
			part.WriteByte('\n')
		}
		part.WriteString(line)
	}
	if part.Len() > 0 {
		parts = append(parts, part.String())
	}
	return parts
}
