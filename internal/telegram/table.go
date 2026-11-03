package telegram

import (
	"html"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// A table is a code block whose language is its caption, which Telegram shows in a strip of
// its own with the copy button beside it, so the button never covers the header. Its rows line
// up whatever width a phone's font gives a Chinese character: each column is padded with ASCII
// spaces for the letters a cell is short of and ideographic spaces for the Chinese characters
// it is short of, so every row has the same number of each before every column.

// ideographicSpace is as wide as a Chinese character, whatever a font makes that.
const ideographicSpace = "　"

const (
	// maxTableRows is how many rows a table shows before saying how many more there are.
	maxTableRows = 10
	// tableWidth is what a row may take, in letters, before a phone wraps it.
	tableWidth = 38
	// A phone's font sets a Chinese character at about five thirds of a letter. The rule is
	// measured at three halves, so it stays a little short of the rows and never wraps.
	wideWidth = 5.0 / 3
	ruleWide  = 1.5
)

type column struct {
	name  string
	right bool
}

type table struct {
	caption string
	columns []column
	rows    [][]string
	// total is the last row, under a rule of its own, when a column adds up.
	total []string
	// whole says no name is cut, for a table whose words are not the bot's to shorten.
	whole bool
}

func isWide(r rune) bool {
	return unicode.Is(unicode.Han, r) || (r >= 0x3000 && r <= 0x303f) || (r >= 0xff00 && r <= 0xffef)
}

// counts says how many narrow and how many wide characters a cell holds.
func counts(s string) (narrow, wide int) {
	for _, r := range s {
		if isWide(r) {
			wide++
		} else {
			narrow++
		}
	}
	return narrow, wide
}

func shown(narrow, wide int, scale float64) int {
	return narrow + int(math.Ceil(float64(wide)*scale))
}

// caption is a language for Telegram, which is one word: parts are joined by `·`, and the
// spaces inside a name are ones that do not break it.
func caption(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " · ", "·")), " ")
}

// lines is the table and, past maxTableRows, how many more there are.
func (t table) lines(lang string) []text {
	rows, more := t.rows, 0
	if len(rows) > maxTableRows {
		rows, more = rows[:maxTableRows], len(rows)-maxTableRows
	}
	n := len(t.columns)
	last := n - 1
	header := make([]string, n)
	for i, c := range t.columns {
		header[i] = c.name
	}
	all := append([][]string{header}, rows...)
	if t.total != nil {
		all = append(all, t.total)
	}
	cell := func(row []string, i int) string {
		if i < len(row) {
			return row[i]
		}
		return ""
	}
	narrow, wide := make([]int, n), make([]int, n)
	measure := func(i int, s string) {
		a, w := counts(s)
		narrow[i], wide[i] = max(narrow[i], a), max(wide[i], w)
	}
	for _, row := range all {
		for i := 0; i < last; i++ {
			measure(i, cell(row, i))
		}
	}
	// A name in the last column is cut to what the columns before it leave of a row.
	before, beforeWide := 2*last, 0
	for i := 0; i < last; i++ {
		before += narrow[i]
		beforeWide += wide[i]
	}
	room := max(tableWidth-shown(before, beforeWide, wideWidth), 6)
	lastCell := func(row []string) string {
		if t.columns[last].right || t.whole {
			return cell(row, last)
		}
		return cut(cell(row, last), room)
	}
	for _, row := range all {
		measure(last, lastCell(row))
	}
	format := func(row []string) string {
		cells := make([]string, n)
		for i := range n {
			value := cell(row, i)
			if i == last {
				value = lastCell(row)
			}
			a, w := counts(value)
			pad := strings.Repeat(" ", narrow[i]-a) + strings.Repeat(ideographicSpace, wide[i]-w)
			if t.columns[i].right {
				cells[i] = pad + value
			} else {
				cells[i] = value + pad
			}
		}
		return strings.TrimRight(strings.Join(cells, "  "), " "+ideographicSpace)
	}
	totalNarrow, totalWide := before+narrow[last], beforeWide+wide[last]
	rule := strings.Repeat("─", totalNarrow+int(float64(totalWide)*ruleWide))
	body := []string{format(header), rule}
	for _, row := range rows {
		body = append(body, format(row))
	}
	if t.total != nil {
		body = append(body, rule, format(t.total))
	}
	source := strings.Join(body, "\n")
	// As plain text, the caption goes on a line of its own above the rows.
	out := []text{{
		html:  `<pre><code class="language-` + html.EscapeString(caption(t.caption)) + `">` + escape(source) + "</code></pre>",
		plain: t.caption + "\n" + source,
	}}
	if more > 0 {
		out = append(out, words(pick(lang, "… "+strconv.Itoa(more)+" more", "… 还有 "+strconv.Itoa(more)+" 个")))
	}
	return out
}

// cut shortens a name to what a row has room for.
func cut(s string, room int) string {
	a, w := counts(s)
	if shown(a, w, wideWidth) <= room {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		a, w := counts(string(runes))
		if shown(a, w, wideWidth)+1 <= room {
			break
		}
	}
	return string(runes) + "…"
}
