package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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
	agents    *telegramagent.Manager
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
	token := func() (string, error) { return telegramagent.GenerateBotToken(userID) }
	r.agents = telegramagent.NewManager(r.cfg.APIServerPort, token, u.Email, u.ID,
		func() agent.Client { return newClient(r.st, userID) },
		&chatHistory{conversations: r.st.Conversations, userID: u.ID},
		r.chosenLanguage,
	)
	logger.Infof("Bot: acting for user %s (%s)", u.ID, u.Email)
	return true
}

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
			sendMsg(r.bot, r.owner, pick(lang,
				"No account found\nOpen the web dashboard to register, then send /start",
				"还没有账户\n请先在网页上注册，然后发送 /start"))
			return
		}
		agents, userID := r.agents, r.userID
		r.enqueue(func() {
			agents.Reset(r.owner)
			sendMarkdownMsg(r.bot, r.owner, welcomeMsg(r.st, userID, lang,
				pick(lang, "✅ **Welcome to Vergex**", "✅ **欢迎使用 Vergex**")))
		})
		return
	case "stop":
		// This one does not wait its turn: stopping what is under way is the point of it.
		if r.stopTurn() {
			return
		}
		sendMsg(r.bot, r.owner, pick(lang, "Nothing is running", "现在没有在做的事"))
		return
	case "cancel":
		if !r.resolveUser() {
			return
		}
		agents := r.agents
		r.enqueue(func() {
			reply, held := agents.CancelPending(r.owner)
			r.closeConfirming()
			if !held {
				reply.Text = pick(lang, "Nothing is waiting for your confirmation", "没有等待确认的操作")
			}
			deliver(r.bot, r.owner, 0, reply.Text)
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
		sendMarkdownMsg(r.bot, r.owner, helpMsg(lang))
		return
	}
	if text == "" {
		return
	}

	// ── Refresh user before every model call ────────────────────────────────
	if !r.resolveUser() {
		sendMsg(r.bot, r.owner, pick(lang, "No account found. Open the web app to register", "还没有账户。请先在网页上注册"))
		return
	}

	// ── Guard: show status if not ready for trading ───────────────────────
	if newClient(r.st, r.userID) == nil {
		sendMarkdownMsg(r.bot, r.owner, statusMsg(r.st, r.userID, r.cfg.APIServerPort, lang))
		return
	}
	r.runAgent(text)
}

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
	agents, chatID := r.agents, r.owner
	r.enqueue(func() {
		r.runTurn(func(_ context.Context, progress *telegramagent.Progress) telegramagent.Reply {
			progress.Stage(pick(lang, "Reading…", "正在读取…"))
			switch command {
			case "positions":
				return telegramagent.Reply{Text: agents.Positions(chatID)}
			case "traders":
				return telegramagent.Reply{Text: agents.Traders(chatID)}
			}
			return telegramagent.Reply{Text: agents.Status(chatID)}
		})
	})
}

// adoptLanguage settles the language for someone who never chose one, the first time they
// write in Chinese. English is what everyone starts on.
func (r *runner) adoptLanguage(text string) {
	if r.chosenLanguage() == "" && telegramagent.PrefersChinese(text) {
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
	msg := tgbotapi.NewMessage(r.owner, langMenuMsg())
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("English", buttonLanguage+":en"),
		tgbotapi.NewInlineKeyboardButtonData("中文", buttonLanguage+":zh"),
	))
	r.bot.Send(msg) //nolint:errcheck
}

