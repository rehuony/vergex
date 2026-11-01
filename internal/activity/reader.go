package activity

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"vergex/internal/agent"
	"vergex/internal/forecast"
	"vergex/internal/logger"
	"vergex/internal/market/feed"
	"vergex/internal/store"
)

const (
	readBatch = 500
	// timedBatch is larger because each read repeats the rows in overlap.
	timedBatch = 2000
	// overlap re-reads rows by the time they were written, for a write that was under way
	// during the last read and finished after it.
	overlap = 10 * time.Second
	// namesEvery bounds how often names are read.
	namesEvery = 30 * time.Second
	detailMax  = 200
)

// RefusedOrder is a Frontier order refused before it reached the exchange, so the
// ledger never hears of it.
type RefusedOrder struct {
	RunID     string
	PlanID    string
	AccountID string
	ClientID  string
	Role      string
	Symbol    string
	Quantity  string
	Error     string
	At        time.Time
}

// Sources is what the feed reads.
type Sources struct {
	Store *store.Store
	// Forecasts is nil when Forecast is unavailable.
	Forecasts *forecast.Store
	// RefusedOrders returns the orders refused at or after a time, oldest first. It must
	// read only the database. Nil when Frontier is unavailable.
	RefusedOrders func(since time.Time, limit int) ([]RefusedOrder, error)
}

type reader struct {
	sources Sources
	now     func() time.Time
	started time.Time

	// Where each table was read to. Tables with an append-only ID use it; the others
	// use a row's update time.
	calls, cycles, fills, records int64
	orders, positions             int64
	refused                       time.Time

	namesAt   time.Time
	traders   map[string]*store.Trader
	exchanges map[string]string
	hosts     map[string]string
}

func newReader(sources Sources, now func() time.Time) *reader {
	started := now().Add(-window)
	return &reader{
		sources:   sources,
		now:       now,
		started:   started,
		orders:    started.UnixMilli(),
		positions: started.UnixMilli(),
		refused:   started,
	}
}

// read returns new events and whether any table may have more.
func (r *reader) read() ([]Event, bool) {
	if !r.loadNames(false) {
		return nil, false
	}
	var events []Event
	more := false
	for _, source := range []func() ([]Event, bool, error){r.readCalls, r.readCycles, r.readOrders, r.readFills, r.readPositions, r.readRefused, r.readForecasts} {
		rows, full, err := source()
		if err != nil {
			logger.Warnf("Activity: %v", err)
			continue
		}
		events = append(events, rows...)
		more = more || full
	}
	return events, more
}

// loadNames refreshes the names events show. It reports false before anyone has
// registered, when there is nothing to read yet.
func (r *reader) loadNames(force bool) bool {
	if !force && r.traders != nil && r.now().Sub(r.namesAt) < namesEvery {
		return true
	}
	users, err := r.sources.Store.User().GetAll()
	if err != nil {
		logger.Warnf("Activity: read users: %v", err)
		return r.traders != nil
	}
	if len(users) == 0 {
		return false
	}
	user := users[0].ID
	traders, err := r.sources.Store.Trader().List(user)
	if err != nil {
		logger.Warnf("Activity: read traders: %v", err)
		return r.traders != nil
	}
	exchanges, err := r.sources.Store.Exchange().ListNames(user)
	if err != nil {
		logger.Warnf("Activity: read exchanges: %v", err)
		return r.traders != nil
	}
	models, err := r.sources.Store.AgentModel().ListEndpoints(user)
	if err != nil {
		logger.Warnf("Activity: read models: %v", err)
		return r.traders != nil
	}
	r.namesAt = r.now()
	r.traders = make(map[string]*store.Trader, len(traders))
	for _, trader := range traders {
		r.traders[trader.ID] = trader
	}
	r.exchanges = make(map[string]string, len(exchanges))
	for _, exchange := range exchanges {
		name := exchange.AccountName
		if name == "" {
			name = exchange.VenueName()
		}
		r.exchanges[exchange.ID] = name
	}
	r.hosts = make(map[string]string, len(models))
	for _, model := range models {
		r.hosts[model.ID] = agent.ProviderHost(model.Provider, model.CustomAPIURL)
	}
	return true
}

