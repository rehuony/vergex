package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"vergex/internal/futures"
)

// ── plan_management ─────────────────────────────────────────────────────────

type planArgs struct {
	Action     string       `json:"action"`
	Kind       string       `json:"kind"`
	AccountRef *targetRef   `json:"account_ref"`
	Symbol     string       `json:"symbol"`
	Targets    []*targetRef `json:"targets"`
}

// What a plan is, for deleting it.
const (
	planFinished = "finished"
	planDraft    = "draft"
	planAtWork   = "at_work"
)

// planEntry is one of the user's plans and where it stands.
type planEntry struct {
	plan    futuresPlan
	kind    string
	account string
}

func (p planEntry) name() string { return orDash(p.plan.Idea.Name) }

func (p planEntry) when() string { return p.plan.Created.Local().Format("2006-01-02 15:04") }

func planKindName(kind string, zh bool) string {
	switch kind {
	case planFinished:
		return status(zh, "已完成", "finished")
	case planDraft:
		return status(zh, "草稿", "draft")
	}
	return status(zh, "进行中", "at work")
}

// plans reads every plan of the user with where it stands: finished once each of its runs
// has settled, at work while one has not, and a draft when it never ran.
func (a *Agent) plans() ([]planEntry, error) {
	raw, err := a.api.raw("GET", "/api/futures/state", nil)
	if err != nil {
		return nil, err
	}
	var state futuresState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	ran := map[string]planEntry{}
	for _, run := range state.Runs {
		entry, seen := ran[run.Plan.ID]
		if !seen {
			entry = planEntry{plan: run.Plan, kind: planFinished, account: run.AccountID}
		}
		if !futures.RunSettled(run.Status) {
			entry.kind = planAtWork
		}
		ran[run.Plan.ID] = entry
	}
	out := make([]planEntry, 0, len(state.Records))
	for id, record := range state.Records {
		if entry, ok := ran[id]; ok {
			out = append(out, entry)
			delete(ran, id)
			continue
		}
		out = append(out, planEntry{plan: record.Plan, kind: planDraft, account: state.Accounts[id]})
	}
	for _, entry := range ran {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].plan.Created.After(out[j].plan.Created) })
	return out, nil
}

func (a *Agent) planManagement(arguments string) map[string]any {
	var args planArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Action != "delete" && args.Action != "clear" {
		return map[string]any{"error": "unsupported_action", "action": args.Action}
	}
	accounts, err := a.accounts()
	if err != nil {
		return errResult(err)
	}
	names := map[string]string{}
	for _, e := range accounts {
		names[e.ID] = e.label()
	}
	account := ""
	if !args.AccountRef.empty() {
		e, rerr := resolve("account", accountNames(accounts), args.AccountRef)
		if rerr != nil {
			return toMap(rerr)
		}
		account = e.ID
	}
	all, err := a.plans()
	if err != nil {
		return errResult(err)
	}
	symbol := strings.ToUpper(strings.TrimSpace(args.Symbol))
	var list []planEntry
	for _, p := range all {
		if (account == "" || p.account == account) && (symbol == "" || p.plan.Snapshot.Symbol == symbol) {
			list = append(list, p)
		}
	}
	if args.Action == "clear" {
		return a.clearPlans(list, args.Kind, names[account])
	}
	return a.deletePlans(list, args.Targets, names)
}

// planNote says what deleting plans leaves, from the kinds they are.
func planNote(plans []planEntry, zh bool) string {
	for _, p := range plans {
		if p.kind == planFinished {
			return status(zh, "它们下过的订单、成交和持仓仍保留在账户记录里", "the orders, fills and positions they sent stay in the account's records")
		}
	}
	return status(zh, "从未下单，没有订单受影响", "they never sent an order")
}

