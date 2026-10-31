package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"vergex/internal/llm"
)

// Skill-first tool layer (see AGENTS.md). The LLM chooses one of a few
// task-level skills; each skill owns its fixed API calls, target resolution,
// missing-field rules and confirmation gate. There is no generic API tool.

const (
	skillTrader   = "trader_management"
	skillStrategy = "strategy_management"
	skillModel    = "model_management"
	skillExchange = "exchange_management"
	skillAccount  = "account_query"
	skillClose    = "close_position"
	skillDiagnose = "diagnose"
)

// maxResultChars bounds a single tool result fed back to the LLM.
const maxResultChars = 12000

func object(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": desc}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var refSchema = object(map[string]any{
	"id":   str("exact id, when known from the account state"),
	"name": str("name as the user said it"),
})

func tool(name, desc string, params map[string]any) llm.Tool {
	return llm.Tool{Type: "function", Function: llm.FunctionDef{Name: name, Description: desc, Parameters: params}}
}

var skillTools = []llm.Tool{
	tool(skillTrader, "Create, update, delete, start, stop or query traders (automated trading bots). start/stop/delete and updates of a running trader are held for user confirmation by the system.",
		object(map[string]any{
			"action":     enum("what to do", "create", "update", "delete", "start", "stop", "query"),
			"target_ref": refSchema,
			"slots": object(map[string]any{
				"name":                  str("trader name (create, or new name on update)"),
				"exchange":              str("exchange account id or name"),
				"model":                 str("AI model id or name"),
				"strategy":              str("strategy id or name"),
				"scan_interval_minutes": map[string]any{"type": "integer", "description": "decision interval in minutes"},
				"initial_balance":       map[string]any{"type": "number", "description": "initial balance in USDT; omit to read it from the exchange"},
				"auto_start":            map[string]any{"type": "boolean", "description": "start right after create (still needs confirmation)"},
			}),
		}, "action")),
	tool(skillStrategy, "Create, update, delete, activate, duplicate or query trading strategies. Updates change only the fields in config_patch; risk-control and grid changes are held for user confirmation.",
		object(map[string]any{
			"action":     enum("what to do", "create", "update", "delete", "activate", "duplicate", "query"),
			"target_ref": refSchema,
			"slots": object(map[string]any{
				"name":         str("strategy name (create / duplicate / rename)"),
				"description":  str("strategy description"),
				"config_patch": map[string]any{"type": "object", "description": "partial strategy config in the nested schema, e.g. {\"ai_config\":{\"risk_control\":{\"max_positions\":2}}}; query the strategy first to see field names"},
			}),
		}, "action")),
	tool(skillModel, "Query AI model configurations (provider, enabled, whether a key is set). Keys and endpoints can only be changed in the web UI.",
		object(map[string]any{
			"action":     enum("only query is available in chat", "query"),
			"target_ref": refSchema,
		}, "action")),
	tool(skillExchange, "Query exchange accounts with their connection status and balance. Credentials can only be changed in the web UI.",
		object(map[string]any{
			"action":     enum("only query is available in chat", "query"),
			"target_ref": refSchema,
		}, "action")),
	tool(skillAccount, "Read trading data of one trader: balance, positions, history, trades, orders, decisions, statistics, AI costs. scope=overview lists all traders.",
		object(map[string]any{
			"scope":      enum("data to read", "overview", "balance", "positions", "position_history", "trades", "orders", "open_orders", "decisions", "statistics", "ai_costs"),
			"trader_ref": refSchema,
			"symbol":     str("optional symbol filter, e.g. BTCUSDT"),
			"limit":      map[string]any{"type": "integer", "description": "optional row limit"},
		}, "scope")),
	tool(skillClose, "Close one open position of a trader at market. Always held for user confirmation.",
		object(map[string]any{
			"trader_ref": refSchema,
			"symbol":     str("position symbol, e.g. BTCUSDT"),
			"side":       enum("position side", "LONG", "SHORT"),
		}, "symbol", "side")),
	tool(skillDiagnose, "Run a fixed troubleshooting path and return findings: why a trader cannot start, model config problems, exchange API connection problems, or why orders are not executing.",
		object(map[string]any{
			"topic":      enum("problem area", "trader_start", "model", "exchange", "order_execution"),
			"target_ref": refSchema,
		}, "topic")),
}