// A name not seen yet belongs to something just created, so look once more.
func (r *reader) trader(id string) *store.Trader {
	trader := r.traders[id]
	if trader == nil {
		r.loadNames(r.now().Sub(r.namesAt) >= pollEvery)
		trader = r.traders[id]
	}
	return trader
}

func (r *reader) traderEnd(id string) Endpoint {
	end := Endpoint{Block: BlockTraders, ID: id}
	if trader := r.trader(id); trader != nil {
		end.Name = trader.Name
	}
	return end
}

// sourceEnd is who an order in the ledger belongs to.
func (r *reader) sourceEnd(source, id string) Endpoint {
	switch source {
	case store.SourceTrader:
		return r.traderEnd(id)
	case store.SourceFrontier:
		return Endpoint{Block: BlockFrontier}
	}
	return Endpoint{Block: BlockOther, ID: source}
}

func (r *reader) accountEnd(id string) Endpoint {
	return Endpoint{Block: BlockAccounts, ID: id, Name: r.exchanges[id]}
}

func modelEnd(host string) Endpoint {
	return Endpoint{Block: BlockModels, ID: host, Name: host}
}

func (r *reader) caller(call store.AgentCall) Endpoint {
	switch call.Feature {
	case agent.FeatureFrontier:
		return Endpoint{Block: BlockFrontier}
	case agent.FeatureTelegram:
		return Endpoint{Block: BlockTelegram}
	case agent.FeatureTrader:
		if r.trader(call.SourceID) != nil {
			return r.traderEnd(call.SourceID)
		}
		return Endpoint{Block: BlockOther, ID: call.SourceID}
	}
	return Endpoint{Block: BlockOther, ID: call.Feature}
}

func (r *reader) readCalls() ([]Event, bool, error) {
	rows, err := r.sources.Store.AgentCall().After(r.calls, r.started, readBatch)
	if err != nil {
		return nil, false, fmt.Errorf("read model calls: %w", err)
	}
	var events []Event
	for _, row := range rows {
		r.calls = row.ID
		host := row.Host
		if host == "" {
			host = row.Provider
		}
		at := row.CreatedAt.UnixMilli()
		events = append(events, Event{
			ID:     fmt.Sprintf("agent:%d", row.ID),
			At:     at,
			Kind:   KindAgent,
			Type:   TypeAgentCall,
			From:   r.caller(row),
			To:     modelEnd(host),
			Tokens: row.PromptTokens + row.CompletionTokens,
			Detail: row.Model,
			Status: callStatus(row),
		})
		if !row.Unpriced && row.CostUSD > 0 {
			events = append(events, Event{
				ID:     fmt.Sprintf("cost:%d", row.ID),
				At:     at,
				Kind:   KindFunds,
				Type:   TypeAgentCost,
				From:   modelEnd(host),
				To:     Endpoint{Block: BlockAgentCost},
				Amount: row.CostUSD,
				Status: StatusOK,
			})
		}
	}
	return events, len(rows) == readBatch, nil
}

func callStatus(call store.AgentCall) string {
	if call.Status == store.CallFailed {
		return StatusFailed
	}
	return StatusOK
}

func (r *reader) readCycles() ([]Event, bool, error) {
	rows, err := r.sources.Store.Decision().CyclesAfter(r.cycles, r.started, readBatch)
	if err != nil {
		return nil, false, err
	}
	var events []Event
	for _, row := range rows {
		r.cycles = row.ID
		events = append(events, r.cycleEvents(row)...)
	}
	return events, len(rows) == readBatch, nil
}

