package telegram

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf16"
)

// maxMessageHTML is what one message may hold once rendered. Telegram allows 4096 UTF-16
// units of what is left after parsing, so measuring the markup as well leaves room.
const maxMessageHTML = 3800

// messagePart is one Telegram message: HTML to send, and the plain text to send instead if
// Telegram refuses the HTML.
type messagePart struct {
	HTML  string
	Plain string
}

type blockKind int

const (
	blockText blockKind = iota
	blockHeading
	blockList
	blockQuote
	blockCode
	blockTable
)

// source is a run of the model's lines that render together.
type source struct {
	kind  blockKind
	lines []string
	// head holds a table's column names.
	head []string
	// language is what a fenced block names after its opening fence.
	language string
}

var (
	headingLine   = regexp.MustCompile(`^#{1,6}\s+`)
	bulletLine    = regexp.MustCompile(`^(\s*)[-*•+]\s+`)
	numberedLine  = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+`)
	quoteLine     = regexp.MustCompile(`^\s*>\s?`)
	tableRule     = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	horizontal    = regexp.MustCompile(`^\s*(-{3,}|\*{3,}|_{3,})\s*$`)
	inlineCode    = regexp.MustCompile("`[^`\n]+`")
	boldMark      = regexp.MustCompile(`\*\*([^*\n]+)\*\*|__([^_\n]+)__`)
	strikeMark    = regexp.MustCompile(`~~([^~\n]+)~~`)
	italicMark    = regexp.MustCompile(`(^|[\s(（])\*([^*\s][^*\n]*)\*`)
	linkMark      = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)]+)\)`)
	collapseSpace = regexp.MustCompile(`\n{3,}`)
)

// isBlockStart says a line ends the paragraph before it.
func isBlockStart(line string) bool {
	return headingLine.MatchString(line) || bulletLine.MatchString(line) || numberedLine.MatchString(line) ||
		quoteLine.MatchString(line) || strings.HasPrefix(strings.TrimSpace(line), "```") || horizontal.MatchString(line)
}

// parseBlocks reads the few Markdown forms models write.
func parseBlocks(text string) []source {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []source
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "", horizontal.MatchString(line):
			i++
		case strings.HasPrefix(trimmed, "```"):
			language := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			i++
			code := []string{}
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++
			blocks = append(blocks, source{kind: blockCode, lines: code, language: language})
		case headingLine.MatchString(line):
			blocks = append(blocks, source{kind: blockHeading, lines: []string{headingLine.ReplaceAllString(line, "")}})
			i++
		case bulletLine.MatchString(line) || numberedLine.MatchString(line):
			items := []string{}
			for i < len(lines) && (bulletLine.MatchString(lines[i]) || numberedLine.MatchString(lines[i])) {
				items = append(items, lines[i])
				i++
			}
			blocks = append(blocks, source{kind: blockList, lines: items})
		case quoteLine.MatchString(line):
			quoted := []string{}
			for i < len(lines) && quoteLine.MatchString(lines[i]) {
				quoted = append(quoted, quoteLine.ReplaceAllString(lines[i], ""))
				i++
			}
			blocks = append(blocks, source{kind: blockQuote, lines: quoted})
		case strings.Contains(line, "|") && i+1 < len(lines) && strings.Contains(lines[i+1], "|") && tableRule.MatchString(lines[i+1]):
			head := tableCells(line)
			i += 2
			rows := []string{}
			for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" {
				rows = append(rows, lines[i])
				i++
			}
			blocks = append(blocks, source{kind: blockTable, head: head, lines: rows})
		default:
			paragraph := []string{}
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !isBlockStart(lines[i]) {
				paragraph = append(paragraph, lines[i])
				i++
			}
			if len(paragraph) == 0 {
				paragraph, i = []string{line}, i+1
			}
			blocks = append(blocks, source{kind: blockText, lines: paragraph})
		}
	}
	return blocks
}

func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// renderInline escapes text first, so nothing a reply says can become markup, then turns
// the marks models use into the tags Telegram takes. Code keeps its contents as they are.
func renderInline(text string) string {
	var out strings.Builder
	last := 0
	for _, at := range inlineCode.FindAllStringIndex(text, -1) {
		out.WriteString(renderMarks(text[last:at[0]]))
		out.WriteString("<code>" + html.EscapeString(text[at[0]+1:at[1]-1]) + "</code>")
		last = at[1]
	}
	out.WriteString(renderMarks(text[last:]))
	return out.String()
}

func renderMarks(text string) string {
	text = html.EscapeString(text)
	text = linkMark.ReplaceAllString(text, `<a href="$2">$1</a>`)
	text = boldMark.ReplaceAllString(text, "<b>$1$2</b>")
	text = strikeMark.ReplaceAllString(text, "<s>$1</s>")
	return italicMark.ReplaceAllString(text, "$1<i>$2</i>")
}

