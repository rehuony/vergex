package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vergex/internal/account"
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
)

// AlertKinds lists every kind, in the order Settings shows them.
var AlertKinds = []string{AlertOrderRejected, AlertSafeMode, AlertPositionClosed, AlertPlanAttention, AlertForecast}

const (
	alertInterval = 30 * time.Second
	// alertLookback bounds how far back events are reported after downtime.
	alertLookback = 24 * time.Hour
	alertBatch    = 20
)

const (
	markOrders    = "stream:orders"
	markActions   = "stream:actions"
	markPositions = "stream:positions"
	markSafeMode  = "safe_mode:"
	markBlocked   = "blocked:"
	markPlan      = "plan:"
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
	active := make(map[string]bool, len(runs))
	for _, run := range runs {
		if run.State == "needs_attention" {
			key := markPlan + run.ID
			active[key] = true
			if _, seen := w.stores.Telegram.AlertMark(key); !seen {
				p.set[key] = now
				p.add(AlertPlanAttention, planAttentionLine(p.lang, run))
			}
		}
	}
	keys, err := w.stores.Telegram.AlertMarkKeys(markPlan)
	if err != nil {
		logger.Warnf("Telegram alerts: %v", err)
		return
	}
	for _, key := range keys {
		if !active[key] {
			p.cleared = append(p.cleared, key)
		}
	}
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

// TestAlertText is the message Settings sends to check that alerts arrive.
func TestAlertText(lang string) string {
	return pick(lang, "Vergex alerts are working.", "Vergex 告警工作正常。")
}
