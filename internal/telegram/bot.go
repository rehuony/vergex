package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"vergex/internal/agent"
	_ "vergex/internal/agent/provider"
	"vergex/internal/assistant"
	"vergex/internal/config"
	"vergex/internal/logger"
	"vergex/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// apiEndpoint is the Telegram Bot API URL pattern; tests point it at a fake.
var apiEndpoint = tgbotapi.APIEndpoint

// retryDelay is how long Start waits after the bot failed to start or lost its
// update stream (a wrong token, no network) before trying again, unless the
// settings change first.
var retryDelay = time.Minute

// menuRetryDelay is how often a command menu Telegram refused is tried again. The bot keeps
// working meanwhile, so this does not wait as long as a bot that cannot start.
var menuRetryDelay = 15 * time.Second

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

// botName is the running bot's username, empty while none runs.
var botName atomic.Value

// BotName says which bot is running, if one is.
func BotName() string {
	name, _ := botName.Load().(string)
	return name
}

// tokenInURL matches a bot token where the Bot API's URLs carry it.
var tokenInURL = regexp.MustCompile(`bot\d+:[A-Za-z0-9_-]+`)

// scrubToken takes a bot token out of an error or log line.
func scrubToken(text string) string { return tokenInURL.ReplaceAllString(text, "bot***") }

// botLog is where the Bot API library logs. It prints failed requests with their URL, so
// what it says goes through scrubToken.
type botLog struct{}

func (botLog) Println(v ...interface{}) {
	logger.Warnf("Telegram: %s", scrubToken(strings.TrimSpace(fmt.Sprintln(v...))))
}

func (botLog) Printf(format string, v ...interface{}) {
	logger.Warnf("Telegram: %s", scrubToken(fmt.Sprintf(format, v...)))
}

func init() {
	if err := tgbotapi.SetLogger(botLog{}); err != nil {
		logger.Warnf("Telegram: set the logger: %v", err)
	}
}

// runBot serves the bot until a reload signal (nil) or until it cannot start or
// loses its update stream (an error).
func runBot(token string, cfg *config.Config, st *Stores, reloadCh <-chan struct{}) error {
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiEndpoint)
	if err != nil {
		// A transport error names the URL, and the URL holds the token.
		return fmt.Errorf("start: %s", scrubToken(err.Error()))
	}
	logger.Infof("Telegram bot @%s started", bot.Self.UserName)
	botName.Store(bot.Self.UserName)
	defer botName.Store("")

	r := &runner{
		bot: bot, cfg: cfg, st: st, turnedAway: map[int64]time.Time{},
		jobs: make(chan func(), turnQueue), stopped: make(chan struct{}),
	}
	r.resolveOwner()
	r.resolveUser()
	// Before the bot says it runs or reads a message, so its commands are there when it does.
	r.refreshMenu()
	service.Go("telegram-work", r.work)
	defer func() {
		r.stopTurn()
		close(r.stopped)
	}()
	retryMenu := time.NewTicker(menuRetryDelay)
	defer retryMenu.Stop()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	// Nothing else is handled, so nothing else is asked for.
	u.AllowedUpdates = []string{"message", "callback_query"}
	updates := bot.GetUpdatesChan(u)
	for {
		select {
		case <-reloadCh:
			bot.StopReceivingUpdates()
			return nil
		case <-retryMenu.C:
			if r.menuStale {
				r.refreshMenu()
			}
		case update, ok := <-updates:
			if !ok {
				return errors.New("update stream closed")
			}
			switch {
			case update.Message != nil:
				r.handleMessage(update.Message)
			case update.CallbackQuery != nil:
				r.handleButton(update.CallbackQuery)
			}
		}
	}
}

// runner is the bot while it runs.
type runner struct {
	bot *tgbotapi.BotAPI
	cfg *config.Config
	st  *Stores
	// owner is the user bound to this system, 0 when none is. A private chat has its user's
	// id, so this is also the only chat the bot works for.
	owner int64
	// turnedAway remembers who was told this is a private bot, so nobody can use it to
	// send messages, and the owner hears of each of them once.
	turnedAway map[int64]time.Time

	// The registered user and their agents, resolved lazily and refreshed if that changes.
	userID    string
	userEmail string
	agents    *assistant.Manager
	// confirming is the message with the buttons for what is waiting, 0 when none is.
	confirming atomic.Int64
	// menuStale says Telegram refused the last menu, which runBot then tries again.
	menuStale bool

	// jobs is what the chat asked for, waiting its turn; see work.
	jobs    chan func()
	stopped chan struct{}
	// cancelTurn stops the turn under way, nil when none is.
	turnMu     sync.Mutex
	cancelTurn context.CancelFunc
}

