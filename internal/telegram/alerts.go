package telegram

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/syncer"
	"vergex/internal/logger"
	"vergex/internal/service"
	"vergex/internal/trader"
)

// Alert kinds the user can turn on or off in Settings.
const (
	AlertOrderRejected  = "order_rejected"
	AlertSafeMode       = "safe_mode"
	AlertPositionClosed = "position_closed"
	AlertPlanAttention  = "plan_attention"
	AlertForecast       = "forecast"
	AlertSystem         = "system"
	AlertDailyReport    = "daily_report"
)

// AlertKinds lists every kind, in the order Settings shows them.
var AlertKinds = []string{AlertOrderRejected, AlertSafeMode, AlertPositionClosed, AlertPlanAttention, AlertForecast, AlertSystem, AlertDailyReport}

const (
	alertInterval = 30 * time.Second
	// alertLookback bounds how far back events are reported after downtime.
	alertLookback = 24 * time.Hour
	alertBatch    = 20
	// accountFailingFor is how long an account's reads must keep failing to be worth a
	// message: a network hiccup clears by itself.
	accountFailingFor = 5 * time.Minute
	// alertLate is how old an event must be for its message to say when it happened.
	alertLate = 2 * time.Minute
	// maxAlertDetail bounds what an error adds to a line: a provider's message can run long.
	maxAlertDetail = 300
)

const (
	markOrders        = "stream:orders"
	markActions       = "stream:actions"
	markPositions     = "stream:positions"
	markFuturesOpened = "stream:futures_opened"
	markFuturesClosed = "stream:futures_closed"
	markSafeMode      = "safe_mode:"
	markBlocked       = "blocked:"
	markPlan          = "plan:"
	markProblem       = "problem:"
	markAccount       = "account:"
	markReport        = "report:daily"
)

// PlanRun is a Futures run as the alert watcher needs it.
type PlanRun struct {
	ID     string
	Symbol string
	Status string
	Error  string
}

// AlertSources reads state that lives outside the database.
type AlertSources struct {
	// TraderStatus returns a loaded trader's status, or nil.
	TraderStatus func(traderID string) map[string]any
	// PlanRuns returns the user's Futures runs.
	PlanRuns func(ctx context.Context, user string) ([]PlanRun, error)
	// Problems returns what is wrong that has no alert kind of its own.
	Problems func(user string) []Problem
	// FailingAccounts returns the accounts whose reads are failing.
	FailingAccounts func() map[string]syncer.Failure
	// AccountEquity returns what an account holds, if it can be read. It is the one source
	// here that may ask the exchange, once a day for the daily report.
	AccountEquity func(user, accountID string) (float64, bool)
}

// The kinds of problem the watcher has wording for.
const (
	ProblemService     = "service_failed"
	ProblemTraderHalt  = "trader_halted"
	ProblemTraderLoad  = "trader_load_failed"
	ProblemFillSkipped = "account_fill_skipped"
)

// Problem is something wrong that the user should know of. Key stays the same for as long
// as it lasts, so it is told once.
type Problem struct {
	Key    string
	Kind   string
	Name   string
	Detail string
}

type alertWatcher struct {
	stores  *Stores
	sources AlertSources
	send    func(ctx context.Context, part messagePart, silent bool) error
	now     func() time.Time
	// delivered holds the parts already sent of a message that failed part way, so trying
	// again sends only the rest.
	delivered map[string]bool
}

