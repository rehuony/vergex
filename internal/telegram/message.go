package telegram

import "strings"

// This file is the shape every message of the bot's own takes: an emoji and a bold title, a
// meta line under it, then blocks with a blank line between them (bold sections with their
// facts, tables, lists) and a hint at the bottom. Notices have one shape of their own. What
// the model writes comes in through markdown, and everything leaves through parts, so all of
// it is cut into Telegram messages the same way. DESIGN.md has the rules.

// text is a run of a message as Telegram HTML, and as plain text for when Telegram refuses
// the HTML.
type text struct{ html, plain string }

// escape is what Telegram's HTML needs escaped outside a tag.
var escape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace

func words(s string) text  { return text{escape(s), s} }
func strong(s string) text { return text{"<b>" + escape(s) + "</b>", s} }
func chip(s string) text   { return text{"<code>" + escape(s) + "</code>", s} }
func faint(s string) text  { return text{"<i>" + escape(s) + "</i>", s} }

func joined(parts ...text) text {
	var out text
	for _, part := range parts {
		out.html += part.html
		out.plain += part.plain
	}
	return out
}

// The emoji a title opens with, each with one meaning.
const (
	emojiStatus    = "💵"
	emojiPositions = "📊"
	emojiTraders   = "🤖"
	emojiHelp      = "❓"
	emojiLanguage  = "🌐"
	emojiWorking   = "⏳"
	emojiDone      = "✅"
	emojiFailed    = "❌"
	emojiDropped   = "↩️"
	emojiStopped   = "🛑"
	emojiConfirm   = "⚠️"
	emojiNothing   = "ℹ️"
	emojiSetup     = "⚙️"
	emojiAttention = "🚨"
	emojiEntry     = "📥"
	emojiClosed    = "📤"
	emojiReport    = "🗓"
	emojiForecast  = "📈"
	emojiPrivate   = "🔒"
)

// maxDetail bounds the words of others a message quotes: a provider's message can run long.
const maxDetail = 300

// block is a run of lines that stays together, with a blank line before the next. A line can
// hold an element over several lines, a table or a quote, and is never cut.
type block []text

// message is what the bot says at once, in the language it is said in.
type message struct {
	lang   string
	blocks []block
	// silent says it arrives without a sound.
	silent bool
}

func newMessage(lang string) *message { return &message{lang: lang} }

func (m *message) pick(en, zh string) string { return pick(m.lang, en, zh) }

// add starts a block.
func (m *message) add(lines ...text) *message {
	if len(lines) > 0 {
		m.blocks = append(m.blocks, block(lines))
	}
	return m
}

// under adds lines to the block before, or starts one.
func (m *message) under(lines ...text) *message {
	if len(m.blocks) == 0 {
		return m.add(lines...)
	}
	last := len(m.blocks) - 1
	m.blocks[last] = append(m.blocks[last], lines...)
	return m
}

// heading is a title line: an emoji, the title in bold, and the figure that matters most.
func heading(emoji, title string, figure ...string) text {
	line := joined(words(emoji+" "), strong(title))
	for _, f := range figure {
		if f != "" {
			line = joined(line, words(" · "), chip(f))
		}
	}
	return line
}

// title opens the message.
func (m *message) title(emoji, title string, figure ...string) *message {
	return m.add(heading(emoji, title, figure...))
}

// meta is the line under a title: where it happened, what it counts, how long it lasts.
func (m *message) meta(s string) *message {
	if s == "" {
		return m
	}
	return m.under(faint(s))
}

// notice is the one shape of a short message: a title, then what it means or what to do.
func (m *message) notice(emoji, title, body string) *message {
	m.title(emoji, title)
	if body != "" {
		m.add(words(body))
	}
	return m
}

// section is a bold heading with the lines under it.
func (m *message) section(name string, lines ...text) *message {
	return m.add(append([]text{strong(name)}, lines...)...)
}

// fact is a line of a list: a label and its value.
func (m *message) fact(label string, value text) text {
	return joined(words("• "+label+m.pick(": ", "：")), value)
}

// item is a line of a list.
func item(line text) text { return joined(words("• "), line) }

// details quotes what is not ours, collapsed, under what it explains.
func (m *message) details(s string) *message {
	s = strings.TrimSpace(s)
	if s == "" {
		return m
	}
	if runes := []rune(s); len(runes) > maxDetail {
		s = string(runes[:maxDetail]) + "…"
	}
	return m.under(text{"<blockquote expandable>" + escape(s) + "</blockquote>", s})
}

// hint is the last line: what to do next, or how to answer.
func (m *message) hint(s string) *message { return m.add(faint(s)) }

// table adds a table as a block of its own.
func (m *message) table(t table) *message { return m.add(t.lines(m.lang)...) }

// markdown adds what the model wrote.
func (m *message) markdown(source string) *message {
	m.blocks = append(m.blocks, markdownBlocks(source)...)
	return m
}

func (m *message) empty() bool { return len(m.blocks) == 0 }

// parts cuts the message into Telegram messages: whole blocks while they fit, a block too long
// for one between its lines.
func (m *message) parts() []messagePart { return pack(m.blocks, maxMessageHTML) }

func pack(blocks []block, limit int) []messagePart {
	var parts []messagePart
	var current text
	flush := func() {
		if current.html != "" {
			parts = append(parts, messagePart{HTML: current.html, Plain: current.plain})
		}
		current = text{}
	}
	appendRun := func(run text, gap string) {
		if current.html != "" && (utf16Len(current.html)+utf16Len(gap)+utf16Len(run.html) > limit ||
			utf16Len(current.plain)+utf16Len(gap)+utf16Len(run.plain) > limit) {
			flush()
		}
		if current.html != "" {
			current = joined(current, words(gap), run)
			return
		}
		current = run
	}
	for _, b := range blocks {
		whole := lines(b)
		if utf16Len(whole.html) <= limit && utf16Len(whole.plain) <= limit {
			appendRun(whole, "\n\n")
			continue
		}
		for i, line := range b {
			gap := "\n"
			if i == 0 {
				gap = "\n\n"
			}
			appendRun(line, gap)
		}
	}
	flush()
	return parts
}

// lines joins a block's lines.
func lines(b block) text {
	var out text
	for i, line := range b {
		if i > 0 {
			out = joined(out, words("\n"))
		}
		out = joined(out, line)
	}
	return out
}
