package telegram

import (
	"errors"
	"strings"
	"testing"

	"vergex/internal/agent"
	"vergex/internal/assistant"
	"vergex/internal/forecast"
)

// plainOf is a message as one text, as its plain parts read.
func plainOf(t *testing.T, m *message) string {
	t.Helper()
	var out []string
	for _, part := range m.parts() {
		wellFormed(t, part.HTML)
		out = append(out, part.Plain)
	}
	return strings.Join(out, "\n\n")
}

func contains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}

func TestTheCommandsLayOutWhatTheAccountsHold(t *testing.T) {
	holdings := []assistant.Holding{
		{Name: "Main", Equity: 12345.67, EquityRead: true, PositionsRead: true, Positions: []assistant.Position{
			{Symbol: "BTCUSDT", Side: "long", Quantity: 0.01, Gain: 12.3, HasGain: true},
			{Symbol: "SOLUSDT", Side: "short", Quantity: 1.2, Gain: -3.1, HasGain: true},
		}},
		{Name: "Practice", Testnet: true, Equity: 1000, EquityRead: true, PositionsRead: true},
		{Name: "Broken"},
	}
	traders := []assistant.TraderState{{Name: "Trend", Kind: "agent", Running: true}, {Name: "趋势一号", Kind: "grid"}}

	// Test money is in the list, never in the total.
	contains(t, plainOf(t, statusMessage(holdings, nil, traders, nil, "en")),
		"💵 Overview · 12.3K USDT\nReal money only",
		"• Main: 12.3K", "• Practice (testnet): 1.0K", "• Broken: could not be read",
		"• Positions: at least 2", "• Traders: 1/2 running")

	positions := plainOf(t, positionsMessage(holdings, nil, "zh"))
	// Quantities share their decimals, so they line up.
	contains(t, positions, "📊 持仓 · 2\n3 个账户", "BTCUSDT", "0.01", "+12.30", "1.20", "-3.10", "合计", "+9.20",
		"• Practice · 测试网：没有持仓", "• Broken · 实盘：无法读取")
	if strings.Contains(positions, "Main · 实盘\n") == false {
		t.Errorf("the table should carry its account as its caption:\n%s", positions)
	}

	contains(t, plainOf(t, tradersMessage(traders, nil, "zh")), "🤖 交易员 · 1/2 运行中", "运行", "agent", "Trend", "停止", "趋势一号")
	contains(t, plainOf(t, tradersMessage(nil, nil, "en")), "ℹ️ No traders yet\n\nTell me what kind to create")
	contains(t, plainOf(t, positionsMessage(nil, errors.New("down"), "en")), "❌ Accounts could not be read")
}

func TestAReplyLaysOutWhatItDroppedRanAndSaid(t *testing.T) {
	stop := assistant.ConfirmItem{Action: "stop", Target: "Trend"}
	remove := assistant.ConfirmItem{Action: "delete", Target: "Grid BTC"}

	// Writing something else while a batch waits drops it, then the model answers.
	got := plainOf(t, replyMessage(assistant.Reply{Dropped: &assistant.Dropped{Items: []assistant.ConfirmItem{stop}}, Said: "**BTC** is up"}, "zh"))
	contains(t, got, "↩️ 已取消\n\n• 停止交易员 · Trend\n\n**BTC** is up")

	// One of two failing says which, and why, in the words of what refused it.
	got = plainOf(t, replyMessage(assistant.Reply{Results: []assistant.ActionResult{
		{OK: true, Item: &stop},
		{Note: "trader is locked", Item: &remove},
	}}, "zh"))
	contains(t, got, "❌ 1 项失败 · 共 2 项", "• 停止交易员 · Trend：已执行\n• 删除交易员 · Grid BTC：失败\ntrader is locked")
	all := plainOf(t, replyMessage(assistant.Reply{Results: []assistant.ActionResult{{OK: true, Item: &stop}}}, "en"))
	contains(t, all, "✅ Done\n\n• Stop trader · Trend")

	// A model that failed says what to do, with the provider's own words collapsed under it.
	failure := &agent.Failure{Kind: agent.FailureAuth, Detail: "401 invalid_api_key"}
	m := replyMessage(assistant.Reply{Ended: assistant.EndFailed, Failure: failure, Text: "❌ ..."}, "en")
	contains(t, plainOf(t, m), "❌ Model request failed\n\nThe provider refused the API key. Check it in Settings → Agent models\n401 invalid_api_key")
	if html := m.parts()[0].HTML; !strings.Contains(html, "<blockquote expandable>401 invalid_api_key</blockquote>") {
		t.Fatalf("html = %s", html)
	}

	contains(t, plainOf(t, replyMessage(assistant.Reply{Ended: assistant.EndStopped}, "zh")), "🛑 已停止\n\n已经执行的操作不会撤销")
}

func TestACardOfSeveralActionsNumbersThemAndRunsThemTogether(t *testing.T) {
	c := &assistant.Confirmation{ID: "batch123", Items: []assistant.ConfirmItem{
		{Action: "stop", Target: "Trend", Account: "Main", Money: "real"},
		{Action: "update", Target: "趋势一号", Account: "Practice", Money: "testnet", Details: []string{"max_positions: 2"}},
	}}
	contains(t, plainOf(t, confirmationMessage(c, "zh")),
		"⚠️ 等你确认 · 2 项 · 含实盘\n5 分钟内有效",
		"1 停止交易员 · Trend\n• 账户：Main · 实盘",
		"2 修改交易员 · 趋势一号\n• 账户：Practice · 测试网\n• max_positions: 2",
		"点按钮或回复“确认”，其他回复都会取消")
	buttons := confirmationButtons(c, "zh").InlineKeyboard[0]
	if buttons[0].Text != "✅ 全部执行（2）" || buttons[1].Text != "↩️ 取消" || *buttons[0].CallbackData != "confirm:batch123" {
		t.Fatalf("buttons = %+v", buttons)
	}
	one := &assistant.Confirmation{ID: "b", Items: c.Items[:1]}
	if label := confirmationButtons(one, "en").InlineKeyboard[0][0].Text; label != "🛑 Stop trader" {
		t.Fatalf("one action's button = %q", label)
	}
}

func TestFiguresReadAsTheSpecWritesThem(t *testing.T) {
	cases := map[string]string{
		amount(845.2): "845.20", amount(12345.67): "12.3K", amount(1234567): "1.2M", change(12.3): "+12.30", change(-3.1): "-3.10",
		exact(2410.2): "2,410.2", exact(0.0015): "0.0015", exact(-64820.5): "-64,820.5", dollars(0.42): "$0.42",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if got := exacts([]float64{0.01, 1.2, 1250}); got[0] != "0.01" || got[1] != "1.20" || got[2] != "1,250.00" {
		t.Errorf("a column of quantities = %q", got)
	}
}

func TestAForecastAlertKeepsEveryCallWhole(t *testing.T) {
	got := plainOf(t, forecastMessage(forecast.Alert{Strategy: "Momentum", Signals: []forecast.Signal{
		{Symbol: "1000PEPEUSDT", Interval: "4h", Direction: forecast.DirectionDown, DisplayProb: 0.36, Horizon: 6},
	}}, "zh"))
	contains(t, got, "📈 预测信号\n\nMomentum\n", "1000PEPEUSDT 4h", "下跌", "~24 h")
	if !strings.HasSuffix(got, "36") || strings.Contains(got, "…") {
		t.Fatalf("the score was cut:\n%s", got)
	}
}
