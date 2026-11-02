package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// The commands below read the local API and answer in fixed words. No model is involved, so
// they answer at once and cost nothing.

// position is the part of a venue's position these commands show. Venues name the fields
// differently, and give numbers as strings or numbers, so each is looked for under its names.
type position struct {
	symbol, side          string
	quantity, entry, gain float64
	hasGain               bool
}

func readNumber(row map[string]any, names ...string) (float64, bool) {
	for _, name := range names {
		switch value := row[name].(type) {
		case float64:
			return value, true
		case json.Number:
			if number, err := value.Float64(); err == nil {
				return number, true
			}
		case string:
			if number, err := strconv.ParseFloat(value, 64); err == nil {
				return number, true
			}
		}
	}
	return 0, false
}

func readPosition(row map[string]any) position {
	p := position{symbol: fmt.Sprint(row["symbol"]), side: strings.ToLower(fmt.Sprint(row["side"]))}
	p.quantity, _ = readNumber(row, "quantity", "positionAmt", "positionAmount", "size")
	p.entry, _ = readNumber(row, "entry_price", "entryPrice")
	p.gain, p.hasGain = readNumber(row, "unrealized_pnl", "unRealizedProfit", "unrealizedPnl")
	if p.quantity < 0 {
		p.quantity = -p.quantity
	}
	return p
}

func number(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func signedMoney(value float64) string {
	text := strconv.FormatFloat(value, 'f', 2, 64)
	if value >= 0 {
		return "+" + text
	}
	return text
}

// readPositions reads one account's open positions.
func (a *Agent) readPositions(accountID string) ([]position, error) {
	var rows []map[string]any
	if err := a.api.do("GET", "/api/accounts/"+url.PathEscape(accountID)+"/positions", nil, &rows); err != nil {
		return nil, err
	}
	out := make([]position, len(rows))
	for i, row := range rows {
		out[i] = readPosition(row)
	}
	return out, nil
}

func (a *Agent) accountTitle(e accountInfo, zh bool) string {
	if e.Testnet {
		return "**" + e.Name + "**" + status(zh, "（测试网）", " (testnet)")
	}
	return "**" + e.Name + "**"
}

// PositionsText is what every account holds, whoever opened it.
func (a *Agent) PositionsText() string {
	zh := a.zh("")
	accounts, err := a.accounts()
	if err != nil {
		return status(zh, "❌ 无法读取账户", "❌ Accounts could not be read")
	}
	if len(accounts) == 0 {
		return status(zh, "还没有账户。请在网页的 设置 → 交易账户 添加", "No accounts yet. Add one in the web app under Settings → Trading accounts")
	}
	blocks := make([]string, 0, len(accounts))
	for _, e := range accounts {
		lines := []string{a.accountTitle(e, zh)}
		positions, err := a.readPositions(e.ID)
		switch {
		case err != nil:
			lines = append(lines, status(zh, "⚠️ 无法读取", "⚠️ Could not be read"))
		case len(positions) == 0:
			lines = append(lines, status(zh, "没有持仓", "Nothing open"))
		}
		for _, p := range positions {
			line := fmt.Sprintf("- `%s` %s %s @ %s", p.symbol, p.side, number(p.quantity), number(p.entry))
			if p.hasGain {
				line += " · " + signedMoney(p.gain) + " USDT"
			}
			lines = append(lines, line)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// TradersText lists the traders and whether each runs.
func (a *Agent) TradersText() string {
	zh := a.zh("")
	traders, err := a.traders()
	if err != nil {
		return status(zh, "❌ 无法读取交易员", "❌ Traders could not be read")
	}
	if len(traders) == 0 {
		return status(zh, "还没有交易员。可以直接告诉我要创建什么样的", "No traders yet. Tell me what kind to create")
	}
	lines := make([]string, 0, len(traders)+1)
	running := 0
	for _, t := range traders {
		if t.IsRunning {
			running++
		}
	}
	lines = append(lines, status(zh,
		fmt.Sprintf("**交易员** %d 个运行中，共 %d 个", running, len(traders)),
		fmt.Sprintf("**Traders** %d running of %d", running, len(traders))))
	for _, t := range traders {
		state := status(zh, "已停止", "stopped")
		mark := "⚪"
		if t.IsRunning {
			state, mark = status(zh, "运行中", "running"), "🟢"
		}
		lines = append(lines, fmt.Sprintf("- %s %s · %s · %s", mark, t.Name, orDash(t.Kind), state))
	}
	return strings.Join(lines, "\n")
}

// StatusText is the state of things at a glance: what each account holds, in money and
// positions, and how many traders run.
func (a *Agent) StatusText() string {
	zh := a.zh("")
	accounts, err := a.accounts()
	if err != nil {
		return status(zh, "❌ 无法读取账户", "❌ Accounts could not be read")
	}
	states := a.accountStates()
	lines := []string{status(zh, "**账户**", "**Accounts**")}
	if len(accounts) == 0 {
		lines = append(lines, status(zh, "还没有账户", "None yet"))
	}
	open := 0
	readAll := true
	for _, e := range accounts {
		name := e.Name
		if e.Testnet {
			name += status(zh, "（测试网）", " (testnet)")
		}
		balance := status(zh, "无法读取", "could not be read")
		if state, ok := states[e.ID]; ok && state.Status == "ok" && state.Balance != "" {
			balance = state.Balance
		}
		lines = append(lines, "- "+name+": "+balance)
		positions, err := a.readPositions(e.ID)
		if err != nil {
			readAll = false
			continue
		}
		open += len(positions)
	}
	held := strconv.Itoa(open)
	if !readAll {
		held = status(zh, "至少 ", "at least ") + held
	}
	lines = append(lines, "", status(zh, "**持仓** "+held+" 个", "**Positions** "+held))
	if traders, err := a.traders(); err == nil {
		running := 0
		for _, t := range traders {
			if t.IsRunning {
				running++
			}
		}
		lines = append(lines, status(zh,
			fmt.Sprintf("**交易员** %d 个运行中，共 %d 个", running, len(traders)),
			fmt.Sprintf("**Traders** %d running of %d", running, len(traders))))
	}
	return strings.Join(lines, "\n")
}
