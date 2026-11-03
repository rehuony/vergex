package assistant

import "encoding/json"

// What the user reads while a skill runs. It comes from Go and names the action only, so
// neither the model's arguments nor a tool's data reach the screen.
type stageWords struct{ en, zh string }

var (
	stageWorking = stageWords{"Working…", "正在处理…"}

	traderStages = map[string]stageWords{
		"query":  {"Reading traders…", "正在读取交易员…"},
		"create": {"Creating the trader…", "正在创建交易员…"},
		"update": {"Updating the trader…", "正在修改交易员…"},
		"start":  {"Checking the trader…", "正在检查交易员…"},
		"stop":   {"Checking the trader…", "正在检查交易员…"},
		"delete": {"Checking the trader…", "正在检查交易员…"},
	}
	accountStages = map[string]stageWords{
		"overview":         {"Reading traders…", "正在读取交易员…"},
		"system":           {"Reading the overview…", "正在读取总览…"},
		"balance":          {"Reading the balance…", "正在读取余额…"},
		"positions":        {"Reading positions…", "正在读取持仓…"},
		"position_history": {"Reading closed positions…", "正在读取历史持仓…"},
		"trades":           {"Reading trades…", "正在读取成交…"},
		"orders":           {"Reading orders…", "正在读取订单…"},
		"open_orders":      {"Reading open orders…", "正在读取挂单…"},
		"decisions":        {"Reading decisions…", "正在读取决策…"},
		"decision_detail":  {"Reading a decision…", "正在读取决策详情…"},
		"statistics":       {"Reading statistics…", "正在读取统计…"},
		"agent_costs":      {"Reading model costs…", "正在读取模型费用…"},
	}
	chatStages = map[string]stageWords{
		"query":  {"Reading conversations…", "正在读取对话…"},
		"delete": {"Checking the conversations…", "正在核对对话…"},
		"clear":  {"Checking the conversations…", "正在核对对话…"},
	}
	skillStages = map[string]stageWords{
		skillModel:    {"Reading models…", "正在读取模型…"},
		skillAccounts: {"Reading accounts…", "正在读取账户…"},
		skillClose:    {"Checking the position…", "正在检查持仓…"},
		skillDiagnose: {"Looking into it…", "正在排查…"},
		skillFutures:  {"Reading Futures…", "正在读取合约交易…"},
		skillForecast: {"Reading forecasts…", "正在读取预测…"},
		skillPlans:    {"Checking the plans…", "正在核对计划…"},
		skillFillForm: {"Preparing the values…", "正在准备要填的内容…"},
		skillOpenPage: {"Finding the page…", "正在查找页面…"},
	}
)

// stageOf says what a skill call is doing, in words for the user.
func stageOf(skill, arguments string, zh bool) string {
	var args struct {
		Action string `json:"action"`
		Scope  string `json:"scope"`
	}
	_ = json.Unmarshal([]byte(arguments), &args)
	words, known := skillStages[skill]
	switch skill {
	case skillTrader:
		words, known = traderStages[args.Action]
	case skillAccount:
		words, known = accountStages[args.Scope]
	case skillChats:
		words, known = chatStages[args.Action]
	}
	if !known {
		words = stageWorking
	}
	return status(zh, words.zh, words.en)
}