// StartAlerts watches for events worth telling the user about until ctx ends.
// It only reads: it never touches the trading loop and makes no exchange calls.
// Must be called as a goroutine.
func StartAlerts(ctx context.Context, st *Stores, sources AlertSources) {
	w := &alertWatcher{
		stores:  st,
		sources: sources,
		send: func(ctx context.Context, part messagePart, silent bool) error {
			return notifyPart(ctx, st.Telegram, part, silent)
		},
		now: time.Now,
	}
	ticker := time.NewTicker(alertInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

// pending is one check's events, by the section of the message they go in, and the marks
// to save once they are delivered.
type pending struct {
	cfg  *Config
	lang string
	// attention, orders, entries and closes are the digest's sections, in that order.
	attention []text
	orders    []orderEvent
	entries   []positionEvent
	closes    []positionEvent
	// report is the daily report, which goes out on its own.
	report  *message
	set     map[string]int64
	cleared []string
}

// orderEvent is an order that was refused or failed.
type orderEvent struct {
	symbol, what, reason string
	origin               origin
	late                 string
}

// positionEvent is a position that opened or closed.
type positionEvent struct {
	symbol, side    string
	quantity, price float64
	pnl             float64
	origin          origin
	late            string
}

// origin is who an event belongs to: a trader, or Futures, and on what kind of money.
type origin struct {
	name    string
	testnet bool
}

// add keeps an event unless the user muted its kind. Either way the mark moves, so turning
// a kind back on does not replay what happened while it was off.
func (p *pending) add(kind string, keep func()) {
	if !p.cfg.AlertMuted(kind) {
		keep()
	}
}

func (w *alertWatcher) check(ctx context.Context) {
	cfg, err := w.stores.Telegram.Get()
	if err != nil || cfg.BotToken == "" || cfg.ChatID == 0 {
		service.Report(service.TelegramAlerts, service.Off, "")
		return
	}
	users, err := w.stores.Users.GetAll()
	if err != nil || len(users) == 0 {
		return
	}
	user := users[0].ID
	traders, err := w.stores.Traders.List(user)
	if err != nil {
		logger.Warnf("Telegram alerts: read traders: %v", err)
		return
	}
	p := &pending{cfg: cfg, lang: w.stores.Telegram.GetLanguage(), set: map[string]int64{}}
	testnet := w.testnetAccounts(user)
	ids := make([]string, 0, len(traders))
	owners := make(map[string]origin, len(traders))
	for _, trader := range traders {
		ids = append(ids, trader.ID)
		owners[trader.ID] = origin{name: trader.Name, testnet: testnet[trader.AccountID]}
	}

	now := w.now().UnixMilli()
	w.checkOrders(ids, owners, now, p)
	w.checkActions(ids, owners, now, p)
	w.checkPositions(ids, owners, now, p)
	w.checkTraderState(traders, owners, now, p)
	w.checkPlans(ctx, user, now, p)
	w.checkFuturesPositions(user, now, p)
	w.checkProblems(user, now, p)
	w.checkDailyReport(user, p)

	for _, m := range []*message{p.digest(), p.report} {
		if m == nil || m.empty() {
			continue
		}
		if err := w.deliver(ctx, m); err != nil {
			// Marks stay where they are, so the same events go out next time.
			logger.Warnf("Telegram alerts: send: %v", err)
			service.Report(service.TelegramAlerts, service.Degraded, err.Error())
			return
		}
	}
	service.Report(service.TelegramAlerts, service.Running, "")
	marks := w.stores.Telegram
	for key, at := range p.set {
		if err := marks.SetAlertMark(key, at); err != nil {
			logger.Warnf("Telegram alerts: save mark: %v", err)
		}
	}
	for _, key := range p.cleared {
		if err := marks.DeleteAlertMark(key); err != nil {
			logger.Warnf("Telegram alerts: clear mark: %v", err)
		}
	}
}

// deliver sends a message in parts, skipping those an earlier try already got through.
func (w *alertWatcher) deliver(ctx context.Context, m *message) error {
	for _, part := range m.parts() {
		if w.delivered[part.HTML] {
			continue
		}
		if err := w.send(ctx, part, m.silent); err != nil {
			return err
		}
		if w.delivered == nil {
			w.delivered = map[string]bool{}
		}
		w.delivered[part.HTML] = true
	}
	w.delivered = nil
	return nil
}

// since returns where a stream continues from. A stream seen for the first time
// starts now, so connecting the bot does not replay history.
func (w *alertWatcher) since(key string, now int64, p *pending) (int64, bool) {
	mark, ok := w.stores.Telegram.AlertMark(key)
	if !ok {
		p.set[key] = now
		return 0, false
	}
	if oldest := now - alertLookback.Milliseconds(); mark < oldest {
		return oldest, true
	}
	return mark, true
}

func (w *alertWatcher) checkOrders(ids []string, owners map[string]origin, now int64, p *pending) {
	since, ok := w.since(markOrders, now, p)
	if !ok {
		return
	}
	orders, err := w.stores.Orders.RejectedSince(ids, since, alertBatch)
	if err != nil {
		logger.Warnf("Telegram alerts: %v", err)
		return
	}
	for _, order := range orders {
		event := orderEvent{symbol: order.Symbol, what: orderSide(p.lang, order.Side), reason: order.Error, origin: owners[order.TraderID], late: w.late(order.UpdatedAt, now)}
		p.add(AlertOrderRejected, func() { p.orders = append(p.orders, event) })
		p.set[markOrders] = order.UpdatedAt
	}
}

func (w *alertWatcher) checkActions(ids []string, owners map[string]origin, now int64, p *pending) {
	since, ok := w.since(markActions, now, p)
	if !ok {
		return
	}
	actions, err := w.stores.Decisions.FailedActionsSince(ids, since, alertBatch)
	if err != nil {
		logger.Warnf("Telegram alerts: %v", err)
		return
	}
	for _, action := range actions {
		at := action.Timestamp.UnixMilli()
		event := orderEvent{symbol: action.Symbol, what: decisionAction(p.lang, action.Action), reason: action.Error, origin: owners[action.TraderID], late: w.late(at, now)}
		p.add(AlertOrderRejected, func() { p.orders = append(p.orders, event) })
		p.set[markActions] = at
	}
}

func (w *alertWatcher) checkPositions(ids []string, owners map[string]origin, now int64, p *pending) {
	since, ok := w.since(markPositions, now, p)
	if !ok {
		return
	}
	positions, err := w.stores.Positions.ClosedSince(ids, since, alertBatch)
	if err != nil {
		logger.Warnf("Telegram alerts: %v", err)
		return
	}
	for _, position := range positions {
		event := closeEvent(position, owners[position.TraderID], w.late(position.ExitTime, now))
		p.add(AlertPositionClosed, func() { p.closes = append(p.closes, event) })
		p.set[markPositions] = position.ExitTime
	}
}

func closeEvent(position *account.TraderPosition, owner origin, late string) positionEvent {
	return positionEvent{symbol: position.Symbol, side: strings.ToLower(position.Side), pnl: position.RealizedPnL, origin: owner, late: late}
}

// checkTraderState reports safe mode and blocked trading once each time they
// begin; a mark that clears re-arms the alert.
func (w *alertWatcher) checkTraderState(traders []*trader.Trader, owners map[string]origin, now int64, p *pending) {
	if w.sources.TraderStatus == nil {
		return
	}
	for _, trader := range traders {
		status := w.sources.TraderStatus(trader.ID)
		name := owners[trader.ID].label(p.lang)
		safeMode, _ := status["safe_mode"].(bool)
		reason, _ := status["safe_mode_reason"].(string)
		w.standing(markSafeMode+trader.ID, safeMode, now, p, attentionLine(p.lang, pick(p.lang, "Safe mode", "安全模式"), name, reason))
		blocked, _ := status["trading_blocked"].(bool)
		reason, _ = status["trading_error"].(string)
		w.standing(markBlocked+trader.ID, blocked, now, p, attentionLine(p.lang, pick(p.lang, "Trading blocked", "交易受阻"), name, reason))
	}
}

// testnetAccounts says which of the user's accounts hold test money.
func (w *alertWatcher) testnetAccounts(user string) map[string]bool {
	accounts, err := w.stores.Accounts.ListNames(user)
	if err != nil {
		logger.Warnf("Telegram alerts: read accounts: %v", err)
		return nil
	}
	testnet := make(map[string]bool, len(accounts))
	for _, row := range accounts {
		testnet[row.ID] = row.Testnet
	}
	return testnet
}

// late says when an event happened if its message is going out late, as after downtime or a
// failed send, when it would otherwise read as just now.
func (w *alertWatcher) late(at, now int64) string {
	if at <= 0 || now-at < alertLate.Milliseconds() {
		return ""
	}
	return moment(time.UnixMilli(at))
}

func (w *alertWatcher) checkPlans(ctx context.Context, user string, now int64, p *pending) {
	if w.sources.PlanRuns == nil {
		return
	}
	runs, err := w.sources.PlanRuns(ctx, user)
	if err != nil {
		logger.Warnf("Telegram alerts: read Futures runs: %v", err)
		return
	}
	lines := make(map[string]text)
	for _, run := range runs {
		if run.Status == "needs_attention" {
			lines[markPlan+run.ID] = attentionLine(p.lang, pick(p.lang, "Futures plan needs attention", "合约交易计划需要处理"), run.Symbol, run.Error)
		}
	}
	w.ongoing(markPlan, AlertPlanAttention, lines, now, p)
}

// checkProblems reports what is wrong outside any trade: services that failed, traders
// halted or not loaded, trades the ledger could not book and accounts that stopped reading.
func (w *alertWatcher) checkProblems(user string, now int64, p *pending) {
	if w.sources.Problems != nil {
		lines := make(map[string]text)
		for _, problem := range w.sources.Problems(user) {
			lines[markProblem+problem.Key] = problemLine(p.lang, problem)
		}
		w.ongoing(markProblem, AlertSystem, lines, now, p)
	}
	if w.sources.FailingAccounts == nil {
		return
	}
	accounts, err := w.stores.Accounts.ListNames(user)
	if err != nil {
		logger.Warnf("Telegram alerts: read accounts: %v", err)
		return
	}
	failing := w.sources.FailingAccounts()
	lines := make(map[string]text)
	for _, row := range accounts {
		failure, ok := failing[row.ID]
		if ok && w.now().Sub(failure.Since) >= accountFailingFor {
			lines[markAccount+row.ID] = attentionLine(p.lang, pick(p.lang, "Account unreadable", "账户无法读取"), row.Name, failure.Reason)
		}
	}
	w.ongoing(markAccount, AlertSystem, lines, now, p)
}

// ongoing tells of each line once, when its mark first appears, and re-arms it once it is
// gone. Marks under the prefix that are not in lines are gone.
func (w *alertWatcher) ongoing(prefix, kind string, lines map[string]text, now int64, p *pending) {
	keys := make([]string, 0, len(lines))
	for key := range lines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, seen := w.stores.Telegram.AlertMark(key); !seen {
			p.set[key] = now
			line := lines[key]
			p.add(kind, func() { p.attention = append(p.attention, line) })
		}
	}
	marks, err := w.stores.Telegram.AlertMarkKeys(prefix)
	if err != nil {
		logger.Warnf("Telegram alerts: %v", err)
		return
	}
	for _, key := range marks {
		if _, active := lines[key]; !active {
			p.cleared = append(p.cleared, key)
		}
	}
}

// checkFuturesPositions reports what the exchange did with a plan: its entry filling and
// its position closing, by a stop or a target as much as by hand. Both come from the
// ledger, which the account's sync keeps.
func (w *alertWatcher) checkFuturesPositions(user string, now int64, p *pending) {
	testnet := w.testnetAccounts(user)
	futures := func(accountID string) origin {
		return origin{name: pick(p.lang, "Futures", "合约交易"), testnet: testnet[accountID]}
	}
	if since, ok := w.since(markFuturesOpened, now, p); ok {
		positions, err := w.stores.Positions.FuturesOpenedSince(since, alertBatch)
		if err != nil {
			logger.Warnf("Telegram alerts: %v", err)
		}
		for _, position := range positions {
			event := positionEvent{symbol: position.Symbol, side: strings.ToLower(position.Side), quantity: position.EntryQuantity,
				price: position.EntryPrice, origin: futures(position.AccountID), late: w.late(position.EntryTime, now)}
			p.add(AlertPlanAttention, func() { p.entries = append(p.entries, event) })
			p.set[markFuturesOpened] = position.EntryTime
		}
	}
	if since, ok := w.since(markFuturesClosed, now, p); ok {
		positions, err := w.stores.Positions.FuturesClosedSince(since, alertBatch)
		if err != nil {
			logger.Warnf("Telegram alerts: %v", err)
		}
		for _, position := range positions {
			event := closeEvent(position, futures(position.AccountID), w.late(position.ExitTime, now))
			p.add(AlertPositionClosed, func() { p.closes = append(p.closes, event) })
			p.set[markFuturesClosed] = position.ExitTime
		}
	}
}

// checkDailyReport sends the last UTC day's summary once, the first check of each new day.
// Its mark moves only once the message is delivered, so a restart repeats nothing and a
// failed send goes out again. After days away it reports the last day only, and a muted
// report leaves nothing to catch up on.
func (w *alertWatcher) checkDailyReport(user string, p *pending) {
	today := w.now().UTC().Truncate(24 * time.Hour)
	mark, seen := w.stores.Telegram.AlertMark(markReport)
	if !seen {
		p.set[markReport] = today.UnixMilli()
		return
	}
	if mark >= today.UnixMilli() {
		return
	}
	p.set[markReport] = today.UnixMilli()
	if p.cfg.AlertMuted(AlertDailyReport) {
		return
	}
	if report, ok := w.dailyReport(user, today.AddDate(0, 0, -1), today, p.lang); ok {
		p.report = report
	}
}

// dailyReport sums a day on the real accounts. Test money counts for nothing, and with no
// real account there is nothing to report. A figure that cannot be read is a dash.
func (w *alertWatcher) dailyReport(user string, from, to time.Time, lang string) (*message, bool) {
	accounts, err := w.stores.Accounts.ListNames(user)
	if err != nil {
		logger.Warnf("Telegram alerts: read accounts: %v", err)
		return nil, false
	}
	ids := []string{}
	for _, row := range accounts {
		if !row.Testnet {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil, false
	}
	unread := strong("—")

	equity := unread
	if w.sources.AccountEquity != nil {
		total, known := 0.0, true
		for _, id := range ids {
			value, ok := w.sources.AccountEquity(user, id)
			known = known && ok
			total += value
		}
		if known {
			equity = chip(amount(total))
		}
	}
	realized, headline := unread, ""
	if value, err := w.stores.Positions.RealizedBetween(ids, from, to); err != nil {
		logger.Warnf("Telegram alerts: %v", err)
	} else {
		realized, headline = chip(change(value)), change(value)+" USDT"
	}
	holding := unread
	if positions, err := w.stores.Positions.OpenOnAccounts(ids); err != nil {
		logger.Warnf("Telegram alerts: %v", err)
	} else {
		names := make([]string, 0, len(positions))
		for _, position := range positions {
			names = append(names, position.Symbol+" "+sideOf(strings.ToLower(position.Side), lang))
		}
		holding = strong(strconv.Itoa(len(positions)))
		if len(names) > 0 {
			holding = joined(holding, words(pick(lang, " ("+strings.Join(names, ", ")+")", "（"+strings.Join(names, "、")+"）")))
		}
	}
	cost := unread
	if summary, err := w.stores.Calls.GetSummary("", from.Format("2006-01-02")); err != nil {
		logger.Warnf("Telegram alerts: %v", err)
	} else {
		cost = chip(dollars(summary.TotalUSD))
	}
	m := newMessage(lang)
	m.silent = true
	m.title(emojiReport, pick(lang, "Daily report · ", "每日汇总 · ")+from.Format("01-02"), headline).
		meta(pick(lang, "Real money only · UTC", "只计实盘 · UTC"))
	m.add(
		m.fact(pick(lang, "Equity", "账户权益"), equity),
		m.fact(pick(lang, "Realized", "实现盈亏"), realized),
		m.fact(pick(lang, "Open positions", "当前持仓"), holding),
		m.fact(pick(lang, "Model cost, all calls", "模型花费"), cost),
	)
	return m, true
}

func (w *alertWatcher) standing(key string, active bool, now int64, p *pending, line text) {
	_, seen := w.stores.Telegram.AlertMark(key)
	switch {
	case active && !seen:
		p.set[key] = now
		p.add(AlertSafeMode, func() { p.attention = append(p.attention, line) })
	case !active && seen:
		p.cleared = append(p.cleared, key)
	}
}

func pick(lang, en, zh string) string {
	if lang == "zh" {
		return zh
	}
	return en
}

// detailOf cuts what an error adds to a line: a provider's message can run long.
func detailOf(detail string) string {
	detail = strings.TrimSpace(detail)
	if runes := []rune(detail); len(runes) > maxAlertDetail {
		detail = string(runes[:maxAlertDetail]) + "…"
	}
	return detail
}

// label is who an event belongs to, so test money never reads as real.
func (o origin) label(lang string) string {
	if o.testnet {
		return o.name + " · " + pick(lang, "Testnet", "测试网")
	}
	return o.name
}

// money is who an event belongs to, with the kind of money either way.
func (o origin) money(lang string) string {
	if o.testnet {
		return o.name + " · " + pick(lang, "Testnet", "测试网")
	}
	return o.name + " · " + pick(lang, "Real money", "实盘")
}

// attentionLine is a problem that lasts: what it is, where, and why when that is known.
func attentionLine(lang, what, where, reason string) text {
	line := words("• " + what + " · " + where)
	if reason = detailOf(reason); reason != "" {
		line = joined(line, words(pick(lang, ": ", "：")), strong(reason))
	}
	return line
}

func orderSide(lang, side string) string {
	switch strings.ToUpper(side) {
	case "BUY":
		return pick(lang, "buy", "买入")
	case "SELL":
		return pick(lang, "sell", "卖出")
	}
	return strings.ToLower(side)
}

func decisionAction(lang, action string) string {
	names := map[string][2]string{"open_long": {"open long", "开多"}, "open_short": {"open short", "开空"}, "close_long": {"close long", "平多"}, "close_short": {"close short", "平空"}}
	if name, ok := names[action]; ok {
		return pick(lang, name[0], name[1])
	}
	return action
}

func problemLine(lang string, problem Problem) text {
	var what string
	switch problem.Kind {
	case ProblemService:
		what = pick(lang, "Service failed", "服务故障")
	case ProblemTraderHalt:
		what = pick(lang, "Trader stopped opening positions", "交易员已停止开仓")
	case ProblemTraderLoad:
		what = pick(lang, "Trader could not start", "交易员无法启动")
	case ProblemFillSkipped:
		what = pick(lang, "Trade not booked", "成交未入账")
	default:
		what = pick(lang, "Problem", "问题")
	}
	detail := problem.Detail
	if lang == "zh" && problem.Kind == ProblemTraderHalt {
		detail = haltReasonZh.Replace(detail)
	}
	return attentionLine(lang, what, problem.Name, detail)
}

// haltReasonZh puts the reasons a trader halts for in Chinese. They are written in
// English where they are stored.
var haltReasonZh = strings.NewReplacer("max drawdown exceeded", "超过最大回撤", "daily loss limit exceeded", "超过当日亏损上限")

// digest lays out one check's events: what needs attention, orders that failed, entries
// filled and positions closed, each a section of its own.
func (p *pending) digest() *message {
	m := newMessage(p.lang)
	if n := len(p.attention); n > 0 {
		m.add(heading(emojiAttention, counted(p.lang, "Needs attention", "需要注意", n)))
		m.under(p.attention...)
	}
	switch n := len(p.orders); {
	case n == 1:
		order := p.orders[0]
		m.add(heading(emojiFailed, pick(p.lang, "Order failed · ", "下单失败 · ")+order.symbol+" "+order.what))
		m.meta(lateOf(order.origin.money(p.lang), order.late))
		m.details(order.reason)
	case n > 1:
		m.add(heading(emojiFailed, counted(p.lang, "Orders failed", "下单失败", n)))
		for _, order := range p.orders {
			m.under(item(words(lateOf(order.symbol+" "+order.what+" · "+order.origin.label(p.lang), order.late))))
			m.details(order.reason)
		}
	}
	switch n := len(p.entries); {
	case n == 1:
		entry := p.entries[0]
		m.add(heading(emojiEntry, pick(p.lang, "Entry filled · ", "入场成交 · ")+entry.symbol+" "+sideOf(entry.side, p.lang)))
		m.meta(lateOf(entry.origin.money(p.lang), entry.late))
		m.under(m.fact(pick(p.lang, "Filled", "成交"), strong(exact(entry.quantity)+" @ "+exact(entry.price))))
	case n > 1:
		m.add(heading(emojiEntry, counted(p.lang, "Entries filled", "入场成交", n)))
		m.meta(since(p.lang, p.entries))
		for _, group := range byMoney(p.entries) {
			t := table{caption: moneyCaption(p.lang, pick(p.lang, "Entries", "入场"), group[0].origin.testnet), columns: []column{
				{name: pick(p.lang, "Symbol", "品种")}, {name: pick(p.lang, "Side", "方向")},
				{name: pick(p.lang, "Size", "数量"), right: true}, {name: pick(p.lang, "Price", "价格"), right: true}, {name: pick(p.lang, "From", "来源")},
			}}
			for _, entry := range group {
				t.rows = append(t.rows, []string{entry.symbol, sideOf(entry.side, p.lang), exact(entry.quantity), exact(entry.price), entry.origin.name})
			}
			m.under(t.lines(p.lang)...)
		}
	}
	switch n := len(p.closes); {
	case n == 1:
		closed := p.closes[0]
		m.add(heading(emojiClosed, pick(p.lang, "Position closed · ", "平仓 · ")+closed.symbol+" "+sideOf(closed.side, p.lang)))
		m.meta(lateOf(closed.origin.money(p.lang), closed.late))
		m.under(m.fact(pick(p.lang, "P/L", "盈亏"), chip(change(closed.pnl))))
	case n > 1:
		m.add(heading(emojiClosed, pick(p.lang, fmt.Sprintf("%d positions closed", n), fmt.Sprintf("平仓 %d 笔", n))))
		m.meta(since(p.lang, p.closes))
		for _, group := range byMoney(p.closes) {
			t := table{caption: moneyCaption(p.lang, pick(p.lang, "Closed", "平仓"), group[0].origin.testnet), columns: []column{
				{name: pick(p.lang, "Symbol", "品种")}, {name: pick(p.lang, "Side", "方向")},
				{name: pick(p.lang, "P/L", "盈亏"), right: true}, {name: pick(p.lang, "From", "来源")},
			}}
			sum := 0.0
			for _, closed := range group {
				t.rows = append(t.rows, []string{closed.symbol, sideOf(closed.side, p.lang), change(closed.pnl), closed.origin.name})
				sum += closed.pnl
			}
			if len(group) > 1 {
				t.total = []string{pick(p.lang, "Total", "合计"), "", change(sum), ""}
			}
			m.under(t.lines(p.lang)...)
		}
	}
	return m
}

// byMoney splits events into those on real money and those on test money, so a table and
// its total are only ever one kind.
func byMoney(events []positionEvent) [][]positionEvent {
	var real, test []positionEvent
	for _, event := range events {
		if event.origin.testnet {
			test = append(test, event)
		} else {
			real = append(real, event)
		}
	}
	var groups [][]positionEvent
	for _, group := range [][]positionEvent{real, test} {
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

// moneyCaption names a table of test money as such.
func moneyCaption(lang, name string, testnet bool) string {
	if testnet {
		return name + " · " + pick(lang, "Testnet", "测试网")
	}
	return name
}

// since says from when a table's events are, when some of them go out late.
func since(lang string, events []positionEvent) string {
	earliest := ""
	for _, event := range events {
		if event.late != "" && (earliest == "" || event.late < earliest) {
			earliest = event.late
		}
	}
	if earliest == "" {
		return ""
	}
	return pick(lang, "Since "+earliest, earliest+" 起")
}

// counted is a title with how many it holds, once there are several.
func counted(lang, en, zh string, n int) string {
	if n == 1 {
		return pick(lang, en, zh)
	}
	return pick(lang, en, zh) + " · " + strconv.Itoa(n)
}

// lateOf adds when an event happened to what names it, when it goes out late.
func lateOf(s, late string) string {
	if late == "" {
		return s
	}
	return s + " · " + late
}

// TestAlert is the message Settings sends to check that alerts arrive.
func TestAlert(ctx context.Context, configs ConfigStore) error {
	lang := configs.GetLanguage()
	m := newMessage(lang).title(emojiDone, pick(lang, "Vergex alerts are working", "Vergex 告警工作正常"))
	return Notify(ctx, configs, m)
}
