package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"vergex/internal/llm"
	"vergex/internal/market"
	"vergex/internal/safe"
	"vergex/internal/servicestate"
	"vergex/internal/store"
	"vergex/internal/telegram"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

const (
	// overviewCacheTTL bounds how often an open dashboard reaches the exchanges,
	// which share one request allowance per IP with live trading.
	overviewCacheTTL = 15 * time.Second
	// overviewExchangeTimeout gives up on a slow account without holding up the rest.
	overviewExchangeTimeout = 10 * time.Second
)

type overviewAccount struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ExchangeType string   `json:"exchange_type"`
	Status       string   `json:"status"`
	Asset        string   `json:"asset,omitempty"`
	TotalEquity  *float64 `json:"total_equity,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

type overviewModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Host     string `json:"host,omitempty"`
}

type overviewTrader struct {
	TraderID     string `json:"trader_id"`
	TraderName   string `json:"trader_name"`
	IsRunning    bool   `json:"is_running"`
	AIModelID    string `json:"ai_model_id"`
	ExchangeID   string `json:"exchange_id"`
	StrategyName string `json:"strategy_name,omitempty"`
}

type overviewRun struct {
	ID         string    `json:"id"`
	ExchangeID string    `json:"exchange_id"`
	Symbol     string    `json:"symbol"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type overviewPosition struct {
	ExchangeID       string  `json:"exchange_id"`
	TraderID         string  `json:"trader_id,omitempty"` // Empty when Frontier manages it.
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	Quantity         float64 `json:"quantity"`
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	Leverage         float64 `json:"leverage"`
}

// overviewAttention is something the user should act on. Kind picks the wording;
// the ids say where to send them.
type overviewAttention struct {
	Kind       string `json:"kind"`
	Name       string `json:"name,omitempty"`
	Detail     string `json:"detail,omitempty"`
	TraderID   string `json:"trader_id,omitempty"`
	ExchangeID string `json:"exchange_id,omitempty"`
	PlanID     string `json:"plan_id,omitempty"`
}

// overviewMarket is what market data follows. Absent when it is unavailable.
type overviewMarket struct {
	Coins int  `json:"coins"`
	Live  bool `json:"live"`
}

// overviewForecast is the forecast strategy in use. Absent when Forecast is unavailable.
type overviewForecast struct {
	Name  string `json:"name"`
	Coins int64  `json:"coins"`
}

type overviewResponse struct {
	GeneratedAt   time.Time            `json:"generated_at"`
	Accounts      []overviewAccount    `json:"accounts"`
	Models        []overviewModel      `json:"models"`
	Traders       []overviewTrader     `json:"traders"`
	Runs          []overviewRun        `json:"runs"`
	Services      []servicestate.Entry `json:"services"`
	Market        *overviewMarket      `json:"market,omitempty"`
	Forecast      *overviewForecast    `json:"forecast,omitempty"`
	TelegramBound bool                 `json:"telegram_bound"`
	Positions     []overviewPosition   `json:"positions"`
	// PositionsPartial is true when an account's positions could not be read.
	PositionsPartial bool                `json:"positions_partial"`
	RealizedToday    *float64            `json:"realized_today"`
	Attention        []overviewAttention `json:"attention"`
}

type overviewCache struct {
	flight  singleflight.Group
	mu      sync.Mutex
	entries map[string]cachedOverview
}

type cachedOverview struct {
	value overviewResponse
	at    time.Time
}

// get returns the user's overview, building it at most once per TTL however many
// tabs or requests ask for it.
func (c *overviewCache) get(user string, build func() overviewResponse) overviewResponse {
	c.mu.Lock()
	entry, ok := c.entries[user]
	c.mu.Unlock()
	if ok && time.Since(entry.at) < overviewCacheTTL {
		return entry.value
	}
	value, _, _ := c.flight.Do(user, func() (any, error) {
		built := build()
		c.mu.Lock()
		if c.entries == nil {
			c.entries = map[string]cachedOverview{}
		}
		c.entries[user] = cachedOverview{value: built, at: time.Now()}
		c.mu.Unlock()
		return built, nil
	})
	return value.(overviewResponse)
}

