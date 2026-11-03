package telegram

import (
	"fmt"
	"strconv"

	"vergex/internal/assistant"
)

// What /status, /positions and /traders answer, from what the assistant reads without the
// model.

// sideOf is a position's side in a few letters, as a table shows it.
func sideOf(side, lang string) string {
	switch side {
	case "long", "buy":
		return pick(lang, "long", "多")
	case "short", "sell":
		return pick(lang, "short", "空")
	}
	return side
}

// moneyOf names an account's money where it is test money.
func moneyOf(name string, testnet bool, lang string) string {
	if testnet {
		return name + pick(lang, " (testnet)", "（测试网）")
	}
	return name
}

func statusMessage(holdings []assistant.Holding, holdingsErr error, traders []assistant.TraderState, tradersErr error, lang string) *message {
	m := newMessage(lang)
	if holdingsErr != nil {
		return m.notice(emojiFailed, m.pick("Accounts could not be read", "无法读取账户"), m.pick("Try again in a moment", "请稍后再试"))
	}
	if len(holdings) == 0 {
		return m.notice(emojiSetup, m.pick("No accounts yet", "还没有账户"), m.pick("Add one in the web app: Settings → Trading accounts", "请在网页上添加：设置 → 交易账户"))
	}
	total, read, open, allRead := 0.0, false, 0, true
	accounts := make([]text, len(holdings))
	for i, h := range holdings {
		value := strong(m.pick("could not be read", "无法读取"))
		if h.EquityRead {
			value = chip(amount(h.Equity))
			if !h.Testnet {
				total, read = total+h.Equity, true
			}
		}
		accounts[i] = m.fact(moneyOf(h.Name, h.Testnet, lang), value)
		if h.PositionsRead {
			open += len(h.Positions)
		} else {
			allRead = false
		}
	}
	figure := "—"
	if read {
		figure = amount(total) + " USDT"
	}
	m.title(emojiStatus, m.pick("Overview", "概览"), figure).meta(m.pick("Real money only", "只计实盘"))
	m.section(m.pick("Accounts", "账户"), accounts...)
	held := strconv.Itoa(open)
	if !allRead {
		held = m.pick("at least ", "至少 ") + held
	}
	running := []text{m.fact(m.pick("Positions", "持仓"), strong(held))}
	if tradersErr == nil {
		running = append(running, m.fact(m.pick("Traders", "交易员"), strong(runningOf(traders, lang))))
	}
	return m.section(m.pick("Running", "运行"), running...)
}

func runningOf(traders []assistant.TraderState, lang string) string {
	running := 0
	for _, t := range traders {
		if t.Running {
			running++
		}
	}
	return pick(lang, fmt.Sprintf("%d/%d running", running, len(traders)), fmt.Sprintf("%d/%d 运行中", running, len(traders)))
}

func positionsMessage(holdings []assistant.Holding, err error, lang string) *message {
	m := newMessage(lang)
	if err != nil {
		return m.notice(emojiFailed, m.pick("Accounts could not be read", "无法读取账户"), m.pick("Try again in a moment", "请稍后再试"))
	}
	if len(holdings) == 0 {
		return m.notice(emojiSetup, m.pick("No accounts yet", "还没有账户"), m.pick("Add one in the web app: Settings → Trading accounts", "请在网页上添加：设置 → 交易账户"))
	}
	open := 0
	for _, h := range holdings {
		open += len(h.Positions)
	}
	m.title(emojiPositions, m.pick(fmt.Sprintf("Positions · %d", open), fmt.Sprintf("持仓 · %d", open))).
		meta(m.pick(fmt.Sprintf("%d accounts", len(holdings)), fmt.Sprintf("%d 个账户", len(holdings))))
	var quiet []text
	for _, h := range holdings {
		name := h.Name + " · " + pick(lang, "Real money", "实盘")
		if h.Testnet {
			name = h.Name + " · " + pick(lang, "Testnet", "测试网")
		}
		switch {
		case !h.PositionsRead:
			quiet = append(quiet, m.fact(name, strong(m.pick("could not be read", "无法读取"))))
			continue
		case len(h.Positions) == 0:
			quiet = append(quiet, m.fact(name, strong(m.pick("nothing open", "没有持仓"))))
			continue
		}
		quantities := make([]float64, len(h.Positions))
		for i, p := range h.Positions {
			quantities[i] = p.Quantity
		}
		sizes := exacts(quantities)
		t := table{caption: name, columns: []column{
			{name: m.pick("Symbol", "品种")}, {name: m.pick("Side", "方向")},
			{name: m.pick("Size", "数量"), right: true}, {name: m.pick("P/L", "盈亏"), right: true},
		}}
		sum, summed := 0.0, true
		for i, p := range h.Positions {
			gain := "—"
			if p.HasGain {
				gain, sum = change(p.Gain), sum+p.Gain
			} else {
				summed = false
			}
			t.rows = append(t.rows, []string{p.Symbol, sideOf(p.Side, lang), sizes[i], gain})
		}
		if summed && len(h.Positions) > 1 {
			t.total = []string{m.pick("Total", "合计"), "", "", change(sum)}
		}
		m.table(t)
	}
	if len(quiet) > 0 {
		m.add(quiet...)
	}
	return m
}

func tradersMessage(traders []assistant.TraderState, err error, lang string) *message {
	m := newMessage(lang)
	if err != nil {
		return m.notice(emojiFailed, m.pick("Traders could not be read", "无法读取交易员"), m.pick("Try again in a moment", "请稍后再试"))
	}
	if len(traders) == 0 {
		return m.notice(emojiNothing, m.pick("No traders yet", "还没有交易员"), m.pick("Tell me what kind to create", "可以直接告诉我要创建什么样的"))
	}
	m.title(emojiTraders, m.pick("Traders · ", "交易员 · ")+runningOf(traders, lang))
	t := table{caption: m.pick("All", "全部"), columns: []column{{name: m.pick("State", "状态")}, {name: m.pick("Kind", "类型")}, {name: m.pick("Name", "名称")}}}
	for _, trader := range traders {
		state := m.pick("stopped", "停止")
		if trader.Running {
			state = m.pick("running", "运行")
		}
		kind := trader.Kind
		if kind == "" {
			kind = "—"
		}
		t.rows = append(t.rows, []string{state, kind, trader.Name})
	}
	return m.table(t)
}