// cycleEvents reports a trader's cycle. A record is saved once the cycle ends, after
// its orders, so its times are worked back to keep events in the order they happened.
func (r *reader) cycleEvents(row *store.DecisionRecord) []Event {
	decided := row.Timestamp
	action, symbol, actions := "", "", 0
	for _, item := range row.Decisions {
		if !item.Timestamp.IsZero() && item.Timestamp.Before(decided) {
			decided = item.Timestamp
		}
		if item.Action == "hold" || item.Action == "wait" || item.Action == "" {
			continue
		}
		if actions == 0 {
			action, symbol = item.Action, item.Symbol
		}
		actions++
	}
	decided = decided.Add(-time.Millisecond)

	trader := r.traderEnd(row.TraderID)
	var events []Event
	if coins := len(row.CandidateCoins); coins > 0 {
		events = append(events, Event{
			ID:       fmt.Sprintf("read:%d", row.ID),
			At:       decided.UnixMilli() - row.AgentDurationMs - 1,
			Kind:     KindMarket,
			Type:     TypeMarketRead,
			From:     Endpoint{Block: BlockMarket},
			To:       trader,
			Items:    coins,
			Ref:      strconv.FormatInt(row.ID, 10),
			TraderID: row.TraderID,
		})
	}
	status := StatusOK
	if !row.Success {
		status = StatusFailed
	}
	host := ""
	if known := r.trader(row.TraderID); known != nil {
		host = r.hosts[known.AgentModelID]
	}
	return append(events, Event{
		ID:       fmt.Sprintf("decision:%d", row.ID),
		At:       decided.UnixMilli(),
		Kind:     KindAgent,
		Type:     TypeDecision,
		From:     modelEnd(host),
		To:       trader,
		Symbol:   symbol,
		Action:   action,
		Items:    actions,
		Status:   status,
		Detail:   shorten(row.ErrorMessage),
		Ref:      strconv.FormatInt(row.ID, 10),
		TraderID: row.TraderID,
	})
}

func (r *reader) readOrders() ([]Event, bool, error) {
	rows, err := r.sources.Store.Order().UpdatedSince(r.orders-overlap.Milliseconds(), timedBatch)
	if err != nil {
		return nil, false, err
	}
	var events []Event
	for _, row := range rows {
		if row.UpdatedAt > r.orders {
			r.orders = row.UpdatedAt
		}
		action := row.OrderAction
		if action == "" {
			action = strings.ToLower(row.Side)
		}
		events = append(events, Event{
			ID:       fmt.Sprintf("order:%d:%s", row.ID, row.Status),
			At:       orderTime(row),
			Kind:     KindOrders,
			Type:     TypeOrder,
			From:     r.sourceEnd(row.Source, row.TraderID),
			To:       r.accountEnd(row.ExchangeID),
			Symbol:   row.Symbol,
			Action:   action,
			Quantity: row.Quantity,
			Status:   row.Status,
			TraderID: row.TraderID,
		})
	}
	return events, len(rows) == timedBatch, nil
}

// orderTime is when an order got to its status, not when the ledger heard: a sync
// catching up after an outage writes trades long after they happened.
func orderTime(order *store.TraderOrder) int64 {
	switch {
	case order.FilledAt > 0:
		return order.FilledAt
	case order.Status == store.OrderNew:
		return order.CreatedAt
	}
	return order.UpdatedAt
}

func (r *reader) readFills() ([]Event, bool, error) {
	rows, err := r.sources.Store.Order().FillsAfter(r.fills, r.started.UnixMilli(), readBatch)
	if err != nil {
		return nil, false, err
	}
	var events []Event
	for _, row := range rows {
		r.fills = row.ID
		if row.Commission == 0 {
			continue
		}
		commission := row.Commission
		if commission < 0 {
			commission = -commission
		}
		events = append(events, Event{
			ID:       fmt.Sprintf("fee:%d", row.ID),
			At:       row.CreatedAt,
			Kind:     KindFunds,
			Type:     TypeFee,
			From:     r.accountEnd(row.ExchangeID),
			To:       Endpoint{Block: BlockFees},
			Symbol:   row.Symbol,
			Amount:   commission,
			Asset:    row.CommissionAsset,
			Status:   StatusOK,
			TraderID: row.TraderID,
		})
	}
	return events, len(rows) == readBatch, nil
}

