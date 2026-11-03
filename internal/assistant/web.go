package assistant

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"vergex/internal/agent"
)

// What only a surface with pages can do: suggest what goes in a form the page has open, and
// point at another page. Neither changes anything by itself.

const (
	skillFillForm = "fill_form"
	skillOpenPage = "open_page"
)

var webTools = []agent.Tool{
	tool(skillFillForm, "Suggest values for a form on the page the user has open, named under forms in [Current page]. The window shows them beside the current values. The user presses to fill them in, and saving stays on the page, so nothing is filled or saved by this.",
		object(map[string]any{
			"form":   str("the id of a form under forms in [Current page]"),
			"values": map[string]any{"type": "object", "description": "the fields to change and nothing else, by name. For a field of type object, only the keys to change, however deep, as in {\"config\":{\"risk_control\":{\"max_positions\":2}}}"},
			"reason": str("one short line saying why these values, in the user's language"),
		}, "form", "values")),
	tool(skillOpenPage, "Show a button to a page of the web app, when the next step is there, such as keys in Settings. It goes nowhere until the user presses it.",
		object(map[string]any{
			"page":   enum("which page", appPageNames()...),
			"params": map[string]any{"type": "object", "description": "what the page opens on: traders takes id and tab (performance, positions, decisions, orders, settings); futures takes venue, symbol, account, plan and tab; forecast takes symbol and interval; forecast_strategies takes id and tab (performance, rules); forecast_backtests takes run and strategy"},
		}, "page")),
}

// appPage is a place in the app that can be pointed at. The path is fixed here, so nothing
// the model writes can lead anywhere else. One starting with ? opens over the current page.
type appPage struct {
	path   string
	params []string
	en, zh string
}

var appPages = map[string]appPage{
	"dashboard":           {path: "/dashboard", en: "Open the Dashboard", zh: "打开总览"},
	"traders":             {path: "/traders", params: []string{"id", "tab"}, en: "Open Auto trading", zh: "打开自动交易"},
	"traders_overview":    {path: "/traders/overview", en: "Open the overview of traders", zh: "打开交易员总览"},
	"futures":             {path: "/futures", params: []string{"venue", "symbol", "account", "plan", "tab"}, en: "Open Futures", zh: "打开合约交易"},
	"forecast":            {path: "/forecast", params: []string{"symbol", "interval"}, en: "Open Forecast", zh: "打开预测"},
	"forecast_strategies": {path: "/forecast/strategies", params: []string{"id", "tab"}, en: "Open Strategies", zh: "打开策略"},
	"forecast_backtests":  {path: "/forecast/backtests", params: []string{"run", "strategy"}, en: "Open Backtests", zh: "打开回测"},
	"forecast_settings":   {path: "/forecast/settings", en: "Open Forecast settings", zh: "打开预测设置"},
	"help":                {path: "/help", en: "Open Help", zh: "打开帮助"},
	"settings_models":     {path: "?settings=models", en: "Open Agent models", zh: "打开模型设置"},
	"settings_accounts":   {path: "?settings=accounts", en: "Open Trading accounts", zh: "打开交易账户设置"},
	"settings_costs":      {path: "?settings=costs", en: "Open Agent costs", zh: "打开模型费用"},
	"settings_telegram":   {path: "?settings=telegram", en: "Open Telegram settings", zh: "打开 Telegram 设置"},
}

func appPageNames() []string {
	names := make([]string, 0, len(appPages))
	for name := range appPages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (a *Agent) openPage(arguments string) map[string]any {
	var args struct {
		Page   string            `json:"page"`
		Params map[string]string `json:"params"`
	}
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	page, ok := appPages[args.Page]
	if !ok {
		return map[string]any{"error": "unknown_page", "pages": appPageNames()}
	}
	query := url.Values{}
	for name, value := range args.Params {
		known := false
		for _, param := range page.params {
			known = known || param == name
		}
		if !known {
			return map[string]any{"error": "unknown_param", "param": name, "params": page.params}
		}
		query.Set(name, value)
	}
	to := page.path
	if len(query) > 0 {
		to += "?" + query.Encode()
	}
	a.addLink(to, status(a.zh(""), page.zh, page.en))
	return map[string]any{"status": "button_shown", "instruction": "The window shows a button to this page under your reply. Do not write its address."}
}

// Proposal is what the assistant suggests for a form, for the user to look over. Filling it
// in, and saving after that, stay with them.
type Proposal struct {
	Form   string         `json:"form"`
	Title  string         `json:"title"`
	Values map[string]any `json:"values"`
	Reason string         `json:"reason"`
}

// pageForm is a form as the page it is on declares it.
type pageForm struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Fields []formField `json:"fields"`
}