// setLanguage switches the bot, its menu and its alerts to a language.
func (r *runner) setLanguage(lang string) {
	if err := r.st.Telegram.SetLanguage(lang); err != nil {
		logger.Warnf("Telegram: save the language: %v", err)
	}
	r.refreshMenu()
	sendMarkdownMsg(r.bot, r.owner, statusMsg(r.st, r.userID, r.cfg.APIServerPort, lang))
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
		sendMsg(r.bot, m.Chat.ID, pick(r.st.Telegram.GetLanguage(), "Binding failed. Please try again", "绑定失败，请重试"))
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
	sendMarkdownMsg(r.bot, r.owner, welcomeMsg(r.st, r.userID, lang,
		pick(lang, "✅ **Bound. Welcome to Vergex**", "✅ **绑定成功，欢迎使用 Vergex**")))
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
	sendMsg(r.bot, r.owner, fmt.Sprintf(pick(r.st.Telegram.GetLanguage(),
		"Someone else tried to use this bot: %s (ID %d, %s). They were refused. See Visitors in Settings → Telegram",
		"有人尝试使用这个机器人：%s（ID %d，%s），已拒绝。可在 设置 → Telegram 的访客用户里查看"), name, id, m.Chat.Type))
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
	msg := tgbotapi.NewMessage(m.Chat.ID, refusalText(status, access, note, id, time.Now()))
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := r.bot.Send(msg); err != nil {
		logger.Warnf("Telegram: send a refusal: %s", scrubToken(err.Error()))
	}
}

// runAgent hands a message to the assistant, in turn with whatever came before it.
func (r *runner) runAgent(text string) {
	agents, chatID := r.agents, r.owner
	r.enqueue(func() {
		r.runTurn(func(ctx context.Context, progress *telegramagent.Progress) telegramagent.Reply {
			return agents.Run(ctx, chatID, text, progress)
		})
		agents.Tidy(chatID)
	})
}

// deliver shows a reply: its first part in place of the placeholder, if there is one, and
// the rest as new messages. Nothing to say takes the placeholder away.
func deliver(bot *tgbotapi.BotAPI, chatID int64, placeholderID int, text string) {
	parts := renderReply(text)
	if len(parts) == 0 && placeholderID != 0 {
		bot.Request(tgbotapi.NewDeleteMessage(chatID, placeholderID)) //nolint:errcheck
		return
	}
	for i, part := range parts {
		if i == 0 && placeholderID != 0 && editPart(bot, chatID, placeholderID, part) {
			continue
		}
		sendPart(bot, chatID, part)
	}
}

// editPart replaces a message's text, as HTML and failing that as plain text.
func editPart(bot *tgbotapi.BotAPI, chatID int64, messageID int, part messagePart) bool {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, part.HTML)
	edit.ParseMode = tgbotapi.ModeHTML
	if sendPatiently(bot, edit) == nil {
		return true
	}
	return sendPatiently(bot, tgbotapi.NewEditMessageText(chatID, messageID, part.Plain)) == nil
}

// sendPart sends a message, as HTML and failing that as plain text.
func sendPart(bot *tgbotapi.BotAPI, chatID int64, part messagePart) {
	msg := tgbotapi.NewMessage(chatID, part.HTML)
	msg.ParseMode = tgbotapi.ModeHTML
	if sendPatiently(bot, msg) == nil {
		return
	}
	if err := sendPatiently(bot, tgbotapi.NewMessage(chatID, part.Plain)); err != nil {
		logger.Warnf("Telegram: send a message: %v", err)
	}
}

