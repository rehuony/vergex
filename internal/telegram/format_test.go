package telegram

import (
	"encoding/xml"
	"io"
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

func TestTablesBecomeOneEntryPerRow(t *testing.T) {
	got := oneHTML(t, "| Coin | Side | P/L |\n| --- | :-: | --: |\n| BTC | long | +3 |\n| ETH | short | -1 |")
	want := "<b>BTC</b>\nSide: long · P/L: +3\n<b>ETH</b>\nSide: short · P/L: -1"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
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