// renderBlock turns one block into Telegram HTML. A table that fits a phone becomes a table of
// the bot's own; one too wide becomes one entry per row, its first cell standing as the name.
func renderBlock(b source) string {
	switch b.kind {
	case blockHeading:
		return "<b>" + renderInline(strings.Join(b.lines, " ")) + "</b>"
	case blockCode:
		if b.language != "" {
			return `<pre><code class="language-` + html.EscapeString(caption(b.language)) + `">` + html.EscapeString(strings.Join(b.lines, "\n")) + "</code></pre>"
		}
		return "<pre>" + html.EscapeString(strings.Join(b.lines, "\n")) + "</pre>"
	case blockQuote:
		return "<blockquote>" + renderInline(strings.Join(b.lines, "\n")) + "</blockquote>"
	case blockList:
		items := make([]string, len(b.lines))
		for i, line := range b.lines {
			if m := numberedLine.FindStringSubmatch(line); m != nil {
				items[i] = m[1] + m[2] + ". " + renderInline(numberedLine.ReplaceAllString(line, ""))
				continue
			}
			m := bulletLine.FindStringSubmatch(line)
			items[i] = m[1] + "• " + renderInline(bulletLine.ReplaceAllString(line, ""))
		}
		return strings.Join(items, "\n")
	case blockTable:
		if t, ok := modelTable(b); ok {
			return t.lines("")[0].html
		}
		rows := make([]string, len(b.lines))
		for i, line := range b.lines {
			cells := tableCells(line)
			details := []string{}
			for j := 1; j < len(cells); j++ {
				detail := renderInline(cells[j])
				if j < len(b.head) && b.head[j] != "" {
					detail = renderInline(b.head[j]) + ": " + detail
				}
				details = append(details, detail)
			}
			rows[i] = "<b>" + renderInline(cells[0]) + "</b>"
			if len(details) > 0 {
				rows[i] += "\n" + strings.Join(details, " · ")
			}
		}
		return strings.Join(rows, "\n")
	}
	return renderInline(strings.Join(b.lines, "\n"))
}

// sourceOf is a block as text, for when Telegram refuses its HTML.
func sourceOf(b source) string {
	if t, ok := modelTable(b); ok {
		return t.lines("")[0].plain
	}
	lines := b.lines
	if b.kind == blockTable {
		lines = append([]string{strings.Join(b.head, " | ")}, lines...)
	}
	return strings.Join(lines, "\n")
}

var (
	cellMarks  = regexp.MustCompile("\\*\\*|__|`")
	figureCell = regexp.MustCompile(`^[+\-−]?[$]?[\d,]*\.?\d+\s?[%KMkmx]?$|^[—-]$`)
)

// modelTable is a table the model wrote as one of the bot's own, when it fits a phone: a few
// columns and rows, as wide as a row can be. Figures are right-aligned, and the caption says
// only that it is a table, in the language its cells are in.
func modelTable(b source) (table, bool) {
	if b.kind != blockTable || len(b.head) < 2 || len(b.head) > 5 || len(b.lines) == 0 || len(b.lines) > maxTableRows {
		return table{}, false
	}
	plain := func(cell string) string { return strings.TrimSpace(cellMarks.ReplaceAllString(cell, "")) }
	t := table{caption: "Table", columns: make([]column, len(b.head)), whole: true}
	for i, name := range b.head {
		t.columns[i] = column{name: plain(name), right: true}
	}
	chinese := false
	for _, line := range b.lines {
		cells := tableCells(line)
		row := make([]string, len(b.head))
		for i := range row {
			if i < len(cells) {
				row[i] = plain(cells[i])
			}
			if row[i] != "" && !figureCell.MatchString(row[i]) {
				t.columns[i].right = false
			}
			if _, wide := counts(row[i]); wide > 0 {
				chinese = true
			}
		}
		t.rows = append(t.rows, row)
	}
	if chinese {
		t.caption = "表格"
	}
	for _, line := range strings.Split(t.lines("")[0].plain, "\n")[1:] {
		if a, w := counts(line); shown(a, w, wideWidth) > tableWidth {
			return table{}, false
		}
	}
	return t, true
}

func utf16Len(text string) int { return len(utf16.Encode([]rune(text))) }

// fit cuts a block until each piece renders within a message. Each piece renders on its
// own, so every message closes its own tags.
func fit(b source, limit int) []source {
	if utf16Len(renderBlock(b)) <= limit && utf16Len(sourceOf(b)) <= limit {
		return []source{b}
	}
	first, second := b, b
	if len(b.lines) > 1 {
		half := len(b.lines) / 2
		first.lines, second.lines = b.lines[:half], b.lines[half:]
	} else {
		runes := []rune(b.lines[0])
		half := len(runes) / 2
		first.lines, second.lines = []string{string(runes[:half])}, []string{string(runes[half:])}
	}
	return append(fit(first, limit), fit(second, limit)...)
}

// renderReply turns what the assistant wrote into messages Telegram will take.
func renderReply(text string) []messagePart {
	return pack(markdownBlocks(text), maxMessageHTML)
}

// markdownBlocks is what the model wrote as the blocks of a message, each short enough for
// one Telegram message.
func markdownBlocks(text string) []block {
	var blocks []block
	for _, whole := range parseBlocks(text) {
		for _, b := range fit(whole, maxMessageHTML) {
			rendered := collapseSpace.ReplaceAllString(renderBlock(b), "\n\n")
			blocks = append(blocks, block{{html: rendered, plain: sourceOf(b)}})
		}
	}
	return blocks
}