// newClient builds what calls the model; tests replace it.
var newClient = newAgentClient

// chosenLanguage is the language picked with /lang, empty until one is.
func (r *runner) chosenLanguage() string {
	cfg, err := r.st.Telegram.Get()
	if err != nil {
		return ""
	}
	return cfg.Language
}

// resolveOwner reads who is bound. What is saved decides, not what this run started with,
// so an unbinding holds from the moment it is saved. Only a private chat can be bound: a
// binding left on a group from before this rule counts as none, and has to be made again.
// It reports false when that cannot be read, and then nobody gets through.
func (r *runner) resolveOwner() bool {
	id, err := r.st.Telegram.GetBoundChatID()
	if err != nil {
		logger.Warnf("Telegram: read the bound chat: %v", err)
		return false
	}
	if id < 0 {
		logger.Warnf("Telegram: ignoring a binding to group chat %d; bind a private chat in Settings", id)
		id = 0
	}
	// Turns read this from their own goroutine, so it is written only when it changes.
	if r.owner != id {
		r.owner = id
	}
	return true
}

// isOwner reports whether this is the given user, writing in their own private chat with the
// bot. Both ids have to match: neither says on its own who is at the other end.
func isOwner(owner int64, chat *tgbotapi.Chat, from *tgbotapi.User) bool {
	return owner != 0 && chat != nil && from != nil && chat.IsPrivate() && chat.ID == owner && from.ID == owner
}

// resolveUser finds the registered user the bot acts for, and starts the agents on them.
func (r *runner) resolveUser() bool {
	users, err := r.st.Users.GetAll()
	if err != nil || len(users) == 0 {
		return false
	}
	u := users[0]
	if u.ID == r.userID {
		return true
	}
	r.userID, r.userEmail = u.ID, u.Email
	userID := u.ID
	token := func() (string, error) { return assistant.CallToken(userID) }
	r.agents = assistant.NewManager(r.cfg.APIServerPort, token, u.Email, u.ID,
		func() agent.Client { return newClient(r.st, userID) },
		&chatHistory{conversations: r.st.Conversations, userID: u.ID},
		r.chosenLanguage,
		assistant.Telegram,
	)
	logger.Infof("Bot: acting for user %s (%s)", u.ID, u.Email)
	return true
}

// chat is what the assistant knows the bound chat by.
func (r *runner) chat() string { return strconv.FormatInt(r.owner, 10) }

// parseCommand splits "/start abc" or "/start@some_bot abc" into its command and argument.
// Anything else has no command.
func parseCommand(text string) (command, argument string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	head, argument, _ := strings.Cut(text, " ")
	head, _, _ = strings.Cut(strings.TrimPrefix(head, "/"), "@")
	return head, strings.TrimSpace(argument)
}

