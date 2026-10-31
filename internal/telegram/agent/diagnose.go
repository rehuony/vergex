package agent

import (
	"encoding/json"
	"net/url"
)

// diagnose runs a fixed troubleshooting path per topic and returns the facts
// it collected plus hints keyed to known failure states. The LLM explains the
// findings; it does not decide which checks to run.

type diagnoseArgs struct {
	Topic     string     `json:"topic"`
	TargetRef *targetRef `json:"target_ref"`
}

type finding struct {
	Check  string `json:"check"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

var exchangeHints = map[string]string{
	"disabled":            "The account is disabled. Enable it in the web UI (Settings → Exchanges).",
	"missing_credentials": "API key or secret (and passphrase for OKX) is missing. Fill them in the web UI.",
	"invalid_credentials": "The exchange rejected the key: it may be wrong, revoked, or the server IP is not in the key's IP whitelist. Compare the key in the web UI and whitelist the server IP.",
	"permission_denied":   "The API key lacks permission. Enable futures trading (and reading) for the key; withdrawals are not needed.",
	"unavailable":         "The exchange could not be reached or timed out. Check network/proxy and server clock, then retry.",
}

func (a *Agent) diagnose(arguments string) map[string]any {
	var args diagnoseArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	switch args.Topic {
	case "trader_start":
		return a.diagnoseTraderStart(args.TargetRef)
	case "model":
		return a.diagnoseModel(args.TargetRef)
	case "exchange":
		return a.diagnoseExchange(args.TargetRef)
	case "order_execution":
		return a.diagnoseOrders(args.TargetRef)
	}
	return map[string]any{"error": "unsupported_topic", "topic": args.Topic}
}

func (a *Agent) diagnoseTraderStart(ref *targetRef) map[string]any {
	t, bad := a.resolveTrader(ref)
	if bad != nil {
		return bad
	}
	var preflight struct {
		Ready  bool `json:"ready"`
		Checks []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"checks"`
	}
	if err := a.api.do("GET", "/api/traders/"+url.PathEscape(t.TraderID)+"/preflight", nil, &preflight); err != nil {
		return errResult(err)
	}
	findings := []finding{}
	for _, c := range preflight.Checks {
		f := finding{Check: c.ID, OK: c.Status == "ok" || c.Status == "skipped", Detail: c.Message}
		if !f.OK && c.ID == "exchange_account" {
			f.Hint = exchangeHints[c.Code]
		}
		findings = append(findings, f)
	}
	return map[string]any{"topic": "trader_start", "trader": t.Name, "running": t.IsRunning, "ready": preflight.Ready, "findings": findings}
}

func (a *Agent) diagnoseModel(ref *targetRef) map[string]any {
	models, err := a.models()
	if err != nil {
		return errResult(err)
	}
	if !ref.empty() {
		m, rerr := resolve("model", modelNames(models, false), ref)
		if rerr != nil {
			return toMap(rerr)
		}
		for _, it := range models {
			if it.ID == m.ID {
				models = []modelInfo{it}
				break
			}
		}
	}
	traders, _ := a.traders()
	findings := []finding{}
	for _, m := range models {
		users := []string{}
		for _, t := range traders {
			if t.AIModel == m.ID {
				users = append(users, t.Name)
			}
		}
		usedBy := ""
		if len(users) > 0 {
			b, _ := json.Marshal(users)
			usedBy = "used by traders " + string(b)
		}
		switch {
		case !m.Enabled:
			findings = append(findings, finding{Check: m.Name, Detail: "disabled; " + usedBy, Hint: "Enable the model in the web UI (Settings → AI models)."})
		case !m.HasAPIKey:
			findings = append(findings, finding{Check: m.Name, Detail: "enabled but no API key; " + usedBy, Hint: "Add the provider API key in the web UI."})
		case m.CustomAPIURL != "" || m.CustomModelName != "":
			findings = append(findings, finding{Check: m.Name, OK: true, Detail: "custom endpoint " + m.CustomAPIURL + " model " + m.CustomModelName + "; " + usedBy,
				Hint: "If calls fail, check that the base URL is OpenAI-compatible and the model name exists for that key."})
		default:
			findings = append(findings, finding{Check: m.Name, OK: true, Detail: "enabled with API key; " + usedBy})
		}
	}
	return map[string]any{"topic": "model", "findings": findings,
		"note": "Failures at call time (401, 429, quota) show up in the trader's latest decisions; use diagnose order_execution or account_query decisions."}
}

func (a *Agent) diagnoseExchange(ref *targetRef) map[string]any {
	exchanges, err := a.exchanges()
	if err != nil {
		return errResult(err)
	}
	if !ref.empty() {
		e, rerr := resolve("exchange", exchangeNames(exchanges, false), ref)
		if rerr != nil {
			return toMap(rerr)
		}
		for _, it := range exchanges {
			if it.ID == e.ID {
				exchanges = []exchangeInfo{it}
				break
			}
		}
	}
	states := a.exchangeStates()
	findings := []finding{}
	for _, e := range exchanges {
		st, ok := states[e.ID]
		f := finding{Check: e.label() + " (" + e.ExchangeType + ")"}
		switch {
		case !ok:
			f.Detail = "no state returned"
		case st.Status == "ok":
			f.OK = true
			f.Detail = "connected, balance " + st.Balance
		default:
			f.Detail = st.Status
			if st.ErrorMessage != "" {
				f.Detail += ": " + st.ErrorMessage
			}
			f.Hint = exchangeHints[st.Status]
		}
		if e.ExchangeType == "okx" && !e.HasPass {
			f.OK = false
			f.Hint = exchangeHints["missing_credentials"]
		}
		findings = append(findings, f)
	}
	out := map[string]any{"topic": "exchange", "findings": findings}
	if raw, err := a.api.raw("GET", "/api/server-ip", nil); err == nil {
		out["server_ip"] = rawResult(raw)
	}
	return out
}

func (a *Agent) diagnoseOrders(ref *targetRef) map[string]any {
	t, bad := a.resolveTrader(ref)
	if bad != nil {
		return bad
	}
	q := "?trader_id=" + url.QueryEscape(t.TraderID)
	out := map[string]any{"topic": "order_execution", "trader": t.Name, "running": t.IsRunning}
	if !t.IsRunning {
		out["hint"] = "The trader is stopped, so no orders are placed. Start it first (trader_management start)."
	}
	for key, path := range map[string]string{
		"status":           "/api/status" + q,
		"latest_decisions": "/api/decisions/latest" + q + "&limit=5",
		"open_orders":      "/api/open-orders" + q,
	} {
		raw, err := a.api.raw("GET", path, nil)
		if err != nil {
			out[key] = errResult(err)
			continue
		}
		out[key] = rawResult(raw)
	}
	out["note"] = "Look for AI call errors, safe mode after repeated AI failures, risk-control rejections, insufficient margin or exchange errors in the decisions."
	return out
}
