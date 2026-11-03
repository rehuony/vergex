package telegram

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"vergex/internal/assistant"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// The bot's own messages, each in the shape of message.go.

// What a button sends.
const (
	buttonConfirm  = "confirm"
	buttonCancel   = "cancel"
	buttonLanguage = "lang"
)

// What a refusal says about the bot, in English, since nothing is known of who asks.
const (
	refusedStatus = "Access denied"
	refusedAccess = "bound owner only"
	refusedNote   = "This attempt was recorded"
	unboundStatus = "Not bound"
	badCodeStatus = "Code not valid or expired"
	unboundAccess = "binding code from Settings → Telegram"
	unboundNote   = "Send the code here to bind this chat"
)

// actions names what a held action does, with the emoji its button leads with, in the web
// app's words.
var actions = map[string]struct{ emoji, en, zh string }{
	"create":               {"➕", "Create trader", "创建交易员"},
	"start":                {"▶️", "Start trader", "启动交易员"},
	"stop":                 {"🛑", "Stop trader", "停止交易员"},
	"delete":               {"🗑", "Delete trader", "删除交易员"},
	"update":               {"✏️", "Update trader", "修改交易员"},
	"close":                {"🔻", "Close position", "平仓"},
	"delete_conversation":  {"🗑", "Delete conversation", "删除对话"},
	"delete_conversations": {"🗑", "Delete conversations", "删除对话"},
	"delete_plan":          {"🗑", "Delete plan", "删除计划"},
	"delete_plans":         {"🗑", "Delete plans", "删除计划"},
}

// actionLine is an action as one line: what it does and to what.
func actionLine(it assistant.ConfirmItem, lang string) text {
	named, ok := actions[it.Action]
	if !ok {
		return strong(it.Target)
	}
	return joined(strong(pick(lang, named.en, named.zh)), words(" · "+it.Target))
}

// accountOf is the account an action acts on, and with what kind of money.
func accountOf(it assistant.ConfirmItem, lang string) string {
	switch it.Money {
	case "real":
		return it.Account + " · " + pick(lang, "Real money", "实盘")
	case "testnet":
		return it.Account + " · " + pick(lang, "Testnet", "测试网")
	}
	return it.Account
}

// confirmationMessage is what the user reads before answering: Go's own account of each
// action. The model has no part in it.
func confirmationMessage(c *assistant.Confirmation, lang string) *message {
	m := newMessage(lang)
	title := m.pick("Waiting for your confirmation", "等你确认")
	if n := len(c.Items); n > 1 {
		title += m.pick(fmt.Sprintf(" · %d", n), fmt.Sprintf(" · %d 项", n))
	}
	for _, it := range c.Items {
		if it.Money == "real" {
			title += m.pick(" · real money", " · 含实盘")
			break
		}
	}
	m.title(emojiConfirm, title).meta(m.pick("Expires in 5 min", "5 分钟内有效"))
	for i, it := range c.Items {
		head := actionLine(it, lang)
		if len(c.Items) > 1 {
			head = joined(strong(strconv.Itoa(i+1)+" "), head)
		}
		lines := []text{head}
		if it.Account != "" {
			lines = append(lines, m.fact(m.pick("Account", "账户"), strong(accountOf(it, lang))))
		}
		for _, detail := range it.Details {
			lines = append(lines, item(words(detail)))
		}
		m.add(lines...)
	}
	return m.hint(m.pick(`Tap a button or reply "confirm". Anything else cancels`, "点按钮或回复“确认”，其他回复都会取消"))
}

func confirmationButtons(c *assistant.Confirmation, lang string) tgbotapi.InlineKeyboardMarkup {
	run := "✅ " + pick(lang, fmt.Sprintf("Run all %d", len(c.Items)), fmt.Sprintf("全部执行（%d）", len(c.Items)))
	if len(c.Items) == 1 {
		if named, ok := actions[c.Items[0].Action]; ok {
			run = named.emoji + " " + pick(lang, named.en, named.zh)
		} else {
			run = "✅ " + pick(lang, "Confirm", "确认")
		}
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(run, buttonConfirm+":"+c.ID),
		tgbotapi.NewInlineKeyboardButtonData("↩️ "+pick(lang, "Cancel", "取消"), buttonCancel+":"+c.ID),
	))
}