// handleMessage is the one way in for a message. Who it comes from is settled before
// anything else, so no command or stored state is within reach of anyone but the owner.
func (r *runner) handleMessage(m *tgbotapi.Message) {
	if !r.resolveOwner() {
		return
	}
	text := strings.TrimSpace(m.Text)
	command, argument := parseCommand(text)
	switch {
	case !m.Chat.IsPrivate():
		r.noteStranger(m)
		return
	case r.owner == 0:
		r.handleUnbound(m, text, command, argument)
		return
	case !isOwner(r.owner, m.Chat, m.From):
		// Start is something they pressed, so it always has an answer.
		if command == "start" {
			r.noteStranger(m)
			r.refuse(m, refusedStatus, refusedAccess, refusedNote)
			return
		}
		r.turnAway(m)
		return
	}
	r.adoptLanguage(text)
	lang := r.st.Telegram.GetLanguage()

	switch command {
	case "start":
		if !r.resolveUser() {
			r.send(noUserMessage(lang))
			return
		}
		agents, userID := r.agents, r.userID
		r.enqueue(func() {
			agents.Reset(r.chat())
			r.send(welcomeMessage(r.st, userID, lang, pick(lang, "New chat started", "已开始新对话")))
		})
		return
	case "stop":
		// This one does not wait its turn: stopping what is under way is the point of it.
		if r.stopTurn() {
			return
		}
		r.send(newMessage(lang).title(emojiNothing, pick(lang, "Nothing is running", "现在没有在做的事")))
		return
	case "cancel":
		if !r.resolveUser() {
			return
		}
		agents := r.agents
		r.enqueue(func() {
			reply, held := agents.CancelPending(r.chat())
			r.closeConfirming()
			if !held {
				r.send(newMessage(lang).title(emojiNothing, pick(lang, "Nothing is waiting for your confirmation", "没有等待确认的操作")))
				return
			}
			r.send(replyMessage(reply, lang))
		})
		return
	case "status", "positions", "traders":
		if !r.resolveUser() {
			return
		}
		r.runReadCommand(command, lang)
		return
	case "lang":
		if choice := parseLangChoice(argument); choice != "" {
			r.setLanguage(choice)
			return
		}
		r.askLanguage()
		return
	case "help":
		r.send(helpMessage(lang))
		return
	}
	if text == "" {
		return
	}

	// ── Refresh user before every model call ────────────────────────────────
	if !r.resolveUser() {
		r.send(noUserMessage(lang))
		return
	}

	// ── Guard: say what is left to set up when no model can answer ─────────
	if newClient(r.st, r.userID) == nil {
		r.send(setupMessage(r.st, r.userID, lang))
		return
	}
	r.runAgent(text)
}

// noUserMessage says nobody has registered in the web app yet.
func noUserMessage(lang string) *message {
	return newMessage(lang).notice(emojiSetup, pick(lang, "No account yet", "还没有账户"),
		pick(lang, "Register in the web app, then send /start", "请先在网页上注册，然后发送 /start"))
}

// send says something to the owner.
func (r *runner) send(m *message) { deliver(r.bot, r.owner, 0, m, nil) }

// handleUnbound answers a private chat while no one is bound. Pressing Start sends a bare
// /start, so that is met with what to do next, and the code can then be sent on its own. The
// link sends /start with the code, which binds at once.
func (r *runner) handleUnbound(m *tgbotapi.Message, text, command, argument string) {
	code := ""
	switch command {
	case "":
		code = text
	case "start":
		code = argument
	}
	if isOwner(m.Chat.ID, m.Chat, m.From) && useBindCode(code, time.Now()) {
		r.bind(m)
		return
	}
	r.noteStranger(m)
	status := unboundStatus
	if (command == "start" && code != "") || bindCodeShape.MatchString(code) {
		status = badCodeStatus
	}
	r.refuse(m, status, unboundAccess, unboundNote)
}

// runReadCommand answers a command that only reads, in turn with everything else but
// without the model.
func (r *runner) runReadCommand(command, lang string) {
	agents, chat := r.agents, r.chat()
	r.enqueue(func() {
		r.runWork(func(progress *assistant.Progress) *message {
			progress.Stage(pick(lang, "Reading…", "正在读取…"))
			switch command {
			case "positions":
				holdings, err := agents.Holdings(chat)
				return positionsMessage(holdings, err, lang)
			case "traders":
				traders, err := agents.Traders(chat)
				return tradersMessage(traders, err, lang)
			}
			holdings, holdingsErr := agents.Holdings(chat)
			traders, tradersErr := agents.Traders(chat)
			return statusMessage(holdings, holdingsErr, traders, tradersErr, lang)
		})
	})
}

// adoptLanguage settles the language for someone who never chose one, the first time they
// write in Chinese. English is what everyone starts on.
func (r *runner) adoptLanguage(text string) {
	if r.chosenLanguage() == "" && assistant.PrefersChinese(text) {
		if err := r.st.Telegram.SetLanguage("zh"); err != nil {
			logger.Warnf("Telegram: save the language: %v", err)
			return
		}
		r.refreshMenu()
	}
}

// askLanguage offers the languages as buttons. Nothing waits for a typed answer, so no
// message of the owner's can be mistaken for one.
func (r *runner) askLanguage() {
	markup := languageButtons(r.chosenLanguage())
	deliver(r.bot, r.owner, 0, languageMessage(), &markup)
}

