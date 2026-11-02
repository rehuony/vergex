package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"vergex/internal/agent"
	"vergex/internal/trader/kind"
)

// Skill-first tool layer (see AGENTS.md). The model chooses one of a few
// task-level skills; each skill owns its fixed API calls, target resolution,
// missing-field rules and confirmation gate. There is no generic API tool.

const (
	skillTrader   = "trader_management"
	skillModel    = "model_management"
	skillAccounts = "account_management"
	skillAccount  = "account_query"
	skillClose    = "close_position"
	skillDiagnose = "diagnose"
	skillFutures  = "futures_query"
	skillForecast = "forecast_query"
)

// maxResultChars bounds a single tool result fed back to the model.
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

func tool(name, desc string, params map[string]any) agent.Tool {
	return agent.Tool{Type: "function", Function: agent.FunctionDef{Name: name, Description: desc, Parameters: params}}
}

var skillTools = []agent.Tool{
	tool(skillTrader, "Create, update, delete, start, stop or query traders (automated trading bots). start/stop/delete, updates of a running trader and changes to risk settings are held for user confirmation by the system. Query a trader first to see its config.",
		object(map[string]any{
			"action":     enum("what to do", "create", "update", "delete", "start", "stop", "query"),
			"target_ref": refSchema,
			"slots": object(map[string]any{
				"name":                  str("trader name (create, or new name on update)"),
				"account":               str("account id or name"),
				"model":                 str("Model id or name"),
				"kind":                  enum("what runs the trader; only on create, default agent", "agent", "grid"),
				"preset":                str("starting config on create: careful, balanced or active for agent; default for grid"),
				"config_patch":          map[string]any{"type": "object", "description": "partial config, only the fields to change, e.g. {\"risk_control\":{\"max_positions\":2}} or {\"coin_source\":{\"static_coins\":[\"BTCUSDT\"]}}"},
				"scan_interval_minutes": map[string]any{"type": "integer", "description": "decision interval in minutes"},
				"initial_balance":       map[string]any{"type": "number", "description": "initial balance in USDT; omit to read it from the exchange"},
				"auto_start":            map[string]any{"type": "boolean", "description": "start right after create (still needs confirmation)"},
			}),
		}, "action")),
	tool(skillModel, "Query model configurations (provider, enabled, whether a key is set). Keys and endpoints can only be changed in the web UI.",
		object(map[string]any{
			"action":     enum("only query is available in chat", "query"),
			"target_ref": refSchema,
		}, "action")),
	tool(skillAccounts, "Query accounts with their connection status and balance, or what one account holds on the exchange, whoever opened it. Credentials can only be changed in the web UI.",
		object(map[string]any{
			"action":     enum("query for status and balance, positions for what an account holds", "query", "positions"),
			"target_ref": refSchema,
		}, "action")),
	tool(skillAccount, "Read trading data of one trader: balance, positions, history, trades, orders, decisions, statistics, agent costs. scope=overview lists all traders.",
		object(map[string]any{
			"scope":      enum("data to read", "overview", "balance", "positions", "position_history", "trades", "orders", "open_orders", "decisions", "statistics", "agent_costs"),
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
	tool(skillFutures, "Read Futures, the one-time trade plans made by hand or with the planning assistant: runs that are live or finished, drafts waiting for approval, and what finished plans realized on an account. Read-only: plans are approved and run in the web UI.",
		object(map[string]any{
			"scope":       enum("runs for plans sent to the exchange, drafts for plans not yet approved, results for what finished plans realized", "runs", "drafts", "results"),
			"account_ref": refSchema,
			"symbol":      str("optional symbol filter, e.g. BTCUSDT"),
		}, "scope")),
	tool(skillForecast, "Read Forecast's signals, which are experimental and connected to no trading: scores for the watched coins, or one coin's latest in detail. Read-only.",
		object(map[string]any{
			"scope":    enum("signals for all watched coins, detail for one coin", "signals", "detail"),
			"interval": str("optional timeframe of the signals, such as 1h or 4h"),
			"symbol":   str("the coin for detail, e.g. BTCUSDT"),
		}, "scope")),
	tool(skillDiagnose, "Run a fixed troubleshooting path and return findings: why a trader cannot start, model config problems, exchange API connection problems, or why orders are not executing.",
		object(map[string]any{
			"topic":      enum("problem area", "trader_start", "model", "account", "order_execution"),
			"target_ref": refSchema,
		}, "topic")),
}

// ── shared lookups ──────────────────────────────────────────────────────────

type traderInfo struct {
	TraderID   string  `json:"trader_id"`
	Name       string  `json:"trader_name"`
	AgentModel string  `json:"agent_model"`
	AccountID  string  `json:"account_id"`
	Kind       string  `json:"kind"`
	IsRunning  bool    `json:"is_running"`
	Initial    float64 `json:"initial_balance"`
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

type accountInfo struct {
	ID        string `json:"id"`
	Venue     string `json:"venue"`
	Name      string `json:"name"`
	HasAPIKey bool   `json:"has_api_key"`
	HasSecret bool   `json:"has_secret_key"`
	HasPass   bool   `json:"has_passphrase"`
	Testnet   bool   `json:"testnet"`
}

func (e accountInfo) label() string {
	name := e.Name
	if name == "" {
		name = e.Venue
	}
	if e.Testnet {
		return name + " (testnet)"
	}
	return name
}

func (a *Agent) traders() ([]traderInfo, error) {
	var out []traderInfo
	return out, a.api.do("GET", "/api/traders", nil, &out)
}

func (a *Agent) models() ([]modelInfo, error) {
	var out []modelInfo
	return out, a.api.do("GET", "/api/agent/models", nil, &out)
}

func (a *Agent) accounts() ([]accountInfo, error) {
	var out []accountInfo
	return out, a.api.do("GET", "/api/accounts", nil, &out)
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

func accountNames(items []accountInfo) []named {
	out := []named{}
	for _, e := range items {
		out = append(out, named{ID: e.ID, Name: e.label()})
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

// runSkill dispatches one tool call and returns the JSON result for the model.
func (a *Agent) runSkill(name, arguments string) string {
	var result map[string]any
	switch name {
	case skillTrader:
		result = a.traderManagement(arguments)
	case skillModel:
		result = a.modelManagement(arguments)
	case skillAccounts:
		result = a.accountManagement(arguments)
	case skillAccount:
		result = a.accountQuery(arguments)
	case skillClose:
		result = a.closePosition(arguments)
	case skillDiagnose:
		result = a.diagnose(arguments)
	case skillFutures:
		result = a.futuresQuery(arguments)
	case skillForecast:
		result = a.forecastQuery(arguments)
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
		Name                string         `json:"name"`
		Account             string         `json:"account"`
		Model               string         `json:"model"`
		Kind                string         `json:"kind"`
		Preset              string         `json:"preset"`
		ConfigPatch         map[string]any `json:"config_patch"`
		ScanIntervalMinutes int            `json:"scan_interval_minutes"`
		InitialBalance      float64        `json:"initial_balance"`
		AutoStart           bool           `json:"auto_start"`
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
		return a.hold(fmt.Sprintf("start trader \"%s\" (live trading, kind: %s)", t.Name, orDash(t.Kind)), func() (string, error) {
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
	accounts, err := a.accounts()
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
	account, rerr := resolve("account", accountNames(accounts), refOf(s.Account))
	if rerr != nil {
		problems["account"] = rerr
	}
	if len(missingFields) > 0 || len(problems) > 0 {
		out := map[string]any{"error": "cannot_create_yet", "instruction": "Ask the user for everything below in one message. Only enabled models and accounts are candidates."}
		if len(missingFields) > 0 {
			out["missing"] = missingFields
		}
		if len(problems) > 0 {
			out["problems"] = problems
		}
		return out
	}
	body := map[string]any{"name": s.Name, "agent_model_id": model.ID, "account_id": account.ID}
	if s.Kind != "" {
		body["kind"] = s.Kind
	}
	if s.Preset != "" {
		body["preset"] = s.Preset
	}
	if len(s.ConfigPatch) > 0 {
		body["config"] = s.ConfigPatch
	}
	if s.ScanIntervalMinutes > 0 {
		body["scan_interval_minutes"] = s.ScanIntervalMinutes
	}
	if s.InitialBalance > 0 {
		body["initial_balance"] = s.InitialBalance
	}
	var created struct {
		TraderID       string   `json:"trader_id"`
		Kind           string   `json:"kind"`
		StartupWarning string   `json:"startup_warning"`
		Warnings       []string `json:"warnings"`
	}
	if err := a.api.do("POST", "/api/traders", body, &created); err != nil {
		return errResult(err)
	}
	out := map[string]any{"status": "created", "trader_id": created.TraderID, "name": s.Name, "kind": created.Kind, "model": model.Name, "account": account.Name}
	if created.StartupWarning != "" {
		out["startup_warning"] = created.StartupWarning
	}
	if len(created.Warnings) > 0 {
		out["warnings"] = created.Warnings
	}
	if s.AutoStart {
		for k, v := range a.traderLifecycle("start", traderInfo{TraderID: created.TraderID, Name: s.Name, Kind: created.Kind}) {
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
	body := map[string]any{}
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
		body["agent_model_id"] = m.ID
		changes = append(changes, "model → "+m.Name)
	}
	if s.Account != "" {
		accounts, err := a.accounts()
		if err != nil {
			return errResult(err)
		}
		e, rerr := resolve("account", accountNames(accounts), refOf(s.Account))
		if rerr != nil {
			return toMap(rerr)
		}
		body["account_id"] = e.ID
		changes = append(changes, "account → "+e.Name)
	}
	if len(s.ConfigPatch) > 0 {
		body["config"] = s.ConfigPatch
		patch, _ := json.Marshal(s.ConfigPatch)
		changes = append(changes, "config "+truncate(string(patch), 400))
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
		return missing("at least one field to change (name, model, account, config_patch, scan_interval_minutes, initial_balance)")
	}
	apply := func() (string, error) {
		return "", a.api.do("PUT", "/api/traders/"+id, body, nil)
	}
	summary := fmt.Sprintf("update trader \"%s\": %s", t.Name, strings.Join(changes, ", "))
	risky := kind.TouchesRisk(t.Kind, s.ConfigPatch)
	if t.IsRunning {
		// Nothing but the config of a running agent trader changes without a restart.
		if t.Kind == kind.Agent && len(body) == 1 && len(s.ConfigPatch) > 0 {
			return a.hold(summary+" (it takes effect on the trader's next cycle)", apply)
		}
		return a.hold(summary+" (the running trader will be restarted)", apply)
	}
	if risky {
		return a.hold(summary, apply)
	}
	if _, err := apply(); err != nil {
		return errResult(err)
	}
	return map[string]any{"status": "updated", "summary": summary}
}

// ── model_management / account_management (read-only in chat) ─────────────

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

type accountState struct {
	Status       string  `json:"status"`
	Balance      string  `json:"display_balance"`
	Equity       float64 `json:"total_equity"`
	Available    float64 `json:"available_balance"`
	ErrorCode    string  `json:"error_code"`
	ErrorMessage string  `json:"error_message"`
}

func (a *Agent) accountStates() map[string]accountState {
	var out struct {
		States map[string]accountState `json:"states"`
	}
	if err := a.api.do("GET", "/api/accounts/state", nil, &out); err != nil {
		return nil
	}
	return out.States
}

func (a *Agent) accountManagement(arguments string) map[string]any {
	var args queryArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Action != "query" && args.Action != "positions" {
		return map[string]any{"error": "not_available_in_chat", "instruction": webOnly}
	}
	list, err := a.accounts()
	if err != nil {
		return errResult(err)
	}
	if !args.TargetRef.empty() {
		e, rerr := resolve("account", accountNames(list), args.TargetRef)
		if rerr != nil {
			return toMap(rerr)
		}
		for _, it := range list {
			if it.ID == e.ID {
				list = []accountInfo{it}
				break
			}
		}
	}
	if args.Action == "positions" {
		return a.accountPositions(list)
	}
	states := a.accountStates()
	rows := make([]map[string]any, 0, len(list))
	for _, e := range list {
		row := toMap(e)
		if st, ok := states[e.ID]; ok {
			row["state"] = st
		}
		rows = append(rows, row)
	}
	return map[string]any{"accounts": rows, "note": webOnly}
}

// ── account_query ───────────────────────────────────────────────────────────

type accountArgs struct {
	Scope     string     `json:"scope"`
	TraderRef *targetRef `json:"trader_ref"`
	Symbol    string     `json:"symbol"`
	Limit     int        `json:"limit"`
}

// accountPaths are what each scope reads under /api/traders/:id.
var accountPaths = map[string]string{
	"balance":          "/account",
	"positions":        "/open-positions",
	"position_history": "/positions",
	"trades":           "/trades",
	"orders":           "/orders",
	"open_orders":      "/open-orders",
	"decisions":        "/decisions/latest",
	"statistics":       "/statistics",
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
	if !ok && args.Scope != "agent_costs" {
		return map[string]any{"error": "unsupported_scope", "scope": args.Scope}
	}
	t, bad := a.resolveTrader(args.TraderRef)
	if bad != nil {
		return bad
	}
	path = "/api/traders/" + url.PathEscape(t.TraderID) + path
	q := url.Values{}
	if args.Scope == "agent_costs" {
		path = "/api/agent/costs"
		q.Set("trader_id", t.TraderID)
	}
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

// accountPositions reads what each account holds. One that cannot be read says so, and
// the others still answer.
func (a *Agent) accountPositions(list []accountInfo) map[string]any {
	rows := make([]map[string]any, 0, len(list))
	for _, e := range list {
		row := map[string]any{"account": e.label(), "venue": e.Venue, "testnet": e.Testnet}
		raw, err := a.api.raw("GET", "/api/accounts/"+url.PathEscape(e.ID)+"/positions", nil)
		if err != nil {
			row["error"] = err.Error()
		} else {
			row["positions"] = rawResult(raw)
		}
		rows = append(rows, row)
	}
	return map[string]any{"accounts": rows}
}

// ── futures_query ───────────────────────────────────────────────────────────

type futuresArgs struct {
	Scope      string     `json:"scope"`
	AccountRef *targetRef `json:"account_ref"`
	Symbol     string     `json:"symbol"`
}

type futuresPlan struct {
	ID         string `json:"id"`
	Quantity   string `json:"quantity"`
	RewardRisk string `json:"reward_risk"`
	Expires    string `json:"expires"`
	Idea       struct {
		Name   string `json:"name"`
		Side   string `json:"side"`
		Entry  string `json:"entry"`
		Stop   string `json:"stop"`
		Target string `json:"target"`
	} `json:"idea"`
	Snapshot struct {
		Symbol  string `json:"symbol"`
		Venue   string `json:"venue"`
		Testnet bool   `json:"testnet"`
	} `json:"snapshot"`
}

type futuresState struct {
	Records map[string]struct {
		Plan  futuresPlan `json:"plan"`
		State string      `json:"state"`
	} `json:"records"`
	Runs []struct {
		ID        string      `json:"id"`
		AccountID string      `json:"account_id"`
		State     string      `json:"state"`
		Error     string      `json:"error"`
		Plan      futuresPlan `json:"plan"`
		Orders    []struct {
			Role   string `json:"role"`
			Status string `json:"status"`
			Filled string `json:"filled"`
		} `json:"orders"`
	} `json:"runs"`
}

// futuresShown bounds how many runs or drafts one answer lists.
const futuresShown = 10

func planSummary(plan futuresPlan) map[string]any {
	return map[string]any{
		"name": plan.Idea.Name, "symbol": plan.Snapshot.Symbol, "venue": plan.Snapshot.Venue, "testnet": plan.Snapshot.Testnet,
		"side": plan.Idea.Side, "entry": plan.Idea.Entry, "stop": plan.Idea.Stop, "target": plan.Idea.Target,
		"quantity": plan.Quantity, "reward_risk": plan.RewardRisk,
	}
}

func (a *Agent) futuresQuery(arguments string) map[string]any {
	var args futuresArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Scope == "results" {
		return a.futuresResults(args.AccountRef)
	}
	if args.Scope != "runs" && args.Scope != "drafts" {
		return map[string]any{"error": "unsupported_scope", "scope": args.Scope}
	}
	raw, err := a.api.raw("GET", "/api/futures/state", nil)
	if err != nil {
		return errResult(err)
	}
	var state futuresState
	if err := json.Unmarshal(raw, &state); err != nil {
		return errResult(err)
	}
	symbol := strings.ToUpper(args.Symbol)
	rows := []map[string]any{}
	if args.Scope == "runs" {
		for _, run := range state.Runs {
			if symbol != "" && run.Plan.Snapshot.Symbol != symbol {
				continue
			}
			row := planSummary(run.Plan)
			row["state"], row["orders"] = run.State, run.Orders
			if run.Error != "" {
				row["error"] = run.Error
			}
			rows = append(rows, row)
		}
	} else {
		for _, record := range state.Records {
			if record.State != "draft" || (symbol != "" && record.Plan.Snapshot.Symbol != symbol) {
				continue
			}
			row := planSummary(record.Plan)
			row["expires"] = record.Plan.Expires
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["expires"]) > fmt.Sprint(rows[j]["expires"]) })
	}
	total := len(rows)
	if total > futuresShown {
		rows = rows[:futuresShown]
	}
	return map[string]any{"scope": args.Scope, "total": total, "shown": rows}
}

func (a *Agent) futuresResults(ref *targetRef) map[string]any {
	list, err := a.accounts()
	if err != nil {
		return errResult(err)
	}
	e, rerr := resolve("account", accountNames(list), ref)
	if rerr != nil {
		// With a single account there is nothing to ask.
		if len(list) != 1 || !ref.empty() {
			return toMap(rerr)
		}
		e = accountNames(list)[0]
	}
	raw, err := a.api.raw("GET", "/api/futures/results?account_id="+url.QueryEscape(e.ID), nil)
	if err != nil {
		return errResult(err)
	}
	return map[string]any{"account": e.Name, "results": rawResult(raw)}
}

// ── forecast_query ──────────────────────────────────────────────────────────

type forecastArgs struct {
	Scope    string `json:"scope"`
	Interval string `json:"interval"`
	Symbol   string `json:"symbol"`
}

func (a *Agent) forecastQuery(arguments string) map[string]any {
	var args forecastArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	q := url.Values{}
	if args.Interval != "" {
		q.Set("interval", args.Interval)
	}
	var path string
	switch args.Scope {
	case "signals":
		path = "/api/forecast/signals"
	case "detail":
		if args.Symbol == "" {
			return missing("symbol")
		}
		path = "/api/forecast/detail/" + url.PathEscape(strings.ToUpper(args.Symbol))
	default:
		return map[string]any{"error": "unsupported_scope", "scope": args.Scope}
	}
	raw, err := a.api.raw("GET", path+"?"+q.Encode(), nil)
	if err != nil {
		return errResult(err)
	}
	return map[string]any{"scope": args.Scope, "note": "Forecast is experimental and connected to no trading.", "data": rawResult(raw)}
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
	if err := a.api.do("GET", "/api/traders/"+url.PathEscape(t.TraderID)+"/open-positions", nil, &positions); err != nil {
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
