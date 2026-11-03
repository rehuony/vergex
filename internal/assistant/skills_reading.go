package assistant

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// What Forecast and Futures can tell the assistant beyond a coin's call or a run's state.
// All of it only reads, and every figure is the server's: the model writes what it means.

const (
	// forecastNote travels with whatever Forecast says, so the model reads it as it should
	// be read.
	forecastNote = "Forecast is experimental and connected to no trading. confidence is how far the score is from 50, not a probability, so call it strength. A hit rate means nothing until it beats chance_rate, and figures from fewer than 30 moments say little: name both whenever you give one."
	// forecastCoinsShown bounds how many coins a read of signals lists, strongest first.
	forecastCoinsShown = 40
	// backtestsShown bounds how many runs a read of backtests lists.
	backtestsShown = 20
	// recentSignalsShown bounds how many of a coin's recent calls a read of detail lists.
	recentSignalsShown = 20
	// comparedPlans bounds how many plans one comparison lays out.
	comparedPlans = 8
)

// ── forecast_query ──────────────────────────────────────────────────────────

type forecastCoinRow struct {
	Symbol  string   `json:"symbol"`
	Price   *float64 `json:"price"`
	Change  *float64 `json:"change_24h_pct"`
	Stale   bool     `json:"stale,omitempty"`
	Pending string   `json:"pending,omitempty"`
	Signal  *struct {
		Direction       string  `json:"direction"`
		Score           float64 `json:"display_prob"`
		Calibrated      bool    `json:"calibrated"`
		Confidence      int     `json:"confidence"`
		ExpectedMovePct float64 `json:"expected_move_pct"`
		Regime          string  `json:"regime"`
	} `json:"signal"`
}

// forecastSignals lists the tracked coins, strongest calls first, each as far as a
// comparison needs it. A coin's whole reading is what detail is for.
func (a *Agent) forecastSignals(query url.Values) map[string]any {
	var out struct {
		StrategyID string            `json:"strategy_id"`
		Interval   string            `json:"interval"`
		Coins      []forecastCoinRow `json:"coins"`
	}
	if err := a.api.do("GET", "/api/forecast/signals?"+query.Encode(), nil, &out); err != nil {
		return errResult(err)
	}
	strength := func(row forecastCoinRow) float64 {
		if row.Signal == nil {
			return -1
		}
		if row.Signal.Score > 50 {
			return row.Signal.Score - 50
		}
		return 50 - row.Signal.Score
	}
	sort.SliceStable(out.Coins, func(i, j int) bool { return strength(out.Coins[i]) > strength(out.Coins[j]) })
	total := len(out.Coins)
	if total > forecastCoinsShown {
		out.Coins = out.Coins[:forecastCoinsShown]
	}
	return map[string]any{"scope": "signals", "note": forecastNote, "strategy_id": out.StrategyID, "interval": out.Interval, "total": total, "coins": out.Coins}
}

// forecastDetail is a coin's reading and its recent calls, without the candles behind them,
// which would fill the whole result and push out what they came to.
func (a *Agent) forecastDetail(symbol string, query url.Values) map[string]any {
	if symbol == "" {
		return missing("symbol")
	}
	var detail map[string]any
	if err := a.api.do("GET", "/api/forecast/detail/"+url.PathEscape(strings.ToUpper(symbol))+"?"+query.Encode(), nil, &detail); err != nil {
		return errResult(err)
	}
	for _, heavy := range []string{"candles", "forming", "projection", "indicators"} {
		delete(detail, heavy)
	}
	if signals, ok := detail["signals"].([]any); ok && len(signals) > recentSignalsShown {
		detail["signals"] = signals[:recentSignalsShown]
	}
	return map[string]any{"scope": "detail", "note": forecastNote, "data": detail}
}

type forecastStrategy struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Retired bool            `json:"retired"`
	Created int64           `json:"created_at"`
	Config  json.RawMessage `json:"config"`
}

func (a *Agent) forecastStrategies() (string, []forecastStrategy, error) {
	var out struct {
		Active     string             `json:"active_strategy_id"`
		Strategies []forecastStrategy `json:"strategies"`
	}
	err := a.api.do("GET", "/api/forecast/strategies", nil, &out)
	return out.Active, out.Strategies, err
}

// forecastStrategyList names the saved strategies. A strategy's rules come with how it has
// done, from performance.
func (a *Agent) forecastStrategyList() map[string]any {
	active, strategies, err := a.forecastStrategies()
	if err != nil {
		return errResult(err)
	}
	rows := make([]map[string]any, 0, len(strategies))
	for _, s := range strategies {
		rows = append(rows, map[string]any{"id": s.ID, "name": s.Name, "in_use": s.ID == active, "retired": s.Retired, "created_at": s.Created})
	}
	return map[string]any{"scope": "strategies", "note": forecastNote, "strategies": rows}
}

