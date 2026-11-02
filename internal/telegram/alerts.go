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
	State  string
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
	send    func(ctx context.Context, text string) error
	now     func() time.Time
}

// StartAlerts watches for events worth telling the user about until ctx ends.
// It only reads: it never touches the trading loop and makes no exchange calls.
// Must be called as a goroutine.
func StartAlerts(ctx context.Context, st *Stores, sources AlertSources) {
	w := &alertWatcher{
		stores:  st,
		sources: sources,
		send:    func(ctx context.Context, text string) error { return Notify(ctx, st.Telegram, text) },
		now:     time.Now,
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

// pending is one check's messages and the marks to save once they are delivered.
type pending struct {
	cfg     *Config
	lang    string
	lines   []string
	set     map[string]int64
	cleared []string
}

// add queues a line unless the user muted its kind. Either way the mark moves,
// so turning a kind back on does not replay what happened while it was off.
func (p *pending) add(kind, line string) {
	if !p.cfg.AlertMuted(kind) {
		p.lines = append(p.lines, line)
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
	ids := make([]string, 0, len(traders))
	names := make(map[string]string, len(traders))
	for _, trader := range traders {
		ids = append(ids, trader.ID)
		names[trader.ID] = trader.Name
	}

	p := &pending{cfg: cfg, lang: w.stores.Telegram.GetLanguage(), set: map[string]int64{}}
	now := w.now().UnixMilli()
	w.checkOrders(ids, names, now, p)
	w.checkActions(ids, names, now, p)
	w.checkPositions(ids, names, now, p)
	w.checkTraderState(traders, now, p)
	w.checkPlans(ctx, user, now, p)
	w.checkFuturesPositions(user, now, p)
	w.checkProblems(user, now, p)
	w.checkDailyReport(user, p)

	if len(p.lines) > 0 {
		if err := w.send(ctx, strings.Join(p.lines, "\n")); err != nil {
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

func (w *alertWatcher) checkOrders(ids []string, names map[string]string, now int64, p *pending) {
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
		p.add(AlertOrderRejected, orderRejectedLine(p.lang, order, names[order.TraderID]))
		p.set[markOrders] = order.UpdatedAt
	}
}

func (w *alertWatcher) checkActions(ids []string, names map[string]string, now int64, p *pending) {
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
		p.add(AlertOrderRejected, actionFailedLine(p.lang, action, names[action.TraderID]))
		p.set[markActions] = action.Timestamp.UnixMilli()
	}
}

func (w *alertWatcher) checkPositions(ids []string, names map[string]string, now int64, p *pending) {
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
		p.add(AlertPositionClosed, positionClosedLine(p.lang, position, names[position.TraderID]))
		p.set[markPositions] = position.ExitTime
	}
}

// checkTraderState reports safe mode and blocked trading once each time they
// begin; a mark that clears re-arms the alert.
func (w *alertWatcher) checkTraderState(traders []*trader.Trader, now int64, p *pending) {
	if w.sources.TraderStatus == nil {
		return
	}
	for _, trader := range traders {
		status := w.sources.TraderStatus(trader.ID)
		safeMode, _ := status["safe_mode"].(bool)
		reason, _ := status["safe_mode_reason"].(string)
		w.standing(markSafeMode+trader.ID, safeMode, now, p, safeModeLine(p.lang, trader.Name, reason))
		blocked, _ := status["trading_blocked"].(bool)
		reason, _ = status["trading_error"].(string)
		w.standing(markBlocked+trader.ID, blocked, now, p, tradingBlockedLine(p.lang, trader.Name, reason))
	}
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
	lines := make(map[string]string)
	for _, run := range runs {
		if run.State == "needs_attention" {
			lines[markPlan+run.ID] = planAttentionLine(p.lang, run)
		}
	}
	w.ongoing(markPlan, AlertPlanAttention, lines, now, p)
}

// checkProblems reports what is wrong outside any trade: services that failed, traders
// halted or not loaded, trades the ledger could not book and accounts that stopped reading.
func (w *alertWatcher) checkProblems(user string, now int64, p *pending) {
	if w.sources.Problems != nil {
		lines := make(map[string]string)
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
	lines := make(map[string]string)
	for _, row := range accounts {
		failure, ok := failing[row.ID]
		if ok && w.now().Sub(failure.Since) >= accountFailingFor {
			lines[markAccount+row.ID] = withDetail(fmt.Sprintf(pick(p.lang, "Account unreadable: %s", "账户无法读取：%s"), row.Name), failure.Reason)
		}
	}
	w.ongoing(markAccount, AlertSystem, lines, now, p)
}

// ongoing tells of each line once, when its mark first appears, and re-arms it once it is
// gone. Marks under the prefix that are not in lines are gone.
func (w *alertWatcher) ongoing(prefix, kind string, lines map[string]string, now int64, p *pending) {
	keys := make([]string, 0, len(lines))
	for key := range lines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, seen := w.stores.Telegram.AlertMark(key); !seen {
			p.set[key] = now
			p.add(kind, lines[key])
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
	accounts, err := w.stores.Accounts.ListNames(user)
	if err != nil {
		logger.Warnf("Telegram alerts: read accounts: %v", err)
		return
	}
	testnet := make(map[string]bool, len(accounts))
	for _, row := range accounts {
		testnet[row.ID] = row.Testnet
	}
	if since, ok := w.since(markFuturesOpened, now, p); ok {
		positions, err := w.stores.Positions.FuturesOpenedSince(since, alertBatch)
		if err != nil {
			logger.Warnf("Telegram alerts: %v", err)
		}
		for _, position := range positions {
			p.add(AlertPlanAttention, futuresFilledLine(p.lang, position, testnet[position.AccountID]))
			p.set[markFuturesOpened] = position.EntryTime
		}
	}
	if since, ok := w.since(markFuturesClosed, now, p); ok {
		positions, err := w.stores.Positions.FuturesClosedSince(since, alertBatch)
		if err != nil {
			logger.Warnf("Telegram alerts: %v", err)
		}
		for _, position := range positions {
			p.add(AlertPositionClosed, positionClosedLine(p.lang, position, futuresName(p.lang, testnet[position.AccountID])))
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
	report, ok := w.dailyReport(user, today.AddDate(0, 0, -1), today, p.lang)
	if ok {
		p.add(AlertDailyReport, report)
	}
}

// dailyReport sums a day on the real accounts. Test money counts for nothing, and with no
// real account there is nothing to report. A figure that cannot be read is a dash.
func (w *alertWatcher) dailyReport(user string, from, to time.Time, lang string) (string, bool) {
	accounts, err := w.stores.Accounts.ListNames(user)
	if err != nil {
		logger.Warnf("Telegram alerts: read accounts: %v", err)
		return "", false
	}
	ids := []string{}
	for _, row := range accounts {
		if !row.Testnet {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return "", false
	}

	equity := "—"
	if w.sources.AccountEquity != nil {
		total, known := 0.0, true
		for _, id := range ids {
			value, ok := w.sources.AccountEquity(user, id)
			known = known && ok
			total += value
		}
		if known {
			equity = strconv.FormatFloat(total, 'f', 2, 64) + " USDT"
		}
	}
	realized := "—"
	if value, err := w.stores.Positions.RealizedBetween(ids, from, to); err != nil {
		logger.Warnf("Telegram alerts: %v", err)
	} else {
		realized = strconv.FormatFloat(value, 'f', 2, 64) + " USDT"
		if value >= 0 {
			realized = "+" + realized
		}
	}
	holding := "—"
	if positions, err := w.stores.Positions.OpenOnAccounts(ids); err != nil {
		logger.Warnf("Telegram alerts: %v", err)
	} else {
		names := make([]string, 0, len(positions))
		for _, position := range positions {
			names = append(names, position.Symbol+" "+strings.ToLower(position.Side))
		}
		holding = strconv.Itoa(len(positions))
		if len(names) > 0 {
			holding += " (" + strings.Join(names, ", ") + ")"
		}
	}
	cost := "—"
	if summary, err := w.stores.Calls.GetSummary("", from.Format("2006-01-02")); err != nil {
		logger.Warnf("Telegram alerts: %v", err)
	} else {
		cost = "$" + strconv.FormatFloat(summary.TotalUSD, 'f', 2, 64)
	}
	return fmt.Sprintf(pick(lang,
		"Daily report, %s UTC\nEquity: %s\nRealized: %s\nOpen positions: %s\nModel cost: %s",
		"每日汇总，%s UTC\n权益：%s\n已实现：%s\n持仓：%s\n模型花费：%s"),
		from.Format("2006-01-02"), equity, realized, holding, cost), true
}

func (w *alertWatcher) standing(key string, active bool, now int64, p *pending, line string) {
	_, seen := w.stores.Telegram.AlertMark(key)
	switch {
	case active && !seen:
		p.set[key] = now
		p.add(AlertSafeMode, line)
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

func withDetail(line, detail string) string {
	if detail == "" {
		return line
	}
	return line + ": " + detail
}

func orderRejectedLine(lang string, order *account.TraderOrder, trader string) string {
	return fmt.Sprintf(pick(lang, "Order rejected: %s %s, %s", "订单被拒：%s %s，%s"), order.Symbol, order.Side, trader)
}

func actionFailedLine(lang string, action trader.FailedAction, trader string) string {
	line := fmt.Sprintf(pick(lang, "Order failed: %s %s, %s", "下单失败：%s %s，%s"), action.Symbol, action.Action, trader)
	return withDetail(line, action.Error)
}

func positionClosedLine(lang string, position *account.TraderPosition, trader string) string {
	pnl := strconv.FormatFloat(position.RealizedPnL, 'f', 2, 64)
	if position.RealizedPnL >= 0 {
		pnl = "+" + pnl
	}
	return fmt.Sprintf(pick(lang, "Position closed: %s %s, %s, P/L %s USDT", "持仓已平：%s %s，%s，盈亏 %s USDT"), position.Symbol, strings.ToLower(position.Side), trader, pnl)
}

func safeModeLine(lang, trader, reason string) string {
	return withDetail(fmt.Sprintf(pick(lang, "Safe mode: %s", "安全模式：%s"), trader), reason)
}

func tradingBlockedLine(lang, trader, reason string) string {
	return withDetail(fmt.Sprintf(pick(lang, "Trading blocked: %s", "交易受阻：%s"), trader), reason)
}

func planAttentionLine(lang string, run PlanRun) string {
	return withDetail(fmt.Sprintf(pick(lang, "Futures plan needs attention: %s", "合约交易计划需要处理：%s"), run.Symbol), run.Error)
}

func futuresName(lang string, testnet bool) string {
	if testnet {
		return pick(lang, "Futures on testnet", "合约交易·测试网")
	}
	return pick(lang, "Futures", "合约交易")
}

func futuresFilledLine(lang string, position *account.TraderPosition, testnet bool) string {
	return fmt.Sprintf(pick(lang, "Entry filled: %s %s, %.8g at %.8g, %s", "入场已成交：%s %s，%.8g @ %.8g，%s"),
		position.Symbol, strings.ToLower(position.Side), position.EntryQuantity, position.EntryPrice, futuresName(lang, testnet))
}

func problemLine(lang string, problem Problem) string {
	var line string
	switch problem.Kind {
	case ProblemService:
		line = pick(lang, "Service failed: %s", "服务故障：%s")
	case ProblemTraderHalt:
		line = pick(lang, "Trader stopped opening positions: %s", "交易员已停止开仓：%s")
	case ProblemTraderLoad:
		line = pick(lang, "Trader could not start: %s", "交易员无法启动：%s")
	case ProblemFillSkipped:
		line = pick(lang, "Trade not booked: %s", "成交未入账：%s")
	default:
		line = pick(lang, "Problem: %s", "问题：%s")
	}
	detail := problem.Detail
	if lang == "zh" && problem.Kind == ProblemTraderHalt {
		detail = haltReasonZh.Replace(detail)
	}
	return withDetail(fmt.Sprintf(line, problem.Name), detail)
}

// haltReasonZh puts the reasons a trader halts for in Chinese. They are written in
// English where they are stored.
var haltReasonZh = strings.NewReplacer("max drawdown exceeded", "超过最大回撤", "daily loss limit exceeded", "超过当日亏损上限")

// TestAlertText is the message Settings sends to check that alerts arrive.
func TestAlertText(lang string) string {
	return pick(lang, "Vergex alerts are working.", "Vergex 告警工作正常。")
}