// replyMessage lays out what a turn came to: what it dropped and ran, how it ended, and what
// the model said.
func replyMessage(r assistant.Reply, lang string) *message {
	m := newMessage(lang)
	if r.Dropped != nil {
		title := m.pick("Cancelled", "已取消")
		if r.Dropped.Expired {
			title = m.pick("Expired, nothing ran", "已超时，没有执行")
		}
		m.title(emojiDropped, title)
		lines := make([]text, len(r.Dropped.Items))
		for i, it := range r.Dropped.Items {
			lines[i] = item(actionLine(it, lang))
		}
		m.add(lines...)
	}
	if len(r.Results) > 0 {
		resultsOf(m, r.Results)
	}
	switch r.Ended {
	case assistant.EndStopped:
		m.notice(emojiStopped, m.pick("Stopped", "已停止"), m.pick("Anything already done stays done", "已经执行的操作不会撤销"))
	case assistant.EndTimeout:
		m.notice(emojiFailed, m.pick("Out of time, not finished", "超过时限，没有做完"), m.pick("Anything already done stays done", "已经执行的操作不会撤销"))
	case assistant.EndSteps:
		m.notice(emojiFailed, m.pick("Too many steps, not finished", "步骤太多，没有做完"), m.pick("Ask in smaller parts, or look in the web app", "可以拆开来问，或在网页上查看"))
	case assistant.EndEmpty:
		m.notice(emojiFailed, m.pick("The model returned nothing", "模型没有返回内容"), m.pick("Try again, or check the model in Settings → Agent models", "请重试，或在 设置 → Agent 模型 检查"))
	case assistant.EndUnavailable:
		m.notice(emojiSetup, m.pick("No model set up", "还没有可用的模型"), m.pick("Add one in the web app: Settings → Agent models", "请在网页上添加：设置 → Agent 模型"))
	case assistant.EndFailed:
		m.notice(emojiFailed, m.pick("Model request failed", "模型请求失败"), "")
		if r.Failure != nil {
			failure := *r.Failure
			detail := failure.Detail
			failure.Detail = ""
			m.add(words(failure.Describe(lang == "zh")))
			m.details(detail)
		}
	}
	if r.Said != "" {
		m.markdown(r.Said)
	} else if m.empty() {
		m.markdown(r.Text)
	}
	return m
}

// resultsOf says how confirmed actions went: all done, or which failed and why.
func resultsOf(m *message, results []assistant.ActionResult) {
	failed := 0
	for _, result := range results {
		if !result.OK {
			failed++
		}
	}
	n := len(results)
	switch {
	case failed == 0 && n == 1:
		m.title(emojiDone, m.pick("Done", "已执行"))
	case failed == 0:
		m.title(emojiDone, m.pick(fmt.Sprintf("Done · %d", n), fmt.Sprintf("已执行 · %d 项", n)))
	case n == 1:
		m.title(emojiFailed, m.pick("Failed", "执行失败"))
	default:
		m.title(emojiFailed, m.pick(fmt.Sprintf("%d of %d failed", failed, n), fmt.Sprintf("%d 项失败 · 共 %d 项", failed, n)))
	}
	for i, result := range results {
		line := strong(result.Text)
		if result.Item != nil {
			line = actionLine(*result.Item, m.lang)
		}
		if failed > 0 && n > 1 {
			state := m.pick("Done", "已执行")
			if !result.OK {
				state = m.pick("Failed", "失败")
			}
			line = joined(line, words(m.pick(": ", "：")), strong(state))
		}
		if i == 0 {
			m.add(item(line))
		} else {
			m.under(item(line))
		}
		m.details(result.Note)
	}
}

// refusalMessage is all someone other than the owner hears, and says nothing of what is
// behind the bot.
func refusalMessage(status, access, note string, id int64, now time.Time) *message {
	m := newMessage("en")
	m.title(emojiPrivate, "Private bot").meta(status + " · " + access)
	m.add(m.fact("Your ID", chip(strconv.FormatInt(id, 10))), m.fact("Time", strong(moment(now))))
	return m.hint(note)
}

// commandsBlock lists commands in the menu's words, shortest first.
func commandsBlock(lang string, names ...string) []text {
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) < len(names[j]) })
	lines := make([]text, 0, len(names))
	for _, name := range names {
		for _, entry := range menu {
			if entry.command == name {
				lines = append(lines, words("/"+name+" — "+pick(lang, entry.en, entry.zh)))
			}
		}
	}
	return lines
}

// welcomeCommands are what a welcome lists.
var welcomeCommands = []string{"help", "status", "traders", "positions"}

// readySection says what can be done once everything is set up.
func readySection(m *message) {
	m.add(words(m.pick(`Tell me what you want, such as "show my positions" or "stop all traders"`, "直接告诉我要做什么，比如“看看我的持仓”或“停止所有交易员”")))
	m.section(m.pick("Commands", "命令"), commandsBlock(m.lang, welcomeCommands...)...)
}

// missingSetup says what has to be set up in the web app first.
func missingSetup(st *Stores, userID string) (model, account bool) {
	if _, err := st.Models.GetDefault(userID); err != nil {
		model = true
	}
	if accounts, err := st.Accounts.List(userID); err != nil || len(accounts) == 0 {
		account = true
	}
	return model, account
}

