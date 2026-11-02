package telegram

import (
	"fmt"
	"html"
	"strings"
	"time"

	telegramagent "vergex/internal/telegram/agent"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// What a confirmation's buttons send.
const (
	buttonConfirm  = "confirm"
	buttonCancel   = "cancel"
	buttonLanguage = "lang"
)

// What a refusal says about the bot.
const (
	refusedStatus     = "Access denied"
	refusedAccess     = "Bound owner only"
	refusedNote       = "This attempt was recorded"
	unboundStatus     = "Not bound"
	badCodeStatus     = "Code not valid or expired"
	unboundAccess     = "Binding code from Settings → Telegram"
	unboundNote       = "Send the code here to bind this chat"
	refusalTimeLayout = "2006-01-02 15:04 UTC"
)

// refusalText is all someone other than the owner hears. It is in English whoever they are,
// since nothing is known of them, and says nothing of what is behind the bot.
func refusalText(status, access, note string, id int64, now time.Time) string {
	return fmt.Sprintf("🔒 <b>Private bot</b>\n\n<b>Status:</b> %s\n<b>Access:</b> %s\n<b>Your ID:</b> <code>%d</code>\n<b>Time:</b> %s\n\n%s",
		status, access, id, now.UTC().Format(refusalTimeLayout), note)
}

// confirmationText is what the user reads before answering: Go's own summary of each
// action. The model has no part in it.
func confirmationText(c *telegramagent.Confirmation, lang string) string {
	var b strings.Builder
	b.WriteString("⚠️ <b>")
	b.WriteString(pick(lang, "Waiting for your confirmation", "等待你确认"))
	b.WriteString("</b>\n")
	for _, line := range c.Lines {
		b.WriteString("\n• ")
		b.WriteString(html.EscapeString(line))
	}
	b.WriteString("\n\n<i>")
	b.WriteString(pick(lang,
		"Tap a button or reply “confirm”. Anything else cancels. Nothing runs after 5 minutes",
		"点按钮或回复“确认”，其他回复都会取消。5 分钟后失效"))
	b.WriteString("</i>")
	return b.String()
}

func confirmationButtons(c *telegramagent.Confirmation, lang string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ "+pick(lang, "Confirm", "确认"), buttonConfirm+":"+c.ID),
		tgbotapi.NewInlineKeyboardButtonData("❌ "+pick(lang, "Cancel", "取消"), buttonCancel+":"+c.ID),
	))
}
