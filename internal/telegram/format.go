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

// block is a run of source lines that render together.
type block struct {
	kind  blockKind
	lines []string
	// head holds a table's column names.
	head []string
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
func parseBlocks(text string) []block {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []block
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "", horizontal.MatchString(line):
			i++
		case strings.HasPrefix(trimmed, "```"):
			i++
			code := []string{}
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			i++
			blocks = append(blocks, block{kind: blockCode, lines: code})
		case headingLine.MatchString(line):
			blocks = append(blocks, block{kind: blockHeading, lines: []string{headingLine.ReplaceAllString(line, "")}})
			i++
		case bulletLine.MatchString(line) || numberedLine.MatchString(line):
			items := []string{}
			for i < len(lines) && (bulletLine.MatchString(lines[i]) || numberedLine.MatchString(lines[i])) {
				items = append(items, lines[i])
				i++
			}
			blocks = append(blocks, block{kind: blockList, lines: items})
		case quoteLine.MatchString(line):
			quoted := []string{}
			for i < len(lines) && quoteLine.MatchString(lines[i]) {
				quoted = append(quoted, quoteLine.ReplaceAllString(lines[i], ""))
				i++
			}
			blocks = append(blocks, block{kind: blockQuote, lines: quoted})
		case strings.Contains(line, "|") && i+1 < len(lines) && strings.Contains(lines[i+1], "|") && tableRule.MatchString(lines[i+1]):
			head := tableCells(line)
			i += 2
			rows := []string{}
			for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" {
				rows = append(rows, lines[i])
				i++
			}
			blocks = append(blocks, block{kind: blockTable, head: head, lines: rows})
		default:
			paragraph := []string{}
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !isBlockStart(lines[i]) {
				paragraph = append(paragraph, lines[i])
				i++
			}
			if len(paragraph) == 0 {
				paragraph, i = []string{line}, i+1
			}
			blocks = append(blocks, block{kind: blockText, lines: paragraph})
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

// renderBlock turns one block into Telegram HTML. A table becomes one entry per row, its
// first cell standing as the name, since columns do not fit a phone and Telegram has no
// tables to line them up with.
func renderBlock(b block) string {
	switch b.kind {
	case blockHeading:
		return "<b>" + renderInline(strings.Join(b.lines, " ")) + "</b>"
	case blockCode:
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
func sourceOf(b block) string {
	lines := b.lines
	if b.kind == blockTable {
		lines = append([]string{strings.Join(b.head, " | ")}, lines...)
	}
	return strings.Join(lines, "\n")
}

func utf16Len(text string) int { return len(utf16.Encode([]rune(text))) }

// fit cuts a block until each piece renders within a message. Each piece renders on its
// own, so every message closes its own tags.
func fit(b block, limit int) []block {
	if utf16Len(renderBlock(b)) <= limit && utf16Len(sourceOf(b)) <= limit {
		return []block{b}
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
	return renderParts(text, maxMessageHTML)
}

func renderParts(text string, limit int) []messagePart {
	var parts []messagePart
	var htmlPart, plainPart strings.Builder
	flush := func() {
		if htmlPart.Len() > 0 {
			parts = append(parts, messagePart{
				HTML:  collapseSpace.ReplaceAllString(htmlPart.String(), "\n\n"),
				Plain: plainPart.String(),
			})
		}
		htmlPart.Reset()
		plainPart.Reset()
	}
	for _, whole := range parseBlocks(text) {
		for _, b := range fit(whole, limit) {
			rendered, source := renderBlock(b), sourceOf(b)
			if htmlPart.Len() > 0 && (utf16Len(htmlPart.String())+utf16Len(rendered)+2 > limit || utf16Len(plainPart.String())+utf16Len(source)+2 > limit) {
				flush()
			}
			if htmlPart.Len() > 0 {
				htmlPart.WriteString("\n\n")
				plainPart.WriteString("\n\n")
			}
			htmlPart.WriteString(rendered)
			plainPart.WriteString(source)
		}
	}
	flush()
	return parts
}