// ── shared lookups ──────────────────────────────────────────────────────────

type traderInfo struct {
	TraderID     string  `json:"trader_id"`
	Name         string  `json:"trader_name"`
	AIModel      string  `json:"ai_model"`
	ExchangeID   string  `json:"exchange_id"`
	StrategyID   string  `json:"strategy_id"`
	StrategyName string  `json:"strategy_name"`
	IsRunning    bool    `json:"is_running"`
	Initial      float64 `json:"initial_balance"`
}

type modelInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Enabled         bool   `json:"enabled"`
	HasAPIKey       bool   `json:"has_api_key"`
	CustomAPIURL    string `json:"customApiUrl"`
	CustomModelName string `json:"customModelName"`
}

type exchangeInfo struct {
	ID           string `json:"id"`
	ExchangeType string `json:"exchange_type"`
	AccountName  string `json:"account_name"`
	Enabled      bool   `json:"enabled"`
	HasAPIKey    bool   `json:"has_api_key"`
	HasSecret    bool   `json:"has_secret_key"`
	HasPass      bool   `json:"has_passphrase"`
}

func (e exchangeInfo) label() string {
	if e.AccountName != "" {
		return e.AccountName
	}
	return e.ExchangeType
}

type strategyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	IsDefault   bool   `json:"is_default"`
}

func (a *Agent) traders() ([]traderInfo, error) {
	var out []traderInfo
	return out, a.api.do("GET", "/api/my-traders", nil, &out)
}

func (a *Agent) models() ([]modelInfo, error) {
	var out []modelInfo
	return out, a.api.do("GET", "/api/models", nil, &out)
}

func (a *Agent) exchanges() ([]exchangeInfo, error) {
	var out []exchangeInfo
	return out, a.api.do("GET", "/api/exchanges", nil, &out)
}

func (a *Agent) strategies() ([]strategyInfo, error) {
	var out struct {
		Strategies []strategyInfo `json:"strategies"`
	}
	return out.Strategies, a.api.do("GET", "/api/strategies", nil, &out)
}

func traderNames(items []traderInfo) []named {
	out := make([]named, 0, len(items))
	for _, t := range items {
		out = append(out, named{ID: t.TraderID, Name: t.Name})
	}
	return out
}

func modelNames(items []modelInfo, enabledOnly bool) []named {
	out := []named{}
	for _, m := range items {
		if !enabledOnly || m.Enabled {
			out = append(out, named{ID: m.ID, Name: m.Name})
		}
	}
	return out
}

func exchangeNames(items []exchangeInfo, enabledOnly bool) []named {
	out := []named{}
	for _, e := range items {
		if !enabledOnly || e.Enabled {
			out = append(out, named{ID: e.ID, Name: e.label()})
		}
	}
	return out
}

func strategyNames(items []strategyInfo) []named {
	out := make([]named, 0, len(items))
	for _, s := range items {
		out = append(out, named{ID: s.ID, Name: s.Name})
	}
	return out
}

func refOf(s string) *targetRef {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &targetRef{ID: s}
}

// resolveTrader finds the trader a skill acts on.
func (a *Agent) resolveTrader(ref *targetRef) (traderInfo, map[string]any) {
	list, err := a.traders()
	if err != nil {
		return traderInfo{}, errResult(err)
	}
	t, rerr := resolve("trader", traderNames(list), ref)
	if rerr != nil {
		return traderInfo{}, toMap(rerr)
	}
	for _, it := range list {
		if it.TraderID == t.ID {
			return it, nil
		}
	}
	return traderInfo{}, map[string]any{"error": "not_found", "kind": "trader"}
}

func errResult(err error) map[string]any {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		var body any
		if json.Unmarshal([]byte(apiErr.Body), &body) != nil {
			body = truncate(apiErr.Body, 500)
		}
		return map[string]any{"error": "api_error", "status": apiErr.Status, "detail": body}
	}
	return map[string]any{"error": "failed", "detail": err.Error()}
}

func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func rawResult(raw json.RawMessage) any {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	return v
}

