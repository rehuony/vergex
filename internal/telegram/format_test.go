package telegram

import (
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"testing"
)

func oneHTML(t *testing.T, text string) string {
	t.Helper()
	parts := renderReply(text)
	if len(parts) != 1 {
		t.Fatalf("%q became %d parts", text, len(parts))
	}
	return parts[0].HTML
}

func TestWhatModelsWriteBecomesTelegramHTML(t *testing.T) {
	cases := map[string]string{
		"plain":                     "plain",
		"a < b & c > d":             "a &lt; b &amp; c &gt; d",
		"**BTC** is `up` now":       "<b>BTC</b> is <code>up</code> now",
		"`<b>` stays code":          "<code>&lt;b&gt;</code> stays code",
		"BTC_USDT and ETH_USDT":     "BTC_USDT and ETH_USDT",
		"2 * 3 * 4":                 "2 * 3 * 4",
		"it is *really* so":         "it is <i>really</i> so",
		"~~gone~~":                  "<s>gone</s>",
		"## Positions":              "<b>Positions</b>",
		"- one\n- **two**":          "• one\n• <b>two</b>",
		"  - nested":                "  • nested",
		"1. one\n2) two":            "1. one\n2. two",
		"> quoted\n> more":          "<blockquote>quoted\nmore</blockquote>",
		"```\nx < 1\n```":           "<pre>x &lt; 1</pre>",
		"[docs](https://a.b/c)":     `<a href="https://a.b/c">docs</a>`,
		"[bad](javascript:x())":     "[bad](javascript:x())",
		"one\n\n---\n\ntwo":         "one\n\ntwo",
		"<script>alert(1)</script>": "&lt;script&gt;alert(1)&lt;/script&gt;",
	}
	for input, want := range cases {
		if got := oneHTML(t, input); got != want {
			t.Errorf("%q\n got %q\nwant %q", input, got, want)
		}
	}
}

func TestATableThatFitsAPhoneLinesUp(t *testing.T) {
	got := oneHTML(t, "| Coin | Side | P/L |\n| --- | :-: | --: |\n| **BTC** | long | +3 |\n| ETH | short | -1.5 |")
	want := "<pre><code class=\"language-Table\">Coin  Side    P/L\n─────────────────\nBTC   long     +3\nETH   short  -1.5</code></pre>"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// Chinese is padded with spaces as wide as it is, so the columns hold on any phone.
	got = oneHTML(t, "| 币种 | 方向 | 盈亏 |\n| - | - | - |\n| BTC | 多 | +3 |")
	if !strings.Contains(got, `language-表格`) || !strings.Contains(got, "币种\u3000\u3000 方向  盈亏\n") && !strings.Contains(got, "BTC\u3000\u3000  多\u3000") {
		t.Fatalf("got %q", got)
	}
}

func TestATableTooWideForAPhoneBecomesOneEntryPerRow(t *testing.T) {
	long := strings.Repeat("very long words ", 4)
	got := oneHTML(t, "| Coin | Note |\n| --- | --- |\n| BTC | "+long+"|\n| ETH | short |")
	want := "<b>BTC</b>\nNote: " + strings.TrimSpace(long) + "\n<b>ETH</b>\nNote: short"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestATableOfTheBotsOwnLinesUpWhateverItsRowsHold(t *testing.T) {
	lines := table{caption: "主账户 · 实盘", columns: []column{{name: "品种"}, {name: "方向"}, {name: "盈亏", right: true}}, rows: [][]string{
		{"BTCUSDT", "多", "+12.30"}, {"SOLUSDT", "空", "-3.10"},
	}, total: []string{"合计", "", "+9.20"}}.lines("zh")
	if len(lines) != 1 || !strings.HasPrefix(lines[0].html, `<pre><code class="language-主账户·实盘">`) {
		t.Fatalf("lines = %q", lines)
	}
	rows := strings.Split(strings.TrimPrefix(lines[0].plain, "主账户 · 实盘\n"), "\n")
	// Every row holds as many narrow and as many wide characters, header and total too, so
	// each column starts in the same place whatever a wide one measures.
	narrow, wide := counts(rows[2])
	for _, row := range []string{rows[0], rows[3], rows[5]} {
		if a, w := counts(row); a != narrow || w != wide {
			t.Errorf("%q holds %d narrow and %d wide, %q %d and %d", row, a, w, rows[2], narrow, wide)
		}
	}

	// A name too long for a row is cut, and more rows than fit are counted.
	many := table{caption: "x", columns: []column{{name: "n"}, {name: "Name"}}}
	for i := 0; i < 12; i++ {
		many.rows = append(many.rows, []string{strconv.Itoa(i), strings.Repeat("名", 40)})
	}
	got := many.lines("en")
	if len(got) != 2 || got[1].plain != "… 2 more" {
		t.Fatalf("got %q", got)
	}
	for _, row := range strings.Split(got[0].plain, "\n") {
		if a, w := counts(row); shown(a, w, wideWidth) > tableWidth {
			t.Errorf("%q is wider than a phone", row)
		}
	}
}

// wellFormed checks every tag closes, which is what Telegram refuses a message over.
func wellFormed(t *testing.T, text string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader("<m>" + text + "</m>"))
	decoder.Strict = false
	for {
		if _, err := decoder.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("%v in %q", err, text)
		}
	}
}

func TestLongRepliesSplitIntoMessagesTelegramTakes(t *testing.T) {
	inputs := map[string]string{
		"paragraphs": strings.Repeat("**word** word word. ", 200) + "\n\n" + strings.Repeat("second paragraph. ", 400),
		"one line":   strings.Repeat("字", 9000),
		"emoji":      strings.Repeat("🙂", 5000),
		"escapes":    strings.Repeat("<&>", 4000),
		"code":       "```\n" + strings.Repeat("line of code\n", 1000) + "```",
		"list":       strings.Repeat("- an item of the list\n", 800),
		"table":      "| a | b |\n| - | - |\n" + strings.Repeat("| x | y |\n", 900),
	}
	for name, input := range inputs {
		parts := renderReply(input)
		if len(parts) < 2 {
			t.Errorf("%s: %d parts", name, len(parts))
		}
		for _, part := range parts {
			if size := utf16Len(part.HTML); size > 4096 {
				t.Errorf("%s: HTML of %d units", name, size)
			}
			if size := utf16Len(part.Plain); size > 4096 {
				t.Errorf("%s: plain text of %d units", name, size)
			}
			if part.HTML == "" || part.Plain == "" {
				t.Errorf("%s: an empty part", name)
			}
			wellFormed(t, part.HTML)
		}
	}
}

func TestNothingToSayIsNoMessage(t *testing.T) {
	for _, input := range []string{"", "  \n \n", "---"} {
		if parts := renderReply(input); len(parts) != 0 {
			t.Errorf("%q became %v", input, parts)
		}
	}
}