// setLanguage switches the bot, its menu and its alerts to a language.
func (r *runner) setLanguage(lang string) {
	if err := r.st.Telegram.SetLanguage(lang); err != nil {
		logger.Warnf("Telegram: save the language: %v", err)
	}
	r.refreshMenu()
	r.send(languageSetMessage(lang))
}

// refreshMenu publishes the commands and reports how the bot stands: it runs once they are in
// place, and until then says why not while runBot keeps trying.
func (r *runner) refreshMenu() {
	err := r.publishMenu()
	r.menuStale = err != nil
	if err != nil {
		detail := "commands not published: " + scrubToken(err.Error())
		logger.Warnf("Telegram: %s; retrying in %v", detail, menuRetryDelay)
		service.Report(service.Telegram, service.Degraded, detail)
		return
	}
	service.Report(service.Telegram, service.Running, "")
}

// publishMenu lists /start in every private chat, which is all anyone can do there who is not
// bound and says nothing of what the bot does, and the whole menu in the owner's chat. Groups
// get none, as the bot does not answer them.
func (r *runner) publishMenu() error {
	if err := requestPatiently(r.bot, tgbotapi.NewDeleteMyCommands()); err != nil {
		return err
	}
	start := tgbotapi.BotCommand{Command: "start", Description: "Start this bot"}
	if err := requestPatiently(r.bot, tgbotapi.NewSetMyCommandsWithScope(tgbotapi.NewBotCommandScopeAllPrivateChats(), start)); err != nil {
		return err
	}
	if r.owner == 0 {
		return nil
	}
	lang := r.st.Telegram.GetLanguage()
	commands := make([]tgbotapi.BotCommand, 0, len(menu))
	for _, entry := range menu {
		commands = append(commands, tgbotapi.BotCommand{Command: entry.command, Description: pick(lang, entry.en, entry.zh)})
	}
	return requestPatiently(r.bot, tgbotapi.NewSetMyCommandsWithScope(tgbotapi.NewBotCommandScopeChat(r.owner), commands...))
}

// menu is what the owner sees after typing "/", kept short enough for a phone to show whole.
var menu = []struct{ command, en, zh string }{
	{"status", "Accounts at a glance", "账户概览"},
	{"positions", "Open positions", "查看持仓"},
	{"traders", "List traders", "查看交易员"},
	{"stop", "Stop what is running", "停止当前操作"},
	{"cancel", "Cancel what awaits you", "取消待确认的操作"},
	{"lang", "Change language", "切换语言"},
	{"help", "What I can do", "能做什么"},
	{"start", "Start a new chat", "开始新对话"},
}

// displayName is what Settings calls the bound user: their @username, which is optional on
// Telegram, or else their name.
func displayName(user *tgbotapi.User) string {
	if user.UserName != "" {
		return "@" + user.UserName
	}
	return strings.TrimSpace(user.FirstName + " " + user.LastName)
}

// bind makes this user the owner, for someone who came with the code Settings gave them.
func (r *runner) bind(m *tgbotapi.Message) {
	name := displayName(m.From)
	if lang := bindLanguage(); lang != "" && r.chosenLanguage() == "" {
		r.st.Telegram.SetLanguage(lang) //nolint:errcheck
	}
	if err := r.st.Telegram.BindUser(m.Chat.ID, name); err != nil {
		logger.Errorf("Failed to bind Telegram user: %v", err)
		lang := r.st.Telegram.GetLanguage()
		deliver(r.bot, m.Chat.ID, 0, newMessage(lang).notice(emojiFailed, pick(lang, "Binding failed", "绑定失败"), pick(lang, "Try again", "请重试")), nil)
		return
	}
	r.owner = m.Chat.ID
	logger.Infof("Telegram bound to %s (chatID: %d)", name, m.Chat.ID)
	if err := r.st.Visitors.Forget(m.Chat.ID); err != nil {
		logger.Warnf("Telegram: forget the bound user's attempts: %v", err)
	}
	r.resolveUser()
	r.refreshMenu()
	lang := r.st.Telegram.GetLanguage()
	r.send(welcomeMessage(r.st, r.userID, lang, pick(lang, "Bound", "绑定成功")))
}

// turnAwayEvery is how often one stranger hears that this is a private bot.
const turnAwayEvery = 24 * time.Hour