// forecastPerformance is one strategy's rules, how it has done live, and its latest backtest
// to set that beside. With no strategy named it is the one in use.
func (a *Agent) forecastPerformance(ref *targetRef) map[string]any {
	active, strategies, err := a.forecastStrategies()
	if err != nil {
		return errResult(err)
	}
	if ref.empty() && active != "" {
		ref = &targetRef{ID: active}
	}
	names := make([]named, 0, len(strategies))
	for _, s := range strategies {
		names = append(names, named{ID: s.ID, Name: s.Name})
	}
	picked, rerr := resolve("strategy", names, ref)
	if rerr != nil {
		return toMap(rerr)
	}
	raw, err := a.api.raw("GET", "/api/forecast/strategies/"+url.PathEscape(picked.ID)+"/performance", nil)
	if err != nil {
		return errResult(err)
	}
	out := map[string]any{"scope": "performance", "note": forecastNote, "performance": rawResult(raw)}
	for _, s := range strategies {
		if s.ID == picked.ID {
			out["strategy"] = map[string]any{"id": s.ID, "name": s.Name, "in_use": s.ID == active, "retired": s.Retired, "rules": rawResult(s.Config)}
		}
	}
	if rows, err := a.backtestRows(); err == nil {
		for _, row := range rows {
			if row["strategy_id"] == picked.ID && row["status"] == "done" {
				out["latest_backtest"] = row
				break
			}
		}
	}
	return out
}

// backtestRows lists the latest runs, newest first, each with its overall line.
func (a *Agent) backtestRows() ([]map[string]any, error) {
	var out struct {
		Backtests []map[string]any `json:"backtests"`
	}
	if err := a.api.do("GET", "/api/forecast/backtests", nil, &out); err != nil {
		return nil, err
	}
	return out.Backtests, nil
}

func (a *Agent) forecastBacktests() map[string]any {
	rows, err := a.backtestRows()
	if err != nil {
		return errResult(err)
	}
	total := len(rows)
	if total > backtestsShown {
		rows = rows[:backtestsShown]
	}
	return map[string]any{"scope": "backtests", "note": forecastNote, "total": total, "backtests": rows}
}

func (a *Agent) forecastBacktest(id int64) map[string]any {
	if id <= 0 {
		return missing("backtest_id")
	}
	raw, err := a.api.raw("GET", fmt.Sprintf("/api/forecast/backtests/%d", id), nil)
	if err != nil {
		return errResult(err)
	}
	return map[string]any{"scope": "backtest", "note": forecastNote, "data": rawResult(raw)}
}

// ── futures_query compare ───────────────────────────────────────────────────

// futuresCompare lays plans out side by side. With none named it is the latest drafts still
// waiting on a market. Every figure comes from the plan compiler, so nothing is worked out
// here: which is better is for the model to say, and the user to decide on the page.
func (a *Agent) futuresCompare(state futuresState, ids []string, symbol string) map[string]any {
	found := map[string]futuresPlan{}
	states := map[string]string{}
	for id, record := range state.Records {
		found[id], states[id] = record.Plan, record.Status
	}
	for _, run := range state.Runs {
		found[run.Plan.ID], states[run.Plan.ID] = run.Plan, run.Status
	}
	now := time.Now()
	if len(ids) == 0 {
		for id, plan := range found {
			if states[id] == "draft" && plan.Expires.After(now) && (symbol == "" || plan.Snapshot.Symbol == symbol) {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return found[ids[i]].Created.After(found[ids[j]].Created) })
	}
	rows := []map[string]any{}
	var unknown []string
	for _, id := range ids {
		plan, ok := found[id]
		if !ok {
			unknown = append(unknown, id)
			continue
		}
		if len(rows) == comparedPlans {
			break
		}
		row := planSummary(plan)
		row["id"], row["status"] = id, states[id]
		row["kind"] = plan.Idea.Execution.Kind
		row["risk"], row["reward"] = plan.Risk, plan.Reward
		row["expires"], row["expired"] = plan.Expires, !plan.Expires.After(now)
		if plan.SizingLimit != "" {
			row["sizing_limit"] = plan.SizingLimit
		}
		if plan.Idea.Rationale != "" {
			row["rationale"] = truncate(plan.Idea.Rationale, 400)
		}
		rows = append(rows, row)
	}
	out := map[string]any{
		"scope": "compare",
		"plans": rows,
		"note":  "risk, reward and reward_risk come from the plan compiler, after estimated fees and slippage. A draft runs out 5 minutes after it was made. Plans are approved and run on the Futures page, not here.",
	}
	if len(unknown) > 0 {
		out["not_found"] = unknown
	}
	return out
}