// deletePlans holds the deletion of each plan named, once every name has been matched to
// exactly one that is done with. Nothing is held while any is ambiguous, unknown or at work.
func (a *Agent) deletePlans(list []planEntry, targets []*targetRef, accounts map[string]string) map[string]any {
	if len(targets) == 0 {
		return missing("targets")
	}
	candidates := make([]named, len(list))
	byID := map[string]planEntry{}
	for i, p := range list {
		candidates[i] = named{ID: p.plan.ID, Name: p.name()}
		byID[p.plan.ID] = p
	}
	var chosen []planEntry
	seen := map[string]bool{}
	for _, ref := range targets {
		match, rerr := resolve("plan", candidates, ref)
		if rerr != nil {
			return toMap(rerr)
		}
		if byID[match.ID].kind == planAtWork {
			return map[string]any{"error": "plan_at_work", "plan": match.Name,
				"instruction": "This plan is still at work on the exchange and cannot be deleted. Tell the user it can be deleted once it has finished."}
		}
		if !seen[match.ID] {
			seen[match.ID] = true
			chosen = append(chosen, byID[match.ID])
		}
	}
	zh := a.zh("")
	var result map[string]any
	for _, p := range chosen {
		where := planKindName(p.kind, zh) + status(zh, "，", ", ") + p.plan.Snapshot.Symbol
		if name := accounts[p.account]; name != "" {
			where += status(zh, "，账户“"+name+"”", `, account "`+name+`"`)
		}
		summary := status(zh, fmt.Sprintf("删除计划“%s”（%s，%s）", p.name(), where, p.when()), fmt.Sprintf("delete plan \"%s\" (%s, %s)", p.name(), where, p.when()))
		item := ConfirmItem{Action: actionDeletePlan, Target: p.name(), Account: accounts[p.account], Destructive: true,
			Details: []string{planKindName(p.kind, zh) + " · " + p.plan.Snapshot.Symbol + " · " + p.when(), planNote([]planEntry{p}, zh)}}
		ids := []string{p.plan.ID}
		result = a.holdItem(item, summary, func() (string, error) { return a.removePlans(ids, zh) })
	}
	return result
}

// clearPlans holds the deletion of every plan of a kind that is done with, as one action, by
// the plans listed now. A plan at work is never among them.
func (a *Agent) clearPlans(list []planEntry, kind, account string) map[string]any {
	if kind != planFinished && kind != "drafts" && kind != "all" {
		return missing("kind (finished, drafts or all)")
	}
	zh := a.zh("")
	var plans []planEntry
	counts := map[string]int{}
	atWork := 0
	for _, p := range list {
		switch {
		case p.kind == planAtWork:
			atWork++
		case kind == "all", kind == planFinished && p.kind == planFinished, kind == "drafts" && p.kind == planDraft:
			plans = append(plans, p)
			counts[p.kind]++
		}
	}
	if len(plans) == 0 {
		return map[string]any{"status": "nothing_to_delete", "at_work_kept": atWork}
	}
	var parts []string
	for _, k := range []string{planFinished, planDraft} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", planKindName(k, zh), counts[k]))
		}
	}
	breakdown := strings.Join(parts, status(zh, "、", ", "))
	summary := status(zh, fmt.Sprintf("删除 %d 份计划（%s）", len(plans), breakdown), fmt.Sprintf("delete %d plans (%s)", len(plans), breakdown))
	if account != "" {
		summary += status(zh, "，账户“"+account+"”", ` on account "`+account+`"`)
	}
	details := []string{breakdown, planNote(plans, zh)}
	if atWork > 0 {
		note := status(zh, fmt.Sprintf("%d 份进行中的计划保留", atWork), fmt.Sprintf("%d still at work stay", atWork))
		summary += status(zh, "；"+note, "; "+note)
		details = append(details, note)
	}
	ids := make([]string, len(plans))
	for i, p := range plans {
		ids[i] = p.plan.ID
	}
	item := ConfirmItem{Action: actionDeletePlans, Target: status(zh, fmt.Sprintf("%d 份", len(plans)), fmt.Sprint(len(plans))),
		Account: account, Destructive: true, Details: details}
	return a.holdItem(item, summary, func() (string, error) { return a.removePlans(ids, zh) })
}

// removePlans deletes plans by id, and says which had gone to work since they were listed.
func (a *Agent) removePlans(ids []string, zh bool) (string, error) {
	var out struct {
		Deleted int      `json:"deleted"`
		AtWork  []string `json:"at_work"`
	}
	if err := a.api.do("DELETE", "/api/futures/plans", map[string]any{"ids": ids}, &out); err != nil {
		return "", err
	}
	if len(out.AtWork) > 0 {
		return status(zh, fmt.Sprintf("其中 %d 份已经开始执行，保留", len(out.AtWork)), fmt.Sprintf("%d of them went to work since and stay", len(out.AtWork))), nil
	}
	return "", nil
}