func (s *Server) handleOverview(c *gin.Context) {
	user := c.GetString("user_id")
	c.JSON(http.StatusOK, s.overview.get(user, func() overviewResponse {
		return s.buildOverview(user)
	}))
}

func (s *Server) buildOverview(user string) overviewResponse {
	out := overviewResponse{
		GeneratedAt: time.Now().UTC(),
		Accounts:    []overviewAccount{},
		Models:      []overviewModel{},
		Traders:     []overviewTrader{},
		Runs:        []overviewRun{},
		Positions:   []overviewPosition{},
		Attention:   []overviewAttention{},
	}

	if models, err := s.store.AIModel().List(user); err == nil {
		for _, model := range models {
			if model.Enabled {
				out.Models = append(out.Models, overviewModel{ID: model.ID, Name: model.Name, Provider: model.Provider, Model: model.CustomModelName, Host: llm.ProviderHost(model.Provider, model.CustomAPIURL)})
			}
		}
	}

	traders, _ := s.store.Trader().List(user)
	s.addOverviewTraders(user, traders, &out)
	s.addOverviewRuns(user, &out)

	exchanges, _ := s.store.Exchange().List(user)
	ready := s.addOverviewAccounts(user, exchanges, &out)
	addOverviewExchangeData(user, ready, &out)

	s.addOverviewServices(&out)
	s.addOverviewFeeds(&out)
	return out
}

// addOverviewFeeds reports what the Dashboard's scene shows beside the accounts and
// traders. All of it is local.
func (s *Server) addOverviewFeeds(out *overviewResponse) {
	if s.marketData != nil {
		if status, err := s.marketData.Status(); err == nil {
			out.Market = &overviewMarket{Coins: status.Coins, Live: status.Stream.Connected}
		}
	}
	if s.forecast != nil {
		if strategy, err := s.forecast.Store.ActiveStrategy(time.Now()); err == nil {
			coins, _ := s.forecast.Store.CoinCount(strategy.ID)
			out.Forecast = &overviewForecast{Name: strategy.Name, Coins: coins}
		}
	}
	if cfg, err := s.store.TelegramConfig().Get(); err == nil {
		out.TelegramBound = cfg.BotToken != "" && cfg.ChatID != 0
	}
}

func (s *Server) addOverviewTraders(user string, traders []*store.Trader, out *overviewResponse) {
	ids := make([]string, 0, len(traders))
	for _, trader := range traders {
		ids = append(ids, trader.ID)
		row := overviewTrader{
			TraderID:   trader.ID,
			TraderName: trader.Name,
			IsRunning:  trader.IsRunning,
			AIModelID:  trader.AIModelID,
			ExchangeID: trader.ExchangeID,
		}
		if trader.StrategyID != "" {
			if strategy, err := s.store.Strategy().Get(user, trader.StrategyID); err == nil {
				row.StrategyName = strategy.Name
			}
		}
		if at, err := s.traderManager.GetTrader(trader.ID); err == nil {
			status := at.GetStatus()
			if running, ok := status["is_running"].(bool); ok {
				row.IsRunning = running
			}
			if blocked, _ := status["trading_blocked"].(bool); blocked {
				detail, _ := status["trading_error"].(string)
				out.Attention = append(out.Attention, overviewAttention{Kind: "trader_blocked", Name: trader.Name, Detail: detail, TraderID: trader.ID})
			}
			if safeMode, _ := status["safe_mode"].(bool); safeMode {
				detail, _ := status["safe_mode_reason"].(string)
				out.Attention = append(out.Attention, overviewAttention{Kind: "trader_safe_mode", Name: trader.Name, Detail: detail, TraderID: trader.ID})
			}
		} else if loadErr := s.traderManager.GetLoadError(trader.ID); loadErr != nil && trader.IsRunning {
			out.Attention = append(out.Attention, overviewAttention{Kind: "trader_load_failed", Name: trader.Name, Detail: loadErr.Error(), TraderID: trader.ID})
		}
		out.Traders = append(out.Traders, row)
	}

	startOfDay := time.Now().UTC().Truncate(24 * time.Hour).UnixMilli()
	if realized, err := s.store.Position().RealizedSince(ids, startOfDay); err == nil && len(ids) > 0 {
		out.RealizedToday = &realized
	}
}

