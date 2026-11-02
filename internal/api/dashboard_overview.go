package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/service"
	"vergex/internal/telegram"
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

const (
	// overviewCacheTTL bounds how often an open dashboard reaches the exchanges,
	// which share one request allowance per IP with live trading.
	overviewCacheTTL = 15 * time.Second
	// overviewAccountTimeout gives up on a slow account without holding up the rest.
	overviewAccountTimeout = 10 * time.Second
)

type overviewAccount struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Venue        string   `json:"venue"`
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
	AgentModelID string `json:"agent_model_id"`
	AccountID    string `json:"account_id"`
	Kind         string `json:"kind"`
}

type overviewRun struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Symbol    string    `json:"symbol"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
}

type overviewPosition struct {
	AccountID        string  `json:"account_id"`
	TraderID         string  `json:"trader_id,omitempty"` // Empty when Futures manages it.
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
	Kind      string `json:"kind"`
	Name      string `json:"name,omitempty"`
	Detail    string `json:"detail,omitempty"`
	TraderID  string `json:"trader_id,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	PlanID    string `json:"plan_id,omitempty"`
}

// overviewMarket is what market data follows. Absent when it is unavailable.
type overviewMarket struct {
	Coins   int      `json:"coins"`
	Live    bool     `json:"live"`
	Symbols []string `json:"symbols"`
}

// overviewForecast is the forecast strategy in use. Absent when Forecast is unavailable.
type overviewForecast struct {
	Name    string   `json:"name"`
	Coins   int      `json:"coins"`
	Symbols []string `json:"symbols"`
}

type overviewResponse struct {
	GeneratedAt   time.Time          `json:"generated_at"`
	Accounts      []overviewAccount  `json:"accounts"`
	Models        []overviewModel    `json:"models"`
	Traders       []overviewTrader   `json:"traders"`
	Runs          []overviewRun      `json:"runs"`
	Services      []service.Entry    `json:"services"`
	Market        *overviewMarket    `json:"market,omitempty"`
	Forecast      *overviewForecast  `json:"forecast,omitempty"`
	TelegramBound bool               `json:"telegram_bound"`
	Positions     []overviewPosition `json:"positions"`
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

	if models, err := s.stores.Models.List(user); err == nil {
		for _, model := range models {
			if model.Enabled {
				out.Models = append(out.Models, overviewModel{ID: model.ID, Name: model.Name, Provider: model.Provider, Model: model.CustomModelName, Host: agent.ProviderHost(model.Provider, model.CustomAPIURL)})
			}
		}
	}

	traders, _ := s.stores.Traders.List(user)
	s.addOverviewTraders(user, traders, &out)
	s.addOverviewRuns(user, &out)

	accounts, _ := s.stores.Accounts.List(user)
	ready := s.addOverviewAccounts(user, accounts, &out)
	addOverviewAccountData(user, ready, &out)
	s.addOverviewSkippedFills(user, &out)
	s.addOverviewRealized(accounts, &out)

	s.addOverviewServices(&out)
	s.addOverviewFeeds(&out)
	return out
}

// addOverviewFeeds reports what the Dashboard's scene shows beside the accounts and
// traders. All of it is local.
func (s *Server) addOverviewFeeds(out *overviewResponse) {
	if s.marketData != nil {
		if status, err := s.marketData.Status(); err == nil {
			symbols, _ := s.marketData.ActiveCoins()
			out.Market = &overviewMarket{Coins: status.Coins, Live: status.Stream.Connected, Symbols: symbols}
		}
	}
	if s.forecast != nil {
		if strategy, err := s.forecast.Store.ActiveStrategy(time.Now()); err == nil {
			symbols, _ := s.forecast.Store.Coins(strategy.ID)
			out.Forecast = &overviewForecast{Name: strategy.Name, Coins: len(symbols), Symbols: symbols}
		}
	}
	if cfg, err := s.stores.Telegram.Get(); err == nil {
		out.TelegramBound = cfg.BotToken != "" && cfg.ChatID != 0
	}
}

func (s *Server) addOverviewTraders(user string, traders []*trader.Trader, out *overviewResponse) {
	for _, trader := range traders {
		row := overviewTrader{
			TraderID:     trader.ID,
			TraderName:   trader.Name,
			IsRunning:    trader.IsRunning,
			AgentModelID: trader.AgentModelID,
			AccountID:    trader.AccountID,
			Kind:         trader.Kind,
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
			if halt := at.ActiveHalt(); halt != nil {
				out.Attention = append(out.Attention, overviewAttention{Kind: "trader_halted", Name: trader.Name, Detail: halt.Reason, TraderID: trader.ID})
			}
		} else if loadErr := s.traderManager.GetLoadError(trader.ID); loadErr != nil && trader.IsRunning {
			out.Attention = append(out.Attention, overviewAttention{Kind: "trader_load_failed", Name: trader.Name, Detail: loadErr.Error(), TraderID: trader.ID})
		}
		out.Traders = append(out.Traders, row)
	}
}