// noteStranger keeps every message from outside the owner's chat for Settings to show, and logs
// it and tells the owner of it once a day per chat. It reports whether this is the first time
// in that day.
func (r *runner) noteStranger(m *tgbotapi.Message) bool {
	now := time.Now()
	if err := r.st.Visitors.Record(newVisitor(m), now); err != nil {
		logger.Warnf("Telegram: record who was refused: %v", err)
	}
	if last, seen := r.turnedAway[m.Chat.ID]; seen && now.Sub(last) < turnAwayEvery {
		return false
	}
	// One entry per stranger, so someone trying many chats cannot grow this for long.
	if len(r.turnedAway) > 1000 {
		r.turnedAway = map[int64]time.Time{}
	}
	r.turnedAway[m.Chat.ID] = now
	id, name := m.Chat.ID, "?"
	if m.From != nil {
		id, name = m.From.ID, displayName(m.From)
	}
	logger.Warnf("Telegram: refused a message from %s (ID %d, %s)", name, id, m.Chat.Type)
	if r.owner == 0 {
		return true
	}
	cfg, err := r.st.Telegram.Get()
	if err != nil || cfg.AlertMuted(AlertSystem) {
		return true
	}
	r.send(strangerMessage(r.st.Telegram.GetLanguage(), name, id, m.Chat.Type))
	return true
}

// turnAway answers someone in a private chat who is not the owner, once a day.
func (r *runner) turnAway(m *tgbotapi.Message) {
	if r.noteStranger(m) {
		r.refuse(m, refusedStatus, refusedAccess, refusedNote)
	}
}

// refuse tells someone where they stand.
func (r *runner) refuse(m *tgbotapi.Message, status, access, note string) {
	id := m.Chat.ID
	if m.From != nil {
		id = m.From.ID
	}
	deliver(r.bot, m.Chat.ID, 0, refusalMessage(status, access, note, id, time.Now()), nil)
}

// runAgent hands a message to the assistant, in turn with whatever came before it.
func (r *runner) runAgent(text string) {
	agents, chat := r.agents, r.chat()
	r.enqueue(func() {
		r.runTurn(func(ctx context.Context, progress *assistant.Progress) assistant.Reply {
			return agents.Run(ctx, chat, text, progress)
		})
		agents.Tidy(chat)
	})
}

// deliver shows a message: its first part in place of the placeholder, if there is one, and
// the rest as new messages, the last with the buttons, if there are any. Nothing to say takes
// the placeholder away. It returns the id of the last message, 0 when none went out.
func deliver(bot *tgbotapi.BotAPI, chatID int64, placeholderID int, m *message, buttons *tgbotapi.InlineKeyboardMarkup) int {
	parts := m.parts()
	if len(parts) == 0 && placeholderID != 0 {
		bot.Request(tgbotapi.NewDeleteMessage(chatID, placeholderID)) //nolint:errcheck
		return 0
	}
	last := 0
	for i, part := range parts {
		var markup *tgbotapi.InlineKeyboardMarkup
		if i == len(parts)-1 {
			markup = buttons
		}
		if i == 0 && placeholderID != 0 && editPart(bot, chatID, placeholderID, part, markup) {
			last = placeholderID
			continue
		}
		if id := sendPart(bot, chatID, part, markup, m.silent); id != 0 {
			last = id
		}
	}
	return last
}

// editPart replaces a message's text, as HTML and failing that as plain text.
func editPart(bot *tgbotapi.BotAPI, chatID int64, messageID int, part messagePart, buttons *tgbotapi.InlineKeyboardMarkup) bool {
	for _, edit := range []tgbotapi.EditMessageTextConfig{
		tgbotapi.NewEditMessageText(chatID, messageID, part.HTML),
		tgbotapi.NewEditMessageText(chatID, messageID, part.Plain),
	} {
		if edit.Text == part.HTML {
			edit.ParseMode = tgbotapi.ModeHTML
		}
		edit.DisableWebPagePreview = true
		edit.ReplyMarkup = buttons
		if sendPatiently(bot, edit) == nil {
			return true
		}
	}
	return false
}