// runSkill dispatches one tool call and returns the JSON result for the LLM.
func (a *Agent) runSkill(name, arguments string) string {
	var result map[string]any
	switch name {
	case skillTrader:
		result = a.traderManagement(arguments)
	case skillStrategy:
		result = a.strategyManagement(arguments)
	case skillModel:
		result = a.modelManagement(arguments)
	case skillExchange:
		result = a.exchangeManagement(arguments)
	case skillAccount:
		result = a.accountQuery(arguments)
	case skillClose:
		result = a.closePosition(arguments)
	case skillDiagnose:
		result = a.diagnose(arguments)
	default:
		result = map[string]any{"error": "unknown_skill", "detail": "available skills: " + strings.Join(skillNames(), ", ")}
	}
	b, err := json.Marshal(result)
	if err != nil {
		return `{"error":"failed to encode result"}`
	}
	return truncate(string(b), maxResultChars)
}

func skillNames() []string {
	out := make([]string, 0, len(skillTools))
	for _, t := range skillTools {
		out = append(out, t.Function.Name)
	}
	return out
}

func decodeArgs(arguments string, out any) map[string]any {
	if err := json.Unmarshal([]byte(arguments), out); err != nil {
		return map[string]any{"error": "invalid_arguments", "detail": err.Error()}
	}
	return nil
}

func missing(fields ...string) map[string]any {
	return map[string]any{"error": "missing_slots", "missing": fields, "instruction": "Ask the user for all missing fields in one message."}
}

// ── trader_management ───────────────────────────────────────────────────────

type traderArgs struct {
	Action    string     `json:"action"`
	TargetRef *targetRef `json:"target_ref"`
	Slots     struct {
		Name                string  `json:"name"`
		Exchange            string  `json:"exchange"`
		Model               string  `json:"model"`
		Strategy            string  `json:"strategy"`
		ScanIntervalMinutes int     `json:"scan_interval_minutes"`
		InitialBalance      float64 `json:"initial_balance"`
		AutoStart           bool    `json:"auto_start"`
	} `json:"slots"`
}

func (a *Agent) traderManagement(arguments string) map[string]any {
	var args traderArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	switch args.Action {
	case "query":
		if args.TargetRef.empty() {
			list, err := a.traders()
			if err != nil {
				return errResult(err)
			}
			return map[string]any{"traders": list}
		}
		t, bad := a.resolveTrader(args.TargetRef)
		if bad != nil {
			return bad
		}
		raw, err := a.api.raw("GET", "/api/traders/"+url.PathEscape(t.TraderID)+"/config", nil)
		if err != nil {
			return errResult(err)
		}
		return map[string]any{"trader": rawResult(raw)}
	case "create":
		return a.createTrader(args)
	case "update":
		return a.updateTrader(args)
	case "delete", "start", "stop":
		t, bad := a.resolveTrader(args.TargetRef)
		if bad != nil {
			return bad
		}
		return a.traderLifecycle(args.Action, t)
	}
	return map[string]any{"error": "unsupported_action", "action": args.Action}
}