func (r *reader) readPositions() ([]Event, bool, error) {
	rows, err := r.sources.Store.Position().UpdatedSince(r.positions-overlap.Milliseconds(), timedBatch)
	if err != nil {
		return nil, false, err
	}
	var events []Event
	for _, row := range rows {
		if row.UpdatedAt > r.positions {
			r.positions = row.UpdatedAt
		}
		side := strings.ToLower(row.Side)
		account := r.accountEnd(row.ExchangeID)
		position := Endpoint{
			Block: BlockPositions,
			ID:    row.ExchangeID + ":" + row.Symbol + ":" + side,
			Name:  row.Symbol + " " + side,
		}
		if row.Leverage > 0 && row.EntryPrice > 0 && row.EntryTime > 0 {
			events = append(events, Event{
				ID:       fmt.Sprintf("margin:%d", row.ID),
				At:       row.EntryTime,
				Kind:     KindFunds,
				Type:     TypeMargin,
				From:     account,
				To:       position,
				Symbol:   row.Symbol,
				Amount:   row.EntryQuantity * row.EntryPrice / float64(row.Leverage),
				Status:   StatusOK,
				TraderID: row.TraderID,
			})
		}
		if row.Status == "CLOSED" && row.ExitTime > 0 {
			events = append(events, Event{
				ID:       fmt.Sprintf("realized:%d", row.ID),
				At:       row.ExitTime,
				Kind:     KindFunds,
				Type:     TypeRealized,
				From:     position,
				To:       account,
				Symbol:   row.Symbol,
				Amount:   row.RealizedPnL,
				Status:   StatusOK,
				TraderID: row.TraderID,
			})
		}
	}
	return events, len(rows) == timedBatch, nil
}

// readRefused reports Frontier orders refused before they reached the exchange. Any
// other order is in the ledger, and readOrders reports it.
func (r *reader) readRefused() ([]Event, bool, error) {
	if r.sources.RefusedOrders == nil {
		return nil, false, nil
	}
	orders, err := r.sources.RefusedOrders(r.refused, timedBatch)
	if err != nil {
		return nil, false, fmt.Errorf("read refused Frontier orders: %w", err)
	}
	var events []Event
	for _, order := range orders {
		if order.At.After(r.refused) {
			r.refused = order.At
		}
		quantity, _ := strconv.ParseFloat(order.Quantity, 64)
		events = append(events, Event{
			ID:       "plan:" + order.RunID + ":" + order.ClientID + ":" + StatusFailed,
			At:       order.At.UnixMilli(),
			Kind:     KindOrders,
			Type:     TypePlanOrder,
			From:     Endpoint{Block: BlockFrontier, ID: order.RunID, Name: order.Symbol},
			To:       r.accountEnd(order.AccountID),
			Symbol:   order.Symbol,
			Action:   strings.ToLower(order.Role),
			Quantity: quantity,
			Status:   StatusFailed,
			Detail:   shorten(order.Error),
			Ref:      order.PlanID,
		})
	}
	return events, len(orders) == timedBatch, nil
}

// readForecasts reports new forecasts per interval rather than per coin, since a
// closing candle produces one for every coin at once.
func (r *reader) readForecasts() ([]Event, bool, error) {
	if r.sources.Forecasts == nil {
		return nil, false, nil
	}
	rows, err := r.sources.Forecasts.SignalsAfter(r.records, r.started.UnixMilli(), readBatch)
	if err != nil {
		return nil, false, fmt.Errorf("read forecasts: %w", err)
	}
	groups := map[string]*Event{}
	var order []string
	for _, row := range rows {
		r.records = row.ID
		at := row.OpenTime + feed.IntervalMillis(row.Interval)
		group := groups[row.Interval]
		if group == nil {
			group = &Event{
				ID:     fmt.Sprintf("forecast:%d", row.ID),
				Kind:   KindMarket,
				Type:   TypeForecast,
				From:   Endpoint{Block: BlockMarket},
				To:     Endpoint{Block: BlockForecast},
				Detail: row.Interval,
			}
			groups[row.Interval] = group
			order = append(order, row.Interval)
		}
		group.Items++
		if at > group.At {
			group.At = at
		}
	}
	events := make([]Event, 0, len(order))
	for _, interval := range order {
		events = append(events, *groups[interval])
	}
	return events, len(rows) == readBatch, nil
}

func shorten(text string) string {
	if len(text) <= detailMax {
		return text
	}
	return strings.ToValidUTF8(text[:detailMax], "") + "…"
}