// sendPart sends a message, as HTML and failing that as plain text, and returns its id.
func sendPart(bot *tgbotapi.BotAPI, chatID int64, part messagePart, buttons *tgbotapi.InlineKeyboardMarkup, silent bool) int {
	var err error
	for i, body := range []string{part.HTML, part.Plain} {
		msg := tgbotapi.NewMessage(chatID, body)
		if i == 0 {
			msg.ParseMode = tgbotapi.ModeHTML
		}
		msg.DisableWebPagePreview = true
		msg.DisableNotification = silent
		if buttons != nil {
			msg.ReplyMarkup = *buttons
		}
		var sent tgbotapi.Message
		if sent, err = sendPatientlyFor(bot, msg); err == nil {
			return sent.MessageID
		}
	}
	logger.Warnf("Telegram: send a message: %v", err)
	return 0
}

// askToConfirm shows what is waiting for an answer, in Go's words, with its buttons.
func (r *runner) askToConfirm(c *assistant.Confirmation) {
	if c == nil {
		return
	}
	lang := r.st.Telegram.GetLanguage()
	buttons := confirmationButtons(c, lang)
	if id := deliver(r.bot, r.owner, 0, confirmationMessage(c, lang), &buttons); id != 0 {
		r.confirming.Store(int64(id))
	}
}

// closeConfirming takes the buttons off the last request to confirm.
func (r *runner) closeConfirming() {
	if id := r.confirming.Swap(0); id != 0 {
		r.removeButtons(int(id))
	}
}

func (r *runner) removeButtons(messageID int) {
	none := tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
	r.bot.Send(tgbotapi.NewEditMessageReplyMarkup(r.owner, messageID, none)) //nolint:errcheck
}

// handleButton is the one way in for a button press. It passes the same gate as a message,
// and a press from anyone else does nothing at all.
func (r *runner) handleButton(q *tgbotapi.CallbackQuery) {
	r.bot.Request(tgbotapi.NewCallback(q.ID, "")) //nolint:errcheck
	if !r.resolveOwner() {
		return
	}
	if q.Message == nil || !isOwner(r.owner, q.Message.Chat, q.From) {
		if q.Message != nil && q.Message.Chat != nil {
			logger.Warnf("Telegram: refused a button from chat %d", q.Message.Chat.ID)
		}
		return
	}
	action, id, _ := strings.Cut(q.Data, ":")
	switch action {
	case buttonStop:
		r.stopTurn()
		return
	case buttonLanguage:
		r.removeButtons(q.Message.MessageID)
		if choice := parseLangChoice(id); choice != "" {
			r.setLanguage(choice)
		}
		return
	}
	if (action != buttonConfirm && action != buttonCancel) || !r.resolveUser() {
		return
	}
	messageID := q.Message.MessageID
	r.confirming.CompareAndSwap(int64(messageID), 0)
	r.removeButtons(messageID)
	agents, chat, lang := r.agents, r.chat(), r.st.Telegram.GetLanguage()
	r.enqueue(func() {
		r.runWork(func(progress *assistant.Progress) *message {
			if action == buttonConfirm {
				progress.Stage(pick(lang, "Running it…", "正在执行…"))
			}
			reply, live := agents.Answer(chat, id, action == buttonConfirm)
			if !live {
				return newMessage(lang).notice(emojiDropped, pick(lang, "Button no longer valid", "按钮已失效"),
					pick(lang, "That no longer waits for an answer, so nothing was done", "这项已经不在等待确认，没有执行任何操作"))
			}
			return replyMessage(reply, lang)
		})
	})
}

// ── model client ───────────────────────────────────────────────────────────────

func newAgentClient(st *Stores, userID string) agent.Client {
	// 1. Prefer the model explicitly configured for Telegram (Settings → Telegram → Model)
	if tgCfg, err := st.Telegram.Get(); err == nil && tgCfg.AgentModelID != "" {
		if model, err := st.Models.Get(userID, tgCfg.AgentModelID); err == nil && model.Enabled {
			apiKey := string(model.APIKey)
			if apiKey != "" {
				client := clientForProvider(model.Provider)
				client.SetAPIKey(apiKey, model.BaseURL, model.ModelName)
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
			client.SetAPIKey(apiKey, model.BaseURL, model.ModelName)
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

// ── Language ──────────────────────────────────────────────────────────────────

func parseLangChoice(text string) string {
	switch strings.TrimSpace(text) {
	case "en", "EN", "English", "english":
		return "en"
	case "zh", "ZH", "chinese", "Chinese", "中文":
		return "zh"
	}
	return ""
}