// settledRunStates are the run states with nothing left to watch.
var settledRunStates = map[string]bool{"completed": true, "cancelled": true, "expired": true}

// PlanRunsForAlerts lists a user's Frontier runs for the Telegram alert watcher.
func (s *Server) PlanRunsForAlerts(ctx context.Context, user string) ([]telegram.PlanRun, error) {
	services, err := s.planningService()
	if err != nil {
		return nil, err
	}
	runs, err := services.live.ListContext(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make([]telegram.PlanRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, telegram.PlanRun{ID: run.ID, Symbol: run.Plan.Snapshot.Symbol, State: run.State, Error: run.Error})
	}
	return out, nil
}

func (s *Server) addOverviewRuns(user string, out *overviewResponse) {
	services, err := s.planningService()
	if err != nil {
		out.Services = append(out.Services, servicestate.Entry{Name: "frontier", State: servicestate.Failed, Detail: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), overviewExchangeTimeout)
	defer cancel()
	runs, err := services.live.ListContext(ctx, user)
	if err != nil {
		out.Services = append(out.Services, servicestate.Entry{Name: "frontier", State: servicestate.Degraded, Detail: err.Error()})
		return
	}
	for _, run := range runs {
		if settledRunStates[run.State] {
			continue
		}
		symbol := run.Plan.Snapshot.Symbol
		out.Runs = append(out.Runs, overviewRun{ID: run.ID, ExchangeID: run.ExchangeID, Symbol: symbol, State: run.State, UpdatedAt: run.UpdatedAt})
		if run.State == "needs_attention" {
			out.Attention = append(out.Attention, overviewAttention{Kind: "plan_needs_attention", Name: symbol, Detail: run.Error, ExchangeID: run.ExchangeID, PlanID: run.PlanID})
		}
	}
	state := servicestate.Off
	if len(out.Runs) > 0 {
		state = servicestate.Running
	}
	out.Services = append(out.Services, servicestate.Entry{Name: "frontier", State: state})
}

// addOverviewAccounts reports every account and returns the ones whose positions
// can be read.
func (s *Server) addOverviewAccounts(user string, exchanges []*store.Exchange, out *overviewResponse) []*store.Exchange {
	states, err := s.getExchangeAccountStates(user)
	if err != nil {
		states = map[string]ExchangeAccountState{}
	}
	ready := make([]*store.Exchange, 0, len(exchanges))
	for _, exchange := range exchanges {
		state, known := states[exchange.ID]
		row := overviewAccount{ID: exchange.ID, Name: exchange.AccountName, ExchangeType: exchange.ExchangeType, Status: exchangeAccountStatusUnavailable}
		if row.Name == "" {
			row.Name = exchange.Name
		}
		if known {
			row.Status = state.Status
			row.Asset = state.Asset
			row.ErrorMessage = state.ErrorMessage
			if state.Status == exchangeAccountStatusOK {
				equity := state.TotalEquity
				row.TotalEquity = &equity
			}
		}
		out.Accounts = append(out.Accounts, row)
		switch row.Status {
		case exchangeAccountStatusOK:
			ready = append(ready, exchange)
		case exchangeAccountStatusDisabled, exchangeAccountStatusMissingCredentials:
			// Not set up, which is a choice rather than a fault.
		default:
			// Its positions and orders are unknown, which is not the same as none.
			out.PositionsPartial = true
			out.Attention = append(out.Attention, overviewAttention{Kind: "account_" + row.Status, Name: row.Name, Detail: row.ErrorMessage, ExchangeID: exchange.ID})
		}
	}
	return ready
}

type overviewAccountData struct {
	positions []overviewPosition
	failed    bool
}

// addOverviewExchangeData reads the positions of each usable account in parallel.
// A slow or failing account marks the result partial, never wrong.
func addOverviewExchangeData(user string, exchanges []*store.Exchange, out *overviewResponse) {
	traders, runs := out.Traders, out.Runs
	results := make(chan overviewAccountData, len(exchanges))
	for _, exchange := range exchanges {
		exchange := exchange
		safe.GoNamed("overview-account", func() {
			results <- readOverviewAccount(user, exchange, traders, runs)
		})
	}

	timeout := time.After(overviewExchangeTimeout)
	for range exchanges {
		select {
		case data := <-results:
			out.Positions = append(out.Positions, data.positions...)
			out.PositionsPartial = out.PositionsPartial || data.failed
		case <-timeout:
			out.PositionsPartial = true
			sortOverviewPositions(out.Positions)
			return
		}
	}
	sortOverviewPositions(out.Positions)
}

func sortOverviewPositions(positions []overviewPosition) {
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].ExchangeID != positions[j].ExchangeID {
			return positions[i].ExchangeID < positions[j].ExchangeID
		}
		return positions[i].Symbol < positions[j].Symbol
	})
}

