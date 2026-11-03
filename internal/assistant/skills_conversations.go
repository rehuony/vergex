package assistant

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"vergex/internal/agent"
)

// ── conversation_management ─────────────────────────────────────────────────

type conversationArgs struct {
	Action     string       `json:"action"`
	Feature    string       `json:"feature"`
	AccountRef *targetRef   `json:"account_ref"`
	Targets    []*targetRef `json:"targets"`
	Search     string       `json:"search"`
}

// chatInfo is one of the user's chats, as GET /api/conversations lists it.
type chatInfo struct {
	ID        string    `json:"id"`
	Feature   string    `json:"feature"`
	Title     string    `json:"title"`
	AccountID string    `json:"account_id"`
	Symbol    string    `json:"symbol"`
	UpdatedAt time.Time `json:"updated_at"`
}

// conversationsShown bounds how many conversations one answer lists.
const conversationsShown = 30

// featureWords name the features people chat in. One added later goes by its own name until
// it has words here.
var featureWords = map[string]stageWords{
	agent.FeatureFutures:   {"Futures", "合约交易"},
	agent.FeatureAssistant: {"Assistant", "助手"},
	agent.FeatureTelegram:  {"Telegram", "Telegram"},
}

func featureName(feature string, zh bool) string {
	if words, ok := featureWords[feature]; ok {
		return status(zh, words.zh, words.en)
	}
	return feature
}

func (c chatInfo) when() string { return c.UpdatedAt.Local().Format("2006-01-02 15:04") }

// name is what a conversation goes by: its title, or for a chat without one, where and when.
func (c chatInfo) name() string {
	if c.Title != "" {
		return c.Title
	}
	return featureName(c.Feature, false) + " " + c.when()
}

func (a *Agent) conversationManagement(arguments string) map[string]any {
	var args conversationArgs
	if bad := decodeArgs(arguments, &args); bad != nil {
		return bad
	}
	if args.Action != "query" && args.Action != "delete" && args.Action != "clear" {
		return map[string]any{"error": "unsupported_action", "action": args.Action}
	}
	if args.Feature != "" && !agent.IsChat(args.Feature) {
		return map[string]any{"error": "unknown_feature", "features": agent.ChatFeatures}
	}
	accounts, err := a.accounts()
	if err != nil {
		return errResult(err)
	}
	q := url.Values{}
	if args.Feature != "" {
		q.Set("feature", args.Feature)
	}
	account := ""
	if !args.AccountRef.empty() {
		e, rerr := resolve("account", accountNames(accounts), args.AccountRef)
		if rerr != nil {
			return toMap(rerr)
		}
		q.Set("account_id", e.ID)
		account = e.Name
	}
	var list []chatInfo
	if err := a.api.do("GET", "/api/conversations?"+q.Encode(), nil, &list); err != nil {
		return errResult(err)
	}
	if search := strings.ToLower(strings.TrimSpace(args.Search)); search != "" {
		found := []chatInfo{}
		for _, c := range list {
			if strings.Contains(strings.ToLower(c.name()), search) {
				found = append(found, c)
			}
		}
		list = found
	}
	names := map[string]string{}
	for _, e := range accounts {
		names[e.ID] = e.label()
	}
	switch args.Action {
	case "delete":
		return a.deleteChats(list, args.Targets, names)
	case "clear":
		return a.clearChats(list, account)
	}
	rows := []map[string]any{}
	for _, c := range list {
		if len(rows) == conversationsShown {
			break
		}
		row := map[string]any{"id": c.ID, "feature": c.Feature, "name": c.name(), "updated_at": c.when()}
		if c.AccountID != "" {
			row["account"] = orDash(names[c.AccountID])
		}
		if c.Symbol != "" {
			row["symbol"] = c.Symbol
		}
		if c.ID == a.conversation() {
			row["current"] = true
		}
		rows = append(rows, row)
	}
	return map[string]any{"total": len(list), "shown": rows}
}

// notHere refuses to delete the conversation the request is made in: the turn is still
// writing to it.
var notHere = map[string]any{
	"error":       "current_conversation",
	"instruction": "The conversation you are in cannot be deleted from inside it. Tell the user it is deleted from the conversation list, or that a new conversation can be started first.",
}

// chainNote says what goes with the conversations, from the features they are in.
func chainNote(chats []chatInfo, zh bool) string {
	for _, c := range chats {
		if c.Feature == agent.FeatureFutures {
			return status(zh, "消息、模型调用记录和未执行的计划一并删除", "with their messages, model calls and plans that never ran")
		}
	}
	return status(zh, "消息和模型调用记录一并删除", "with their messages and model calls")
}