// addOverviewRealized sums today's realized P/L on the user's accounts, whoever traded:
// a trader, Futures or a manual close. Test money does not count, and with no real account
// there is nothing to report.
func (s *Server) addOverviewRealized(accounts []*account.Account, out *overviewResponse) {
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if !account.Testnet {
			ids = append(ids, account.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	startOfDay := time.Now().UTC().Truncate(24 * time.Hour).UnixMilli()
	if realized, err := s.stores.Positions.RealizedOnAccounts(ids, startOfDay); err == nil {
		out.RealizedToday = &realized
	}
}

// settledRunStates are the run states with nothing left to watch.
var settledRunStates = map[string]bool{"completed": true, "cancelled": true, "expired": true}

// PlanRunsForAlerts lists a user's Futures runs for the Telegram alert watcher.
func (s *Server) PlanRunsForAlerts(ctx context.Context, user string) ([]telegram.PlanRun, error) {
	services, err := s.futuresService()
	if err != nil {
		return nil, err
	}
	runs, err := services.controller.ListContext(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make([]telegram.PlanRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, telegram.PlanRun{ID: run.ID, Symbol: run.Plan.Snapshot.Symbol, State: run.State, Error: run.Error})
	}
	return out, nil
}

// AccountEquityForAlerts is what an account holds, for the Telegram daily report. It reads
// the exchange past the cache, as its figure should be today's.
func (s *Server) AccountEquityForAlerts(user, accountID string) (float64, bool) {
	row, err := s.stores.Accounts.GetByID(user, accountID)
	if err != nil {
		return 0, false
	}
	state := probeAccountState(row, user)
	return state.TotalEquity, state.Status == accountStatusOK
}

// ProblemsForAlerts lists what Needs attention says that has no Telegram alert of its own.
// It runs every half minute whether or not anyone is looking, so none of it asks the
// exchange.
func (s *Server) ProblemsForAlerts(user string) []telegram.Problem {
	var out overviewResponse
	traders, _ := s.stores.Traders.List(user)
	s.addOverviewTraders(user, traders, &out)
	s.addOverviewServices(&out)
	problems := []telegram.Problem{}
	for _, row := range out.Attention {
		switch row.Kind {
		case telegram.ProblemService, telegram.ProblemTraderHalt, telegram.ProblemTraderLoad:
			problems = append(problems, telegram.Problem{Key: row.Kind + ":" + row.Name, Kind: row.Kind, Name: row.Name, Detail: row.Detail})
		}
	}
	// One line per trade, while it is under a day old, so history is not told again.
	fills, err := s.stores.Ledger.SkippedFillsSince(user, time.Now().Add(-24*time.Hour), 20)
	if err != nil {
		logger.Warnf("Alerts: read skipped fills: %v", err)
	}
	for _, fill := range fills {
		problems = append(problems, telegram.Problem{Key: telegram.ProblemFillSkipped + ":" + fill.ExchangeTradeID, Kind: telegram.ProblemFillSkipped, Name: fill.NativeSymbol, Detail: fill.Detail})
	}
	return problems
}

func (s *Server) addOverviewRuns(user string, out *overviewResponse) {
	services, err := s.futuresService()
	if err != nil {
		out.Services = append(out.Services, service.Entry{Name: "futures", State: service.Failed, Detail: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), overviewAccountTimeout)
	defer cancel()
	runs, err := services.controller.ListContext(ctx, user)
	if err != nil {
		out.Services = append(out.Services, service.Entry{Name: "futures", State: service.Degraded, Detail: err.Error()})
		return
	}
	for _, run := range runs {
		if settledRunStates[run.State] {
			continue
		}
		symbol := run.Plan.Snapshot.Symbol
		out.Runs = append(out.Runs, overviewRun{ID: run.ID, AccountID: run.AccountID, Symbol: symbol, State: run.State, UpdatedAt: run.UpdatedAt})
		if run.State == "needs_attention" {
			out.Attention = append(out.Attention, overviewAttention{Kind: "plan_needs_attention", Name: symbol, Detail: run.Error, AccountID: run.AccountID, PlanID: run.PlanID})
		}
	}
	state := service.Off
	if len(out.Runs) > 0 {
		state = service.Running
	}
	out.Services = append(out.Services, service.Entry{Name: "futures", State: state})
}

// addOverviewAccounts reports every account and returns the ones whose positions
// can be read.
func (s *Server) addOverviewAccounts(user string, accounts []*account.Account, out *overviewResponse) []*account.Account {
	states, err := s.getAccountStates(user)
	if err != nil {
		states = map[string]AccountState{}
	}
	ready := make([]*account.Account, 0, len(accounts))
	for _, account := range accounts {
		state, known := states[account.ID]
		row := overviewAccount{ID: account.ID, Name: account.Name, Venue: account.Venue, Status: accountStatusUnavailable}
		if row.Name == "" {
			row.Name = account.VenueName()
		}
		if account.Testnet {
			row.Name += " (testnet)"
		}
		if known {
			row.Status = state.Status
			row.Asset = state.Asset
			row.ErrorMessage = state.ErrorMessage
			// Test money counts towards nothing the page adds up.
			if state.Status == accountStatusOK && !account.Testnet {
				equity := state.TotalEquity
				row.TotalEquity = &equity
			}
		}
		out.Accounts = append(out.Accounts, row)
		switch row.Status {
		case accountStatusOK:
			ready = append(ready, account)
		case accountStatusMissingCredentials:
			// Not set up, which is a choice rather than a fault.
		default:
			// Its positions and orders are unknown, which is not the same as none.
			out.PositionsPartial = true
			out.Attention = append(out.Attention, overviewAttention{Kind: "account_" + row.Status, Name: row.Name, Detail: row.ErrorMessage, AccountID: account.ID})
		}
	}
	return ready
}

// How long a trade that could not be booked stays on the Dashboard, and how many it spells out.
const (
	skippedFillsShownFor = 30 * 24 * time.Hour
	skippedFillsSpelled  = 5
)

// addOverviewSkippedFills says which trades the ledger is missing. It takes one row, so
// its detail is not lost to the list merging rows of a kind.
func (s *Server) addOverviewSkippedFills(user string, out *overviewResponse) {
	fills, err := s.stores.Ledger.SkippedFillsSince(user, time.Now().Add(-skippedFillsShownFor), skippedFillsSpelled+1)
	if err != nil {
		logger.Warnf("Overview: read skipped fills: %v", err)
		return
	}
	if len(fills) == 0 {
		return
	}
	var lines []string
	for i, fill := range fills {
		if i == skippedFillsSpelled {
			lines = append(lines, "and more")
			break
		}
		line := fmt.Sprintf("trade %s: %s", fill.ExchangeTradeID, fill.Detail)
		// The row already names the first contract, and a phone has little room.
		if fill.NativeSymbol != fills[0].NativeSymbol {
			line = fill.NativeSymbol + " " + line
		}
		lines = append(lines, line)
	}
	out.Attention = append(out.Attention, overviewAttention{Kind: "account_fill_skipped", Name: fills[0].NativeSymbol, Detail: strings.Join(lines, "; ")})
}

type overviewAccountData struct {
	positions []overviewPosition
	failed    bool
}

// addOverviewAccountData reads the positions of each usable account in parallel.
// A slow or failing account marks the result partial, never wrong.
func addOverviewAccountData(user string, accounts []*account.Account, out *overviewResponse) {
	traders, runs := out.Traders, out.Runs
	results := make(chan overviewAccountData, len(accounts))
	for _, account := range accounts {
		account := account
		service.Go("overview-account", func() {
			results <- readOverviewAccount(user, account, traders, runs)
		})
	}

	timeout := time.After(overviewAccountTimeout)
	for range accounts {
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
		if positions[i].AccountID != positions[j].AccountID {
			return positions[i].AccountID < positions[j].AccountID
		}
		return positions[i].Symbol < positions[j].Symbol
	})
}

func readOverviewAccount(user string, account *account.Account, traders []overviewTrader, runs []overviewRun) overviewAccountData {
	var data overviewAccountData
	reader, err := buildAccountProbe(account, user)
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
		position.AccountID = account.ID
		position.TraderID = overviewPositionOwner(traders, runs, account.ID, position.Symbol)
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
// for Futures. A running trader owns its account's positions, as Futures refuses
// to touch them; otherwise an active plan on the symbol does; otherwise the trader
// bound to the account, whose page can still close it.
func overviewPositionOwner(traders []overviewTrader, runs []overviewRun, accountID, symbol string) string {
	bound := ""
	for _, trader := range traders {
		if trader.AccountID != accountID {
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
		if run.AccountID == accountID && instrument.Normalize(run.Symbol) == instrument.Normalize(symbol) {
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
	state := service.Off
	if running > 0 {
		state = service.Running
	}
	out.Services = append(out.Services, service.Entry{Name: "traders", State: state})
	out.Services = append(out.Services, service.All()...)
	sort.SliceStable(out.Services, func(i, j int) bool { return out.Services[i].Name < out.Services[j].Name })
	for _, svc := range out.Services {
		if svc.State == service.Failed {
			out.Attention = append(out.Attention, overviewAttention{Kind: "service_failed", Name: svc.Name, Detail: svc.Detail})
		}
	}
}