func readOverviewAccount(user string, exchange *store.Exchange, traders []overviewTrader, runs []overviewRun) overviewAccountData {
	var data overviewAccountData
	reader, err := buildExchangeProbeTrader(exchange, user)
	if err != nil {
		data.failed = true
		return data
	}
	raw, err := reader.GetPositions()
	if err != nil {
		data.failed = true
		return data
	}

	for _, values := range raw {
		position, ok := normalizeOverviewPosition(values)
		if !ok {
			continue
		}
		position.ExchangeID = exchange.ID
		position.TraderID = overviewPositionOwner(traders, runs, exchange.ID, position.Symbol)
		data.positions = append(data.positions, position)
	}
	return data
}

// normalizeOverviewPosition reads the exchange adapters' position maps, whose
// field names differ from the API's other position fields.
func normalizeOverviewPosition(values map[string]interface{}) (overviewPosition, bool) {
	symbol, _ := values["symbol"].(string)
	side, _ := values["side"].(string)
	side = strings.ToLower(side)
	if symbol == "" || (side != "long" && side != "short") {
		return overviewPosition{}, false
	}
	quantity, _ := extractFirstNumeric(values, "quantity", "positionAmt", "size")
	if quantity < 0 {
		quantity = -quantity
	}
	entry, _ := extractFirstNumeric(values, "entry_price", "entryPrice")
	mark, _ := extractFirstNumeric(values, "mark_price", "markPrice")
	pnl, _ := extractFirstNumeric(values, "unrealized_pnl", "unRealizedProfit")
	leverage, _ := extractFirstNumeric(values, "leverage")
	position := overviewPosition{Symbol: symbol, Side: side, Quantity: quantity, EntryPrice: entry, MarkPrice: mark, UnrealizedPnL: pnl, Leverage: leverage}
	if margin := quantity * entry; margin > 0 && leverage > 0 {
		position.UnrealizedPnLPct = pnl / (margin / leverage) * 100
	}
	return position, true
}

// overviewPositionOwner names the trader whose page manages a position, or ""
// for Frontier. A running trader owns its account's positions, as Frontier refuses
// to touch them; otherwise an active plan on the symbol does; otherwise the trader
// bound to the account, whose page can still close it.
func overviewPositionOwner(traders []overviewTrader, runs []overviewRun, exchangeID, symbol string) string {
	bound := ""
	for _, trader := range traders {
		if trader.ExchangeID != exchangeID {
			continue
		}
		if trader.IsRunning {
			return trader.TraderID
		}
		if bound == "" {
			bound = trader.TraderID
		}
	}
	for _, run := range runs {
		if run.ExchangeID == exchangeID && market.Normalize(run.Symbol) == market.Normalize(symbol) {
			return ""
		}
	}
	return bound
}

func (s *Server) addOverviewServices(out *overviewResponse) {
	running := 0
	for _, trader := range out.Traders {
		if trader.IsRunning {
			running++
		}
	}
	state := servicestate.Off
	if running > 0 {
		state = servicestate.Running
	}
	out.Services = append(out.Services, servicestate.Entry{Name: "traders", State: state})
	out.Services = append(out.Services, servicestate.All()...)
	sort.SliceStable(out.Services, func(i, j int) bool { return out.Services[i].Name < out.Services[j].Name })
	for _, service := range out.Services {
		if service.State == servicestate.Failed {
			out.Attention = append(out.Attention, overviewAttention{Kind: "service_failed", Name: service.Name, Detail: service.Detail})
		}
	}
}

func decimalValue(raw string) float64 {
	value, _ := extractFirstNumeric(map[string]interface{}{"v": raw}, "v")
	return value
}