func (a *Agent) traderLifecycle(action string, t traderInfo) map[string]any {
	id := url.PathEscape(t.TraderID)
	switch action {
	case "start":
		if t.IsRunning {
			return map[string]any{"status": "already_running", "trader": t.Name}
		}
		return a.hold(fmt.Sprintf("start trader \"%s\" (live trading, strategy: %s)", t.Name, orDash(t.StrategyName)), func() (string, error) {
			return "", a.api.do("POST", "/api/traders/"+id+"/start", nil, nil)
		})
	case "stop":
		if !t.IsRunning {
			return map[string]any{"status": "already_stopped", "trader": t.Name}
		}
		return a.hold(fmt.Sprintf("stop running trader \"%s\"", t.Name), func() (string, error) {
			return "", a.api.do("POST", "/api/traders/"+id+"/stop", nil, nil)
		})
	default: // delete
		summary := fmt.Sprintf("delete trader \"%s\"", t.Name)
		if t.IsRunning {
			summary += " (it is running and will be stopped)"
		}
		return a.hold(summary, func() (string, error) {
			return "", a.api.do("DELETE", "/api/traders/"+id, nil, nil)
		})
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (a *Agent) createTrader(args traderArgs) map[string]any {
	s := args.Slots
	models, err := a.models()
	if err != nil {
		return errResult(err)
	}
	exchanges, err := a.exchanges()
	if err != nil {
		return errResult(err)
	}
	strategies, err := a.strategies()
	if err != nil {
		return errResult(err)
	}
	problems := map[string]any{}
	var missingFields []string
	if strings.TrimSpace(s.Name) == "" {
		missingFields = append(missingFields, "name")
	}
	model, rerr := resolve("model", modelNames(models, true), refOf(s.Model))
	if rerr != nil {
		problems["model"] = rerr
	}
	exchange, rerr := resolve("exchange", exchangeNames(exchanges, true), refOf(s.Exchange))
	if rerr != nil {
		problems["exchange"] = rerr
	}
	strategyID := ""
	if s.Strategy != "" {
		st, rerr := resolve("strategy", strategyNames(strategies), refOf(s.Strategy))
		if rerr != nil {
			problems["strategy"] = rerr
		}
		strategyID = st.ID
	} else {
		for _, st := range strategies {
			if st.IsActive {
				strategyID = st.ID
			}
		}
	}
	if len(missingFields) > 0 || len(problems) > 0 {
		out := map[string]any{"error": "cannot_create_yet", "instruction": "Ask the user for everything below in one message. Only enabled models and exchanges are candidates."}
		if len(missingFields) > 0 {
			out["missing"] = missingFields
		}
		if len(problems) > 0 {
			out["problems"] = problems
		}
		return out
	}
	body := map[string]any{"name": s.Name, "ai_model_id": model.ID, "exchange_id": exchange.ID}
	if strategyID != "" {
		body["strategy_id"] = strategyID
	}
	if s.ScanIntervalMinutes > 0 {
		body["scan_interval_minutes"] = s.ScanIntervalMinutes
	}
	if s.InitialBalance > 0 {
		body["initial_balance"] = s.InitialBalance
	}
	var created struct {
		TraderID       string `json:"trader_id"`
		StartupWarning string `json:"startup_warning"`
	}
	if err := a.api.do("POST", "/api/traders", body, &created); err != nil {
		return errResult(err)
	}
	out := map[string]any{"status": "created", "trader_id": created.TraderID, "name": s.Name, "model": model.Name, "exchange": exchange.Name}
	if created.StartupWarning != "" {
		out["startup_warning"] = created.StartupWarning
	}
	if s.AutoStart {
		for k, v := range a.traderLifecycle("start", traderInfo{TraderID: created.TraderID, Name: s.Name}) {
			out[k] = v
		}
	}
	return out
}

func (a *Agent) updateTrader(args traderArgs) map[string]any {
	t, bad := a.resolveTrader(args.TargetRef)
	if bad != nil {
		return bad
	}
	id := url.PathEscape(t.TraderID)
	var cfg map[string]any
	if err := a.api.do("GET", "/api/traders/"+id+"/config", nil, &cfg); err != nil {
		return errResult(err)
	}
	body := map[string]any{
		"name":        cfg["trader_name"],
		"ai_model_id": cfg["ai_model"],
		"exchange_id": cfg["exchange_id"],
		"strategy_id": cfg["strategy_id"],
	}
	for _, k := range []string{"initial_balance", "scan_interval_minutes", "is_cross_margin", "btc_eth_leverage", "altcoin_leverage", "trading_symbols", "custom_prompt", "override_base_prompt"} {
		if v, ok := cfg[k]; ok {
			body[k] = v
		}
	}
	s := args.Slots
	var changes []string
	if s.Name != "" {
		body["name"] = s.Name
		changes = append(changes, "name → "+s.Name)
	}
	if s.Model != "" {
		models, err := a.models()
		if err != nil {
			return errResult(err)
		}
		m, rerr := resolve("model", modelNames(models, true), refOf(s.Model))
		if rerr != nil {
			return toMap(rerr)
		}
		body["ai_model_id"] = m.ID
		changes = append(changes, "model → "+m.Name)
	}
	if s.Exchange != "" {
		exchanges, err := a.exchanges()
		if err != nil {
			return errResult(err)
		}
		e, rerr := resolve("exchange", exchangeNames(exchanges, true), refOf(s.Exchange))
		if rerr != nil {
			return toMap(rerr)
		}
		body["exchange_id"] = e.ID
		changes = append(changes, "exchange → "+e.Name)
	}
	if s.Strategy != "" {
		strategies, err := a.strategies()
		if err != nil {
			return errResult(err)
		}
		st, rerr := resolve("strategy", strategyNames(strategies), refOf(s.Strategy))
		if rerr != nil {
			return toMap(rerr)
		}
		body["strategy_id"] = st.ID
		changes = append(changes, "strategy → "+st.Name)
	}
	if s.ScanIntervalMinutes > 0 {
		body["scan_interval_minutes"] = s.ScanIntervalMinutes
		changes = append(changes, fmt.Sprintf("scan interval → %d min", s.ScanIntervalMinutes))
	}
	if s.InitialBalance > 0 {
		body["initial_balance"] = s.InitialBalance
		changes = append(changes, fmt.Sprintf("initial balance → %g", s.InitialBalance))
	}
	if len(changes) == 0 {
		return missing("at least one field to change (name, model, exchange, strategy, scan_interval_minutes, initial_balance)")
	}
	apply := func() (string, error) {
		return "", a.api.do("PUT", "/api/traders/"+id, body, nil)
	}
	summary := fmt.Sprintf("update trader \"%s\": %s", t.Name, strings.Join(changes, ", "))
	if t.IsRunning {
		return a.hold(summary+" (the running trader will be restarted)", apply)
	}
	if _, err := apply(); err != nil {
		return errResult(err)
	}
	return map[string]any{"status": "updated", "summary": summary}
}

// ── strategy_management ─────────────────────────────────────────────────────

type strategyArgs struct {
	Action    string     `json:"action"`
	TargetRef *targetRef `json:"target_ref"`
	Slots     struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		ConfigPatch map[string]any `json:"config_patch"`
	} `json:"slots"`
}

// riskKeys are config sections whose changes affect position sizing or
// leverage of live traders and therefore need confirmation.
var riskKeys = map[string]bool{"risk_control": true, "grid_config": true, "strategy_type": true}

func touchesRisk(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for k, child := range m {
		if riskKeys[k] || touchesRisk(child) {
			return true
		}
	}
	return false
}

func (a *Agent) resolveStrategy(ref *targetRef) (strategyInfo, map[string]any) {
	list, err := a.strategies()
	if err != nil {
		return strategyInfo{}, errResult(err)
	}
	st, rerr := resolve("strategy", strategyNames(list), ref)
	if rerr != nil {
		return strategyInfo{}, toMap(rerr)
	}
	for _, it := range list {
		if it.ID == st.ID {
			return it, nil
		}
	}
	return strategyInfo{}, map[string]any{"error": "not_found", "kind": "strategy"}
}

func (a *Agent) strategyManagement(arguments string) map[string]any {
	var args strategyArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	s := args.Slots
	if args.Action == "create" {
		if strings.TrimSpace(s.Name) == "" {
			return missing("name")
		}
		var created struct {
			ID       string   `json:"id"`
			Warnings []string `json:"warnings"`
		}
		if err := a.api.do("POST", "/api/strategies", map[string]any{"name": s.Name, "description": s.Description}, &created); err != nil {
			return errResult(err)
		}
		out := map[string]any{"status": "created", "strategy_id": created.ID, "name": s.Name, "config": "system defaults"}
		if len(s.ConfigPatch) > 0 {
			if err := a.api.do("PUT", "/api/strategies/"+url.PathEscape(created.ID), map[string]any{"config": s.ConfigPatch}, nil); err != nil {
				out["config_error"] = errResult(err)
			} else {
				out["config"] = "defaults with the requested changes"
			}
		}
		if len(created.Warnings) > 0 {
			out["warnings"] = created.Warnings
		}
		return out
	}
	if args.Action == "query" && args.TargetRef.empty() {
		list, err := a.strategies()
		if err != nil {
			return errResult(err)
		}
		return map[string]any{"strategies": list}
	}
	st, bad := a.resolveStrategy(args.TargetRef)
	if bad != nil {
		return bad
	}
	id := url.PathEscape(st.ID)
	switch args.Action {
	case "query":
		raw, err := a.api.raw("GET", "/api/strategies/"+id, nil)
		if err != nil {
			return errResult(err)
		}
		return map[string]any{"strategy": rawResult(raw)}
	case "update":
		body := map[string]any{}
		var changes []string
		if s.Name != "" {
			body["name"] = s.Name
			changes = append(changes, "name → "+s.Name)
		}
		if s.Description != "" {
			body["description"] = s.Description
			changes = append(changes, "description")
		}
		if len(s.ConfigPatch) > 0 {
			body["config"] = s.ConfigPatch
			patch, _ := json.Marshal(s.ConfigPatch)
			changes = append(changes, "config "+truncate(string(patch), 400))
		}
		if len(changes) == 0 {
			return missing("name, description or config_patch")
		}
		apply := func() (string, error) {
			return "", a.api.do("PUT", "/api/strategies/"+id, body, nil)
		}
		summary := fmt.Sprintf("update strategy \"%s\": %s", st.Name, strings.Join(changes, "; "))
		if touchesRisk(s.ConfigPatch) {
			return a.hold(summary, apply)
		}
		if _, err := apply(); err != nil {
			return errResult(err)
		}
		return map[string]any{"status": "updated", "summary": summary}
	case "delete":
		return a.hold(fmt.Sprintf("delete strategy \"%s\"", st.Name), func() (string, error) {
			return "", a.api.do("DELETE", "/api/strategies/"+id, nil, nil)
		})
	case "activate":
		if err := a.api.do("POST", "/api/strategies/"+id+"/activate", nil, nil); err != nil {
			return errResult(err)
		}
		return map[string]any{"status": "activated", "strategy": st.Name}
	case "duplicate":
		name := s.Name
		if name == "" {
			name = st.Name + " copy"
		}
		var dup struct {
			ID string `json:"id"`
		}
		if err := a.api.do("POST", "/api/strategies/"+id+"/duplicate", map[string]any{"name": name}, &dup); err != nil {
			return errResult(err)
		}
		return map[string]any{"status": "duplicated", "strategy_id": dup.ID, "name": name}
	}
	return map[string]any{"error": "unsupported_action", "action": args.Action}
}

// ── model_management / exchange_management (read-only in chat) ─────────────

type queryArgs struct {
	Action    string     `json:"action"`
	TargetRef *targetRef `json:"target_ref"`
}

const webOnly = "Changing API keys, endpoints or credentials is only possible in the web UI (Settings). Tell the user where to do it."

func (a *Agent) modelManagement(arguments string) map[string]any {
	var args queryArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Action != "query" {
		return map[string]any{"error": "not_available_in_chat", "instruction": webOnly}
	}
	list, err := a.models()
	if err != nil {
		return errResult(err)
	}
	if args.TargetRef.empty() {
		return map[string]any{"models": list, "note": webOnly}
	}
	m, rerr := resolve("model", modelNames(list, false), args.TargetRef)
	if rerr != nil {
		return toMap(rerr)
	}
	for _, it := range list {
		if it.ID == m.ID {
			return map[string]any{"model": it, "note": webOnly}
		}
	}
	return map[string]any{"error": "not_found"}
}

type exchangeState struct {
	Status       string  `json:"status"`
	Balance      string  `json:"display_balance"`
	Equity       float64 `json:"total_equity"`
	Available    float64 `json:"available_balance"`
	ErrorCode    string  `json:"error_code"`
	ErrorMessage string  `json:"error_message"`
}

func (a *Agent) exchangeStates() map[string]exchangeState {
	var out struct {
		States map[string]exchangeState `json:"states"`
	}
	if err := a.api.do("GET", "/api/exchanges/account-state", nil, &out); err != nil {
		return nil
	}
	return out.States
}

func (a *Agent) exchangeManagement(arguments string) map[string]any {
	var args queryArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Action != "query" {
		return map[string]any{"error": "not_available_in_chat", "instruction": webOnly}
	}
	list, err := a.exchanges()
	if err != nil {
		return errResult(err)
	}
	if !args.TargetRef.empty() {
		e, rerr := resolve("exchange", exchangeNames(list, false), args.TargetRef)
		if rerr != nil {
			return toMap(rerr)
		}
		for _, it := range list {
			if it.ID == e.ID {
				list = []exchangeInfo{it}
				break
			}
		}
	}
	states := a.exchangeStates()
	rows := make([]map[string]any, 0, len(list))
	for _, e := range list {
		row := toMap(e)
		if st, ok := states[e.ID]; ok {
			row["state"] = st
		}
		rows = append(rows, row)
	}
	return map[string]any{"exchanges": rows, "note": webOnly}
}

// ── account_query ───────────────────────────────────────────────────────────

type accountArgs struct {
	Scope     string     `json:"scope"`
	TraderRef *targetRef `json:"trader_ref"`
	Symbol    string     `json:"symbol"`
	Limit     int        `json:"limit"`
}

var accountPaths = map[string]string{
	"balance":          "/api/account",
	"positions":        "/api/positions",
	"position_history": "/api/positions/history",
	"trades":           "/api/trades",
	"orders":           "/api/orders",
	"open_orders":      "/api/open-orders",
	"decisions":        "/api/decisions/latest",
	"statistics":       "/api/statistics",
	"ai_costs":         "/api/ai-costs",
}

func (a *Agent) accountQuery(arguments string) map[string]any {
	var args accountArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Scope == "overview" {
		list, err := a.traders()
		if err != nil {
			return errResult(err)
		}
		return map[string]any{"traders": list}
	}
	path, ok := accountPaths[args.Scope]
	if !ok {
		return map[string]any{"error": "unsupported_scope", "scope": args.Scope}
	}
	t, bad := a.resolveTrader(args.TraderRef)
	if bad != nil {
		return bad
	}
	q := url.Values{"trader_id": {t.TraderID}}
	if args.Symbol != "" {
		q.Set("symbol", strings.ToUpper(args.Symbol))
	}
	if args.Limit > 0 {
		q.Set("limit", fmt.Sprint(args.Limit))
	}
	raw, err := a.api.raw("GET", path+"?"+q.Encode(), nil)
	if err != nil {
		return errResult(err)
	}
	return map[string]any{"trader": t.Name, "scope": args.Scope, "data": rawResult(raw)}
}

// ── close_position ──────────────────────────────────────────────────────────

type closeArgs struct {
	TraderRef *targetRef `json:"trader_ref"`
	Symbol    string     `json:"symbol"`
	Side      string     `json:"side"`
}

func (a *Agent) closePosition(arguments string) map[string]any {
	var args closeArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	symbol := strings.ToUpper(strings.TrimSpace(args.Symbol))
	side := strings.ToUpper(strings.TrimSpace(args.Side))
	var missingFields []string
	if symbol == "" {
		missingFields = append(missingFields, "symbol")
	}
	if side != "LONG" && side != "SHORT" {
		missingFields = append(missingFields, "side (LONG or SHORT)")
	}
	if len(missingFields) > 0 {
		return missing(missingFields...)
	}
	t, bad := a.resolveTrader(args.TraderRef)
	if bad != nil {
		return bad
	}
	var positions []map[string]any
	if err := a.api.do("GET", "/api/positions?trader_id="+url.QueryEscape(t.TraderID), nil, &positions); err != nil {
		return errResult(err)
	}
	var open []string
	found := false
	for _, p := range positions {
		ps := strings.ToUpper(fmt.Sprint(p["symbol"]))
		pside := strings.ToUpper(fmt.Sprint(p["side"]))
		open = append(open, ps+" "+pside)
		if ps == symbol && pside == side {
			found = true
		}
	}
	if !found {
		sort.Strings(open)
		return map[string]any{"error": "position_not_found", "trader": t.Name, "open_positions": open}
	}
	id := url.PathEscape(t.TraderID)
	return a.hold(fmt.Sprintf("close %s %s position of trader \"%s\" at market", symbol, side, t.Name), func() (string, error) {
		return "", a.api.do("POST", "/api/traders/"+id+"/close-position", map[string]any{"symbol": symbol, "side": side}, nil)
	})
}