// deleteChats holds the deletion of each conversation named, once every name has been
// matched to exactly one. Nothing is held while any is ambiguous or unknown.
func (a *Agent) deleteChats(list []chatInfo, targets []*targetRef, accounts map[string]string) map[string]any {
	if len(targets) == 0 {
		return missing("targets")
	}
	candidates := make([]named, len(list))
	byID := map[string]chatInfo{}
	for i, c := range list {
		candidates[i] = named{ID: c.ID, Name: c.name()}
		byID[c.ID] = c
	}
	var chosen []chatInfo
	seen := map[string]bool{}
	for _, ref := range targets {
		match, rerr := resolve("conversation", candidates, ref)
		if rerr != nil {
			return toMap(rerr)
		}
		if match.ID == a.conversation() {
			return notHere
		}
		if !seen[match.ID] {
			seen[match.ID] = true
			chosen = append(chosen, byID[match.ID])
		}
	}
	zh := a.zh("")
	var result map[string]any
	for _, c := range chosen {
		where := featureName(c.Feature, zh)
		if name := accounts[c.AccountID]; name != "" {
			where += status(zh, "，账户“"+name+"”", `, account "`+name+`"`)
		}
		summary := status(zh, fmt.Sprintf("删除对话“%s”（%s，%s）", c.name(), where, c.when()), fmt.Sprintf("delete conversation \"%s\" (%s, %s)", c.name(), where, c.when()))
		item := ConfirmItem{Action: actionDeleteChat, Target: c.name(), Account: accounts[c.AccountID], Destructive: true,
			Details: []string{featureName(c.Feature, zh) + " · " + c.when(), chainNote([]chatInfo{c}, zh)}}
		ids := []string{c.ID}
		result = a.holdItem(item, summary, func() (string, error) { return a.removeChats(ids, zh) })
	}
	return result
}

// clearChats holds the deletion of every conversation listed but the one the request is made
// in, as one action. What runs is the list as it is now: a conversation begun after it
// stays.
func (a *Agent) clearChats(list []chatInfo, account string) map[string]any {
	zh := a.zh("")
	var chats []chatInfo
	keptHere := false
	counts := map[string]int{}
	var order []string
	for _, c := range list {
		if c.ID == a.conversation() {
			keptHere = true
			continue
		}
		if counts[c.Feature] == 0 {
			order = append(order, c.Feature)
		}
		counts[c.Feature]++
		chats = append(chats, c)
	}
	if len(chats) == 0 {
		return map[string]any{"status": "nothing_to_delete", "kept_current": keptHere}
	}
	parts := make([]string, len(order))
	for i, feature := range order {
		parts[i] = fmt.Sprintf("%s %d", featureName(feature, zh), counts[feature])
	}
	breakdown := strings.Join(parts, status(zh, "、", ", "))
	summary := status(zh, fmt.Sprintf("删除 %d 条对话（%s）", len(chats), breakdown), fmt.Sprintf("delete %d conversations (%s)", len(chats), breakdown))
	if account != "" {
		summary += status(zh, "，账户“"+account+"”", ` on account "`+account+`"`)
	}
	details := []string{breakdown, chainNote(chats, zh)}
	if keptHere {
		note := status(zh, "当前这条对话保留", "this conversation stays")
		summary += status(zh, "；"+note, "; "+note)
		details = append(details, note)
	}
	target := status(zh, fmt.Sprintf("%d 条", len(chats)), fmt.Sprint(len(chats)))
	ids := make([]string, len(chats))
	for i, c := range chats {
		ids[i] = c.ID
	}
	item := ConfirmItem{Action: actionDeleteChats, Target: target, Account: account, Destructive: true, Details: details}
	return a.holdItem(item, summary, func() (string, error) { return a.removeChats(ids, zh) })
}

// removeChats deletes conversations by id, and says when some had already gone.
func (a *Agent) removeChats(ids []string, zh bool) (string, error) {
	var out struct {
		Deleted int `json:"deleted"`
	}
	if err := a.api.do("DELETE", "/api/conversations", map[string]any{"ids": ids}, &out); err != nil {
		return "", err
	}
	if gone := len(ids) - out.Deleted; gone > 0 {
		return status(zh, fmt.Sprintf("其中 %d 条已经不在了", gone), fmt.Sprintf("%d of them were already gone", gone)), nil
	}
	return "", nil
}