// askToConfirm shows what is waiting for an answer, in Go's words, with its buttons.
func (r *runner) askToConfirm(c *telegramagent.Confirmation) {
	if c == nil {
		return
	}
	msg := tgbotapi.NewMessage(r.owner, confirmationText(c, r.st.Telegram.GetLanguage()))
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = confirmationButtons(c, r.st.Telegram.GetLanguage())
	sent, err := r.bot.Send(msg)
	if err != nil {
		logger.Warnf("Telegram: ask to confirm: %v", err)
		return
	}
	r.confirming.Store(int64(sent.MessageID))
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
	agents, chatID, lang := r.agents, r.owner, r.st.Telegram.GetLanguage()
	r.enqueue(func() {
		r.runTurn(func(_ context.Context, progress *telegramagent.Progress) telegramagent.Reply {
			if action == buttonConfirm {
				progress.Stage(pick(lang, "Running it…", "正在执行…"))
			}
			reply, live := agents.Answer(chatID, id, action == buttonConfirm)
			if !live {
				return telegramagent.Reply{Text: pick(lang, "This button is no longer valid. Nothing was done", "这个按钮已经失效，没有执行任何操作")}
			}
			return reply
		})
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func sendMsg(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	bot.Send(msg) //nolint:errcheck
}

func sendMarkdownMsg(bot *tgbotapi.BotAPI, chatID int64, text string) {
	deliver(bot, chatID, 0, text)
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

// statusMsg says what is left to set up, or that everything is ready.
func statusMsg(st *Stores, userID string, apiPort int, lang string) string {
	if setup := setupMsg(st, userID, lang); setup != "" {
		return setup
	}
	return pick(lang, "✅ **Vergex is ready**", "✅ **Vergex 已就绪**") + "\n\n" + readyMsg(lang)
}

// welcomeMsg is what the owner reads on arriving: a heading, then what is left to set up or
// what they can do now.
func welcomeMsg(st *Stores, userID, lang, heading string) string {
	if setup := setupMsg(st, userID, lang); setup != "" {
		return heading + "\n\n" + setup
	}
	return heading + "\n\n" + readyMsg(lang)
}

// setupMsg lists what has to be set up in the web app first, empty when nothing does.
func setupMsg(st *Stores, userID, lang string) string {
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
				missing += "\n- ❌ 模型：设置 → Agent 模型 → 添加"
			}
			if !hasAccount {
				missing += "\n- ❌ 账户：设置 → 交易账户 → 添加"
			}
			return "⚙️ **需要先完成设置**\n\n请在网页上完成：\n" + missing + "\n\n完成后发送 /start"
		}
		if !hasModel {
			missing += "\n- ❌ Model: Settings → Agent models → Add"
		}
		if !hasAccount {
			missing += "\n- ❌ Account: Settings → Trading accounts → Add"
		}
		return "⚙️ **Setup required**\n\nIn the web app:\n" + missing + "\n\nSend /start when done"
	}
	return ""
}

// readyMsg says what can be done with everything set up.
func readyMsg(lang string) string {
	if lang == "zh" {
		return `直接告诉我要做什么，比如“看看我的持仓”或“停止所有交易员”。

/status 账户概览 · /positions 持仓 · /help 全部功能`
	}
	return `Tell me what you want, such as "show my positions" or "stop all traders".

/status accounts · /positions positions · /help everything`
}

// ── Language ──────────────────────────────────────────────────────────────────

func langMenuMsg() string {
	return "🌐 Choose your language / 选择语言"
}

func parseLangChoice(text string) string {
	switch strings.TrimSpace(text) {
	case "en", "EN", "English", "english":
		return "en"
	case "zh", "ZH", "chinese", "Chinese", "中文":
		return "zh"
	}
	return ""
}

// ── Help ──────────────────────────────────────────────────────────────────────

func helpMsg(lang string) string {
	if lang == "zh" {
		return `**Vergex 帮助**

**立即回答，不用模型**
- /status 账户概览
- /positions 所有账户的持仓
- /traders 交易员

**直接说**
- “为什么交易员不开仓？”
- “创建一个谨慎的 BTC 交易员并启动”
- “把最大持仓改成 2 个”
- “平掉 Trend 的 BTC 多单”
- “合约交易的计划怎么样了？”
- “BTC 的预测是什么？”

启动、停止、删除、平仓和改风控会先列出来等你确认。计划的批准和执行、密钥和绑定只能在网页上做。

**控制**
- /stop 停止当前操作
- /cancel 取消待确认的操作
- /lang 切换语言
- /start 开始新对话`
	}
	return `**Vergex Help**

**Answered at once, without the model**
- /status accounts at a glance
- /positions what every account holds
- /traders your traders

**Just ask**
- "Why isn't my trader opening anything?"
- "Create a careful BTC trader and start it"
- "Set max positions to 2"
- "Close Trend's BTC long"
- "How are my Futures plans doing?"
- "What's the forecast for BTC?"

Starting, stopping, deleting, closing and changing risk settings wait for your confirmation. Approving and running plans, keys and binding are only done in the web app.

**Control**
- /stop stop what is running
- /cancel cancel what awaits your confirmation
- /lang change language
- /start start a new chat`
}