type formField struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Options []struct {
		Value string `json:"value"`
	} `json:"options"`
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Value any      `json:"value"`
}

func (a *Agent) pageForms() []pageForm {
	var page struct {
		Forms []pageForm `json:"forms"`
	}
	if a.page != "" {
		_ = json.Unmarshal([]byte(a.page), &page)
	}
	return page.Forms
}

// fillForm checks a suggestion against what the form declares, so a mistake goes back to the
// model rather than reaching the user. What the values mean is for the page's own checks,
// and the server's, when the user saves.
func (a *Agent) fillForm(arguments string) map[string]any {
	var args struct {
		Form   string         `json:"form"`
		Values map[string]any `json:"values"`
		Reason string         `json:"reason"`
	}
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	forms := a.pageForms()
	var form *pageForm
	open := make([]string, 0, len(forms))
	for i := range forms {
		open = append(open, forms[i].ID)
		if forms[i].ID == args.Form {
			form = &forms[i]
		}
	}
	if form == nil {
		return map[string]any{"error": "form_not_open", "open_forms": open, "instruction": "That form is not on the page the user has open. If another page has it, use open_page and ask them to go there first."}
	}
	if len(args.Values) == 0 {
		return missing("values")
	}
	fields := make(map[string]formField, len(form.Fields))
	for _, field := range form.Fields {
		fields[field.Name] = field
	}
	var problems []string
	for name, value := range args.Values {
		field, known := fields[name]
		if !known {
			problems = append(problems, "no field named "+name)
			continue
		}
		if problem := field.check(value); problem != "" {
			problems = append(problems, name+": "+problem)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return map[string]any{"error": "invalid_values", "problems": problems, "instruction": "Correct these and call fill_form again. Nothing was shown to the user."}
	}
	proposal := Proposal{Form: form.ID, Title: form.Title, Values: args.Values, Reason: truncate(args.Reason, 300)}
	// A second try at one form replaces the first.
	for i := range a.proposals {
		if a.proposals[i].Form == form.ID {
			a.proposals[i] = proposal
			return proposalShown
		}
	}
	a.proposals = append(a.proposals, proposal)
	return proposalShown
}

var proposalShown = map[string]any{
	"status":      "shown_to_user",
	"instruction": "The window shows these values beside the current ones under your reply, with a button to fill them in. Nothing has been filled or saved, and saving stays on the page. Do not list them again or say they are in.",
}

func (f formField) check(value any) string {
	switch f.Type {
	case "number":
		number, ok := value.(float64)
		if !ok {
			return "should be a number"
		}
		if f.Min != nil && number < *f.Min {
			return fmt.Sprintf("is below the least, %g", *f.Min)
		}
		if f.Max != nil && number > *f.Max {
			return fmt.Sprintf("is above the most, %g", *f.Max)
		}
	case "text":
		if _, ok := value.(string); !ok {
			return "should be text"
		}
	case "choice":
		text, ok := value.(string)
		if !ok {
			return "should be one of the options, by value"
		}
		options := make([]string, 0, len(f.Options))
		for _, option := range f.Options {
			if option.Value == text {
				return ""
			}
			options = append(options, option.Value)
		}
		return "should be one of " + strings.Join(options, ", ")
	case "list":
		items, ok := value.([]any)
		if !ok {
			return "should be a list"
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return "should be a list of text"
			}
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return "should be an object with only the keys to change"
		}
		return sameShape(f.Value, value, "")
	default:
		return "cannot be filled in"
	}
	return ""
}

// sameShape says where a value is of another kind than what it replaces, such as text for a
// number, since a form would only find out on saving. A key not there yet is left to the
// form and the server.
func sameShape(current, proposed any, path string) string {
	if current == nil || proposed == nil {
		return ""
	}
	switch old := current.(type) {
	case map[string]any:
		next, ok := proposed.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s should be an object", path)
		}
		for key, value := range next {
			inner := key
			if path != "" {
				inner = path + "." + key
			}
			if problem := sameShape(old[key], value, inner); problem != "" {
				return problem
			}
		}
	case []any:
		if _, ok := proposed.([]any); !ok {
			return fmt.Sprintf("%s should be a list", path)
		}
	case float64:
		if _, ok := proposed.(float64); !ok {
			return fmt.Sprintf("%s should be a number", path)
		}
	case string:
		if _, ok := proposed.(string); !ok {
			return fmt.Sprintf("%s should be text", path)
		}
	case bool:
		if _, ok := proposed.(bool); !ok {
			return fmt.Sprintf("%s should be true or false", path)
		}
	}
	return ""
}