// setupSection lists what is left to set up, and reports whether anything is.
func setupSection(m *message, st *Stores, userID string) bool {
	noModel, noAccount := missingSetup(st, userID)
	if !noModel && !noAccount {
		return false
	}
	m.title(emojiSetup, m.pick("Setup required", "需要先完成设置"))
	var lines []text
	if noModel {
		lines = append(lines, m.fact(m.pick("Model", "模型"), strong(m.pick("Settings → Agent models → Add", "设置 → Agent 模型 → 添加"))))
	}
	if noAccount {
		lines = append(lines, m.fact(m.pick("Account", "账户"), strong(m.pick("Settings → Trading accounts → Add", "设置 → 交易账户 → 添加"))))
	}
	m.add(lines...)
	m.hint(m.pick("Send /start when done", "完成后发送 /start"))
	return true
}

// welcomeMessage is what the owner reads on arriving: a heading, then what is left to set up
// or what they can do now.
func welcomeMessage(st *Stores, userID, lang, heading string) *message {
	m := newMessage(lang)
	m.title(emojiDone, heading).meta(m.pick("Welcome to Vergex", "欢迎使用 Vergex"))
	if !setupSection(m, st, userID) {
		readySection(m)
	}
	return m
}

// setupMessage says what is left to set up, when a message cannot be answered without it.
func setupMessage(st *Stores, userID, lang string) *message {
	m := newMessage(lang)
	if !setupSection(m, st, userID) {
		m.notice(emojiSetup, m.pick("No model set up", "还没有可用的模型"), m.pick("Add one in the web app: Settings → Agent models", "请在网页上添加：设置 → Agent 模型"))
	}
	return m
}

// languageSetMessage says the language changed, in it.
func languageSetMessage(lang string) *message {
	m := newMessage(lang)
	m.title(emojiDone, m.pick("Language set to English", "已切换为中文"))
	m.section(m.pick("Commands", "命令"), commandsBlock(lang, welcomeCommands...)...)
	return m
}

func languageMessage() *message {
	return newMessage("en").title(emojiLanguage, "Choose your language / 选择语言")
}

func languageButtons(current string) tgbotapi.InlineKeyboardMarkup {
	label := func(lang, name string) string {
		if lang == current {
			return "✓ " + name
		}
		return name
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(label("en", "English"), buttonLanguage+":en"),
		tgbotapi.NewInlineKeyboardButtonData(label("zh", "中文"), buttonLanguage+":zh"),
	))
}

func helpMessage(lang string) *message {
	m := newMessage(lang)
	m.title(emojiHelp, m.pick("Help", "帮助"))
	m.section(m.pick("Answered at once, without the model", "立即回答，不用模型"), commandsBlock(lang, "status", "positions", "traders")...)
	asks := []string{"Why isn't my trader opening anything?", "Create a careful BTC trader and start it", "Set max positions to 2", "Close Trend's BTC long", "How are my Futures plans doing?", "What's the forecast for BTC?"}
	if lang == "zh" {
		asks = []string{"为什么交易员不开仓？", "创建一个谨慎的 BTC 交易员并启动", "把最大持仓改成 2 个", "平掉 Trend 的 BTC 多单", "合约交易的计划怎么样了？", "BTC 的预测是什么？"}
	}
	lines := make([]text, len(asks))
	for i, ask := range asks {
		lines[i] = item(words(m.pick(`"`+ask+`"`, "“"+ask+"”")))
	}
	m.section(m.pick("Just ask", "直接说"), lines...)
	m.section(m.pick("Control", "控制"), commandsBlock(lang, "stop", "cancel", "lang", "start")...)
	return m.hint(m.pick("Starting, stopping, deleting, closing and changing risk settings wait for your confirmation. Approving and running plans, keys and binding are only done in the web app",
		"启动、停止、删除、平仓和改风控会先等你确认。计划的批准和执行、密钥和绑定只能在网页上做"))
}

// strangerMessage tells the owner someone else tried the bot.
func strangerMessage(lang, name string, id int64, chatType string) *message {
	m := newMessage(lang)
	m.title(emojiAttention, m.pick("Someone else tried this bot", "有人尝试使用这个机器人")).meta(m.pick("Refused", "已拒绝"))
	chat := map[string][2]string{"private": {"private chat", "私聊"}, "group": {"group", "群组"}, "supergroup": {"group", "群组"}, "channel": {"channel", "频道"}}[chatType]
	lines := []text{m.fact(m.pick("User", "用户"), strong(name)), m.fact("ID", chip(strconv.FormatInt(id, 10)))}
	if chat[0] != "" {
		lines = append(lines, m.fact(m.pick("Chat", "聊天"), strong(m.pick(chat[0], chat[1]))))
	}
	m.add(lines...)
	return m.hint(m.pick("See Visitors in Settings → Telegram", "可在 设置 → Telegram 的访客用户里查看"))
}
