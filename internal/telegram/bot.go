package telegram

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"vergex/internal/agent"
	_ "vergex/internal/agent/provider"
	"vergex/internal/config"
	"vergex/internal/logger"
	"vergex/internal/service"
	telegramagent "vergex/internal/telegram/agent"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// apiEndpoint is the Telegram Bot API URL pattern; tests point it at a fake.
var apiEndpoint = tgbotapi.APIEndpoint

// retryDelay is how long Start waits after the bot failed to start or lost its
// update stream (a wrong token, no network) before trying again, unless the
// settings change first.
var retryDelay = time.Minute

// Start runs the Telegram bot for the life of the process. A signal on reloadCh
// (sent when its token, model or binding is saved) restarts it with the
// settings in the database; with no token it waits for one. Must be called as
// a goroutine from main.go.
func Start(cfg *config.Config, st *Stores, reloadCh <-chan struct{}) {
	for {
		token := resolveToken(cfg, st)
		if token == "" {
			logger.Info("Telegram bot disabled (no token configured), waiting for reload signal...")
			service.Report(service.Telegram, service.Off, "")
			<-reloadCh
			continue
		}

		if err := runBot(token, cfg, st, reloadCh); err != nil {
			logger.Errorf("Telegram bot stopped: %v; retrying in %v or when its settings change", err, retryDelay)
			service.Report(service.Telegram, service.Failed, err.Error())
			select {
			case <-reloadCh:
			case <-time.After(retryDelay):
			}
			continue
		}
		logger.Info("Reloading Telegram bot with the saved settings...")
	}
}

// resolveToken returns the bot token from DB (configured via Web UI).
func resolveToken(cfg *config.Config, st *Stores) string {
	dbCfg, err := st.Telegram.Get()
	if err == nil && dbCfg.BotToken != "" {
		return string(dbCfg.BotToken)
	}
	return ""
}

