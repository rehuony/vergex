package telegram

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

// bindCodeLife is how long a binding link works.
const bindCodeLife = 10 * time.Minute

// bindCode is the one code that can bind a chat. It lives in memory, so a restart only
// means asking Settings for a new link, and a new one replaces the last.
var bindCode struct {
	sync.Mutex
	value   string
	expires time.Time
	// language is the one Settings was in, for a chat that has not chosen one.
	language string
}

// bindCodeShape is what a code looks like, to tell one that failed from any other message.
var bindCodeShape = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ErrNoToken means there is no bot to bind a chat to yet.
var ErrNoToken = errors.New("save a bot token first")

// ErrAlreadyBound means a user holds the binding, and has to give it up in Settings first.
var ErrAlreadyBound = errors.New("a Telegram user is already bound: unbind them first")

// BindLink is what Settings gives the user to bind their chat: a link that opens the bot,
// and the same code to send by hand.
type BindLink struct {
	Code      string    `json:"code"`
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewBindLink starts a binding. Only the signed-in user gets here, which is what makes the
// code proof of who is at the other end of the chat. language is the one they are reading
// Settings in: zh or en, or empty for neither.
func NewBindLink(ctx context.Context, configs ConfigStore, language string, now time.Time) (*BindLink, error) {
	cfg, err := configs.Get()
	if err != nil || cfg.BotToken == "" {
		return nil, ErrNoToken
	}
	if cfg.ChatID > 0 {
		return nil, ErrAlreadyBound
	}
	username, err := GetMe(ctx, string(cfg.BotToken))
	if err != nil {
		return nil, err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	code := hex.EncodeToString(random)
	expires := now.Add(bindCodeLife)
	bindCode.Lock()
	bindCode.value, bindCode.expires, bindCode.language = code, expires, language
	bindCode.Unlock()
	return &BindLink{Code: code, Link: fmt.Sprintf("https://t.me/%s?start=%s", username, code), ExpiresAt: expires}, nil
}

// useBindCode reports whether code is the one in force, and uses it up.
func useBindCode(code string, now time.Time) bool {
	bindCode.Lock()
	defer bindCode.Unlock()
	if code == "" || bindCode.value == "" || now.After(bindCode.expires) ||
		subtle.ConstantTimeCompare([]byte(code), []byte(bindCode.value)) != 1 {
		return false
	}
	bindCode.value = ""
	return true
}

// SaveToken checks a bot token with Telegram and saves it. A binding belongs to one bot, so
// a different bot starts unbound.
func SaveToken(ctx context.Context, configs ConfigStore, token, modelID string) error {
	if _, err := GetMe(ctx, token); err != nil {
		return err
	}
	cfg, err := configs.Get()
	changed := err == nil && cfg.ChatID != 0 && string(cfg.BotToken) != token
	if err := configs.Save(token, modelID); err != nil {
		return err
	}
	if changed {
		return configs.Unbind()
	}
	return nil
}

// ClearToken forgets the saved bot, for setup to start over, and the code asked for it goes
// too. While someone is bound it is refused, as a new link is: unbinding comes first.
func ClearToken(configs ConfigStore) error {
	bound, err := configs.IsBound()
	if err != nil {
		return err
	}
	if bound {
		return ErrAlreadyBound
	}
	if err := configs.ClearToken(); err != nil {
		return err
	}
	bindCode.Lock()
	bindCode.value = ""
	bindCode.Unlock()
	return nil
}

// bindLanguage is the language the last code was asked for in.
func bindLanguage() string {
	bindCode.Lock()
	defer bindCode.Unlock()
	return bindCode.language
}