// runBot serves the bot until a reload signal (nil) or until it cannot start or
// loses its update stream (an error).
func runBot(token string, cfg *config.Config, st *Stores, reloadCh <-chan struct{}) error {
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiEndpoint)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	logger.Infof("Telegram bot @%s started", bot.Self.UserName)
	service.Report(service.Telegram, service.Running, "")

	// Allowed chat ID: read from DB binding (0 = unbound, first /start will bind).
	allowedChatID := int64(0)
	if id, err := st.Telegram.GetBoundChatID(); err == nil && id != 0 {
		allowedChatID = id
	}

	// botUserID / botToken / agents are resolved lazily and refresh when user registers.
	var (
		botUserID    string
		botUserEmail string
		botToken     string
		agents       *telegramagent.Manager
	)

	resolveBotUser := func() bool {
		users, err := st.Users.GetAll()
		if err != nil || len(users) == 0 {
			return false
		}
		u := users[0]
		if u.ID == botUserID {
			return true
		}
		newToken, err := telegramagent.GenerateBotToken(u.ID)
		if err != nil {
			logger.Errorf("Failed to generate bot JWT for user %s: %v", u.ID, err)
			return false
		}
		prev := botUserID
		botUserID = u.ID
		botUserEmail = u.Email
		botToken = newToken
		agents = telegramagent.NewManager(cfg.APIServerPort, botToken, botUserEmail, botUserID,
			func() agent.Client { return newAgentClient(st, botUserID) },
			&chatHistory{conversations: st.Conversations, userID: botUserID},
		)
		if prev == "" {
			logger.Infof("Bot: resolved user %s (%s)", botUserID, botUserEmail)
		} else {
			logger.Infof("Bot: user changed → %s (%s)", botUserID, botUserEmail)
		}
		return true
	}
	resolveBotUser()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// awaitingLang is set only when the user explicitly runs /lang.
	awaitingLang := false

	for {
		var update tgbotapi.Update
		select {
		case <-reloadCh:
			bot.StopReceivingUpdates()
			return nil
		case next, ok := <-updates:
			if !ok {
				return errors.New("update stream closed")
			}
			update = next
		}
		if update.Message == nil {
			continue
		}
		chatID := update.Message.Chat.ID
		text := strings.TrimSpace(update.Message.Text)

		// ── Language selection (triggered only by /lang) ──────────────────────
		if awaitingLang && chatID == allowedChatID {
			if lang := parseLangChoice(text); lang != "" {
				awaitingLang = false
				st.Telegram.SetLanguage(lang) //nolint:errcheck
				sendMarkdownMsg(bot, chatID, statusMsg(st, botUserID, cfg.APIServerPort, lang))
			} else {
				sendMarkdownMsg(bot, chatID, langMenuMsg())
			}
			continue
		}

		// ── /start ────────────────────────────────────────────────────────────
		if text == "/start" {
			resolveBotUser()
			if botUserID == "" {
				sendMsg(bot, chatID,
					"No account found.\nOpen the web dashboard to register, then send /start.")
				continue
			}
			if allowedChatID == 0 {
				username := update.Message.From.UserName
				if err := st.Telegram.BindUser(chatID, "@"+username); err != nil {
					logger.Errorf("Failed to bind Telegram user: %v", err)
					sendMsg(bot, chatID, "Binding failed. Please try again.")
					continue
				}
				allowedChatID = chatID
				logger.Infof("Telegram bound to @%s (chatID: %d)", username, chatID)
			} else if chatID != allowedChatID {
				sendMsg(bot, chatID, "This bot is already bound to another account.")
				continue
			} else {
				agents.Reset(chatID)
			}
			lang := st.Telegram.GetLanguage()
			sendMarkdownMsg(bot, chatID, statusMsg(st, botUserID, cfg.APIServerPort, lang))
			continue
		}

		// ── /lang ─────────────────────────────────────────────────────────────
		if text == "/lang" {
			awaitingLang = true
			sendMarkdownMsg(bot, chatID, langMenuMsg())
			continue
		}

		// ── /help ─────────────────────────────────────────────────────────────
		if text == "/help" {
			lang := st.Telegram.GetLanguage()
			sendMarkdownMsg(bot, chatID, helpMsg(lang))
			continue
		}

		// ── Access control ────────────────────────────────────────────────────
		if allowedChatID != 0 && chatID != allowedChatID {
			sendMsg(bot, chatID, "Unauthorized.")
			continue
		}
		if allowedChatID == 0 {
			sendMsg(bot, chatID, "Send /start first.")
			continue
		}
		if text == "" {
			continue
		}

		// ── Refresh user before every model call ────────────────────────────────
		resolveBotUser()
		if botUserID == "" {
			sendMsg(bot, chatID, "No account found. Open the web dashboard to register.")
			continue
		}

		lang := st.Telegram.GetLanguage()

		// ── Guard: show status if not ready for trading ───────────────────────
		if newAgentClient(st, botUserID) == nil {
			sendMarkdownMsg(bot, chatID, statusMsg(st, botUserID, cfg.APIServerPort, lang))
			continue
		}

		// ── agent ─────────────────────────────────────────────────────────
		go func(chatID int64, text string) {
			sent, err := bot.Send(tgbotapi.NewMessage(chatID, "⏳"))
			placeholderID := 0
			if err == nil {
				placeholderID = sent.MessageID
			}

			var (
				mu       sync.Mutex
				lastEdit time.Time
			)
			onChunk := func(accumulated string) {
				if placeholderID == 0 {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if accumulated != "⏳" && time.Since(lastEdit) < time.Second {
					return
				}
				lastEdit = time.Now()
				edit := tgbotapi.NewEditMessageText(chatID, placeholderID, accumulated)
				bot.Send(edit) //nolint:errcheck
			}

			reply := agents.Run(chatID, text, onChunk)

			if placeholderID != 0 {
				edit := tgbotapi.NewEditMessageText(chatID, placeholderID, reply)
				edit.ParseMode = "Markdown"
				if _, err := bot.Send(edit); err != nil {
					edit2 := tgbotapi.NewEditMessageText(chatID, placeholderID, reply)
					bot.Send(edit2) //nolint:errcheck
				}
			} else {
				msg := tgbotapi.NewMessage(chatID, reply)
				msg.ParseMode = "Markdown"
				if _, err := bot.Send(msg); err != nil {
					msg.ParseMode = ""
					bot.Send(msg) //nolint:errcheck
				}
			}
		}(chatID, text)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func sendMsg(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	bot.Send(msg) //nolint:errcheck
}

func sendMarkdownMsg(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		plain := tgbotapi.NewMessage(chatID, text)
		bot.Send(plain) //nolint:errcheck
	}
}

// ── model client ───────────────────────────────────────────────────────────────

func newAgentClient(st *Stores, userID string) agent.Client {
	// 1. Prefer the model explicitly configured for Telegram (Settings → Telegram → Model)
	if tgCfg, err := st.Telegram.Get(); err == nil && tgCfg.AgentModelID != "" {
		if model, err := st.Models.Get(userID, tgCfg.AgentModelID); err == nil && model.Enabled {
			apiKey := string(model.APIKey)
			if apiKey != "" {
				client := clientForProvider(model.Provider)
				client.SetAPIKey(apiKey, model.CustomAPIURL, model.CustomModelName)
				logger.Infof("Telegram agent: provider=%s user=%s", model.Provider, userID)
				return client
			}
		}
	}

	// 2. Fall back to first enabled model
	if model, err := st.Models.GetDefault(userID); err == nil {
		apiKey := string(model.APIKey)
		if apiKey != "" {
			client := clientForProvider(model.Provider)
			client.SetAPIKey(apiKey, model.CustomAPIURL, model.CustomModelName)
			logger.Infof("Telegram agent: provider=%s user=%s", model.Provider, userID)
			return client
		}
	}

	// 3. Environment variable fallback
	for _, pair := range []struct{ provider, key, url string }{
		{"deepseek", os.Getenv("DEEPSEEK_API_KEY"), agent.DefaultDeepSeekBaseURL},
		{"openai", os.Getenv("OPENAI_API_KEY"), ""},
		{"claude", os.Getenv("ANTHROPIC_API_KEY"), ""},
	} {
		if pair.key != "" {
			client := clientForProvider(pair.provider)
			client.SetAPIKey(pair.key, pair.url, "")
			return client
		}
	}
	return nil
}

func clientForProvider(provider string) agent.Client {
	client := agent.NewClientByProvider(provider)
	if client == nil {
		client = agent.NewClientByProvider("deepseek")
	}
	agent.TagUsage(client, agent.FeatureTelegram, "")
	return client
}

// ── Status message ────────────────────────────────────────────────────────────

// statusMsg is the single entry-point message shown after /start.
// It checks what's configured and shows either a setup prompt or the ready state.
func statusMsg(st *Stores, userID string, apiPort int, lang string) string {
	webURL := "http://localhost:8000"

	// Determine what's missing.
	hasModel := false
	if _, err := st.Models.GetDefault(userID); err == nil {
		hasModel = true
	}

	hasAccount := false
	if accounts, err := st.Accounts.List(userID); err == nil {
		hasAccount = len(accounts) > 0
	}

	if !hasModel || !hasAccount {
		missing := ""
		if lang == "zh" {
			if !hasModel {
				missing += "\n❌ Model → Settings → Models → Add"
			}
			if !hasAccount {
				missing += "\n❌ Account → Settings → Accounts → Add"
			}
			return "⚙️ *Setup required*\n\nOpen the web dashboard to complete setup:\n→ " + webURL + "\n" + missing + "\n\nSend /start when done."
		}
		if !hasModel {
			missing += "\n❌ Model → Settings → Models → Add"
		}
		if !hasAccount {
			missing += "\n❌ Account → Settings → Accounts → Add"
		}
		return "⚙️ *Setup required*\n\nOpen the web dashboard to complete setup:\n→ " + webURL + "\n" + missing + "\n\nSend /start when done."
	}

	// All configured — show ready state.
	if lang == "zh" {
		return `✅ *Vergex is ready!*

Just tell me what you want:

📊 "Show my positions"
💰 "What's my balance?"
🤖 "Create a BTC trend trader and start it"
⏹ "Stop all traders"

/help for more · /lang to change language`
	}
	return `✅ *Vergex is ready!*

Just tell me what you want:

📊 "Show my positions"
💰 "What's my balance?"
🤖 "Create a BTC trend trader and start it"
⏹ "Stop all traders"

/help for more · /lang to change language`
}

// ── Language ──────────────────────────────────────────────────────────────────

func langMenuMsg() string {
	return "🌐 *Choose your language*\n\n1 — English\n2 — Chinese\n\nReply with 1 or 2"
}

func parseLangChoice(text string) string {
	switch strings.TrimSpace(text) {
	case "1", "en", "EN", "English", "english":
		return "en"
	case "2", "zh", "ZH", "chinese", "Chinese":
		return "zh"
	}
	return ""
}

// ── Help ──────────────────────────────────────────────────────────────────────

func helpMsg(lang string) string {
	if lang == "zh" {
		return `*Vergex Help*

*Query*
• "Show my positions"
• "What's my balance?"
• "List my traders"

*Create & start*
• "Create a BTC trend trader and start it"
• "A careful trader, BTC and ETH only"

*Control*
• "Start trader"
• "Pause trader"
• "Stop all trading"

*Commands*
/start — refresh status
/lang  — change language
/help  — show this`
	}
	return `*Vergex Help*

*Query*
• "Show my positions"
• "What's my balance?"
• "List my traders"

*Create & start*
• "Create a BTC trend trader and start it"
• "A careful trader, BTC and ETH only"

*Control*
• "Start trader"
• "Stop trader"
• "Stop all trading"

*Commands*
/start — refresh status
/lang  — change language
/help  — show this`
}
