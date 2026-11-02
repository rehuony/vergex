package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/trader/kind"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
)

// ============================================================================
// Type Definitions
// ============================================================================

// PositionInfo position information
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	PeakPnLPct       float64 `json:"peak_pnl_pct"` // Historical peak profit percentage
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // Position update timestamp (milliseconds)
}

// AccountInfo account information
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // Account equity
	AvailableBalance float64 `json:"available_balance"` // Available balance
	UnrealizedPnL    float64 `json:"unrealized_pnl"`    // Unrealized profit/loss
	TotalPnL         float64 `json:"total_pnl"`         // Total profit/loss
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // Total profit/loss percentage
	MarginUsed       float64 `json:"margin_used"`       // Used margin
	MarginUsedPct    float64 `json:"margin_used_pct"`   // Margin usage rate
	PositionCount    int     `json:"position_count"`    // Number of positions
}

// CandidateCoin candidate coin (from coin pool)
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // Sources: "static", "oi_top", "oi_low", "volume_top", "gainers", "losers"
}

// OITopData is the ranking context of a candidate coin (for agent decision reference)
type OITopData struct {
	Rank              int     // Rank within its source
	OIDeltaPercent    float64 // Open interest value change percentage over Window
	OIDeltaValue      float64 // Open interest value change over Window (USDT)
	PriceDeltaPercent float64 // 24h price change percentage
	QuoteVolume24h    float64 // 24h quote volume (USDT)
	Window            string  // OI change window; empty when the source has no OI data
}

// TradingStats trading statistics (for agent input)
type TradingStats struct {
	TotalTrades    int     `json:"total_trades"`     // Total number of trades (closed)
	WinRate        float64 `json:"win_rate"`         // Win rate (%)
	ProfitFactor   float64 `json:"profit_factor"`    // Profit factor
	SharpeRatio    float64 `json:"sharpe_ratio"`     // Sharpe ratio
	TotalPnL       float64 `json:"total_pnl"`        // Total profit/loss
	AvgWin         float64 `json:"avg_win"`          // Average win
	AvgLoss        float64 `json:"avg_loss"`         // Average loss
	MaxDrawdownPct float64 `json:"max_drawdown_pct"` // Maximum drawdown (%)
}

// RecentOrder recently completed order (for agent input)
type RecentOrder struct {
	Symbol       string  `json:"symbol"`        // Trading pair
	Side         string  `json:"side"`          // long/short
	EntryPrice   float64 `json:"entry_price"`   // Entry price
	ExitPrice    float64 `json:"exit_price"`    // Exit price
	RealizedPnL  float64 `json:"realized_pnl"`  // Realized profit/loss
	PnLPct       float64 `json:"pnl_pct"`       // Profit/loss percentage
	EntryTime    string  `json:"entry_time"`    // Entry time
	ExitTime     string  `json:"exit_time"`     // Exit time
	HoldDuration string  `json:"hold_duration"` // Hold duration, e.g. "2h30m"
}

// Context trading context (complete information passed to agent)
type Context struct {
	CurrentTime     string                      `json:"current_time"`
	RuntimeMinutes  int                         `json:"runtime_minutes"`
	CallCount       int                         `json:"call_count"`
	Account         AccountInfo                 `json:"account"`
	Positions       []PositionInfo              `json:"positions"`
	CandidateCoins  []CandidateCoin             `json:"candidate_coins"`
	PromptVariant   string                      `json:"prompt_variant,omitempty"`
	TradingStats    *TradingStats               `json:"trading_stats,omitempty"`
	RecentOrders    []RecentOrder               `json:"recent_orders,omitempty"`
	MarketDataMap   map[string]*Data            `json:"-"`
	MultiTFMarket   map[string]map[string]*Data `json:"-"`
	OITopDataMap    map[string]*OITopData       `json:"-"`
	BTCETHLeverage  int                         `json:"-"`
	AltcoinLeverage int                         `json:"-"`
	Timeframes      []string                    `json:"-"`
}

// Decision agent trading decision
type Decision struct {
	Symbol string `json:"symbol"`
	Action string `json:"action"` // Standard: "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	// Grid actions: "place_buy_limit", "place_sell_limit", "cancel_order", "cancel_all_orders", "pause_grid", "resume_grid", "adjust_grid"

	// Opening position parameters
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`

	// Grid trading parameters
	Price      float64 `json:"price,omitempty"`       // Limit order price (for grid)
	Quantity   float64 `json:"quantity,omitempty"`    // Order quantity (for grid)
	LevelIndex int     `json:"level_index,omitempty"` // Grid level index
	OrderID    string  `json:"order_id,omitempty"`    // Order ID (for cancel)

	// Common parameters
	Confidence int     `json:"confidence,omitempty"` // Confidence level (0-100)
	RiskUSD    float64 `json:"risk_usd,omitempty"`   // Maximum USD risk
	Reasoning  string  `json:"reasoning"`

	// Rejected says why validation refused this decision. It is not carried out, and the
	// rest of the reply still is.
	Rejected string `json:"-"`
}

// FullDecision agent's complete decision (including chain of thought)
type FullDecision struct {
	SystemPrompt    string     `json:"system_prompt"`
	UserPrompt      string     `json:"user_prompt"`
	CoTTrace        string     `json:"cot_trace"`
	Decisions       []Decision `json:"decisions"`
	RawResponse     string     `json:"raw_response"`
	Timestamp       time.Time  `json:"timestamp"`
	AgentDurationMs int64      `json:"agent_duration_ms,omitempty"`
	// ConversationID is where this exchange with the model is kept; empty when it could
	// not be kept.
	ConversationID string `json:"-"`
}

// ============================================================================
// Engine - Core Execution Engine
// ============================================================================

// Engine builds an agent trader's prompts from its config
type Engine struct {
	config      *kind.AgentConfig
	ranking     RankingSource
	venue       string
	listing     ListingSource // symbols tradable on venue; nil means no filter
	rankingMu   sync.RWMutex
	rankingData map[string]*OITopData // ranking context from the last GetCandidateCoins call
}

// RankingSource provides free market rankings built from public exchange data.
type RankingSource interface {
	TopVolume(ctx context.Context) ([]binance.Ticker, error)
	PriceMovers(ctx context.Context, gainers bool, minQuoteVolume float64) ([]binance.Ticker, error)
	OIChanges(ctx context.Context, window string, poolSize int) ([]binance.OIChange, error)
}

// ListingSource returns the symbols tradable on a venue (generic form, e.g. BTCUSDT).
type ListingSource func(ctx context.Context) (map[string]bool, error)

// NewEngine creates an engine on a config
func NewEngine(config *kind.AgentConfig) *Engine {
	return &Engine{config: config, ranking: binance.Default()}
}

// WithVenue restricts ranked coin sources to symbols tradable on the trader's
// exchange. Rankings are built from Binance data, so an OKX trader would
// otherwise receive coins that OKX does not list.
func (e *Engine) WithVenue(venue string) *Engine {
	e.venue = venue
	switch venue {
	case "okx":
		e.listing = okx.Default().USDTSwaps
	default:
		e.listing = nil
	}
	return e
}

// GetRiskControlConfig gets risk control configuration
func (e *Engine) GetRiskControlConfig() kind.RiskControl {
	return e.config.RiskControl
}

// GetLanguage returns the language of the prompt sections
func (e *Engine) GetLanguage() Language {
	return detectLanguage(e.config.PromptSections.RoleDefinition)
}

// GetConfig gets the config
func (e *Engine) GetConfig() *kind.AgentConfig {
	return e.config
}

// ============================================================================
// Candidate Coins
// ============================================================================

// rankedSources are the coin sources computed from free Binance rankings.
var rankedSources = []string{"oi_top", "oi_low", "volume_top", "gainers", "losers"}

// GetCandidateCoins gets candidate coins based on the config
func (e *Engine) GetCandidateCoins() ([]CandidateCoin, error) {
	coinSource := e.config.CoinSource
	ranking := make(map[string]*OITopData)
	defer func() {
		e.rankingMu.Lock()
		e.rankingData = ranking
		e.rankingMu.Unlock()
	}()

	switch coinSource.SourceType {
	case "static":
		return e.filterExcludedCoins(e.staticCandidates()), nil

	case "oi_top", "oi_low", "volume_top", "gainers", "losers":
		return e.rankedCoins(coinSource.SourceType, e.sourceLimit(coinSource.SourceType), ranking)

	case "mixed":
		var order []string
		symbolSources := make(map[string][]string)
		add := func(symbol, source string) {
			if _, seen := symbolSources[symbol]; !seen {
				order = append(order, symbol)
			}
			symbolSources[symbol] = append(symbolSources[symbol], source)
		}
		for _, source := range rankedSources {
			if !e.sourceEnabled(source) {
				continue
			}
			coins, err := e.rankedCoins(source, e.sourceLimit(source), ranking)
			if err != nil {
				logger.Warnf("⚠️  Failed to get %s coins: %v", source, err)
				continue
			}
			for _, coin := range coins {
				add(coin.Symbol, source)
			}
		}
		for _, coin := range e.staticCandidates() {
			add(coin.Symbol, "static")
		}
		candidates := make([]CandidateCoin, 0, len(order))
		for _, symbol := range order {
			candidates = append(candidates, CandidateCoin{Symbol: symbol, Sources: symbolSources[symbol]})
		}
		return e.filterExcludedCoins(candidates), nil

	default:
		return nil, fmt.Errorf("unsupported coin source type %q", coinSource.SourceType)
	}
}

// RankingContext returns the ranking data (OI change, 24h volume and price
// change) of candidates selected by the last GetCandidateCoins call.
func (e *Engine) RankingContext() map[string]*OITopData {
	if e == nil {
		return nil
	}
	e.rankingMu.RLock()
	defer e.rankingMu.RUnlock()
	return e.rankingData
}

func (e *Engine) staticCandidates() []CandidateCoin {
	candidates := make([]CandidateCoin, 0, len(e.config.CoinSource.StaticCoins))
	for _, symbol := range e.config.CoinSource.StaticCoins {
		candidates = append(candidates, CandidateCoin{Symbol: instrument.Normalize(symbol), Sources: []string{"static"}})
	}
	return candidates
}

func (e *Engine) sourceEnabled(source string) bool {
	src := e.config.CoinSource
	switch source {
	case "oi_top":
		return src.UseOITop
	case "oi_low":
		return src.UseOILow
	case "volume_top":
		return src.UseVolumeTop
	case "gainers":
		return src.UseGainers
	case "losers":
		return src.UseLosers
	}
	return false
}

func (e *Engine) sourceLimit(source string) int {
	src := e.config.CoinSource
	limit := 0
	switch source {
	case "oi_top":
		limit = src.OITopLimit
	case "oi_low":
		limit = src.OILowLimit
	case "volume_top":
		limit = src.VolumeTopLimit
	case "gainers":
		limit = src.GainersLimit
	case "losers":
		limit = src.LosersLimit
	}
	if limit <= 0 {
		limit = 3
	}
	if limit > kind.MaxCandidateCoins {
		limit = kind.MaxCandidateCoins
	}
	return limit
}

func (e *Engine) excludedSet() map[string]bool {
	excluded := make(map[string]bool, len(e.config.CoinSource.ExcludedCoins))
	for _, coin := range e.config.CoinSource.ExcludedCoins {
		excluded[instrument.Normalize(coin)] = true
	}
	return excluded
}

// rankedCoins selects up to limit coins from a free Binance ranking. Excluded
// coins are skipped before counting so they never consume a slot.
func (e *Engine) rankedCoins(source string, limit int, ranking map[string]*OITopData) ([]CandidateCoin, error) {
	if e.ranking == nil {
		return nil, fmt.Errorf("no ranking source configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	src := e.config.CoinSource
	excluded := e.excludedSet()
	var tradable map[string]bool
	if e.listing != nil {
		var err error
		if tradable, err = e.listing(ctx); err != nil {
			return nil, fmt.Errorf("failed to load %s listing: %w", e.venue, err)
		}
	}
	candidates := make([]CandidateCoin, 0, limit)
	skipped := 0
	take := func(symbol string, data *OITopData) bool {
		symbol = instrument.Normalize(symbol)
		if excluded[symbol] {
			return false
		}
		if tradable != nil && !tradable[symbol] {
			skipped++
			return false
		}
		data.Rank = len(candidates) + 1
		candidates = append(candidates, CandidateCoin{Symbol: symbol, Sources: []string{source}})
		if _, exists := ranking[symbol]; !exists {
			ranking[symbol] = data
		}
		return len(candidates) >= limit
	}

	switch source {
	case "oi_top", "oi_low":
		changes, err := e.ranking.OIChanges(ctx, src.OIWindow, src.OIPoolSize)
		if err != nil {
			return nil, fmt.Errorf("failed to compute open interest ranking: %w", err)
		}
		for i := range changes {
			c := changes[i]
			if source == "oi_low" {
				c = changes[len(changes)-1-i]
			}
			// oi_top only takes rising OI, oi_low only falling OI
			if (source == "oi_top" && c.OIChangePct <= 0) || (source == "oi_low" && c.OIChangePct >= 0) {
				break
			}
			if c.QuoteVolume < src.MinQuoteVolume {
				continue
			}
			if take(c.Symbol, &OITopData{
				OIDeltaPercent:    c.OIChangePct,
				OIDeltaValue:      c.OIChangeValue,
				PriceDeltaPercent: c.PriceChangePercent,
				QuoteVolume24h:    c.QuoteVolume,
				Window:            c.Window,
			}) {
				break
			}
		}

	case "volume_top", "gainers", "losers":
		var tickers []binance.Ticker
		var err error
		if source == "volume_top" {
			tickers, err = e.ranking.TopVolume(ctx)
		} else {
			tickers, err = e.ranking.PriceMovers(ctx, source == "gainers", src.MinQuoteVolume)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load 24h tickers: %w", err)
		}
		for _, t := range tickers {
			if (source == "gainers" && t.PriceChangePercent <= 0) || (source == "losers" && t.PriceChangePercent >= 0) {
				break
			}
			if t.QuoteVolume < src.MinQuoteVolume {
				continue
			}
			if take(t.Symbol, &OITopData{PriceDeltaPercent: t.PriceChangePercent, QuoteVolume24h: t.QuoteVolume}) {
				break
			}
		}

	default:
		return nil, fmt.Errorf("unknown ranking source %q", source)
	}

	if tradable != nil {
		logger.Infof("✅ Loaded %d %s candidates (Binance public data, %d not listed on %s skipped)", len(candidates), source, skipped, e.venue)
	} else {
		logger.Infof("✅ Loaded %d %s candidates (Binance public data)", len(candidates), source)
	}
	return candidates, nil
}

// filterExcludedCoins removes excluded coins from the candidates list
func (e *Engine) filterExcludedCoins(candidates []CandidateCoin) []CandidateCoin {
	if len(e.config.CoinSource.ExcludedCoins) == 0 {
		return candidates
	}
	excluded := e.excludedSet()
	filtered := make([]CandidateCoin, 0, len(candidates))
	for _, c := range candidates {
		if !excluded[c.Symbol] {
			filtered = append(filtered, c)
		} else {
			logger.Infof("🚫 Excluded coin: %s", c.Symbol)
		}
	}
	return filtered
}

// ============================================================================
// External Data
// ============================================================================

// ============================================================================
// Helper Functions
// ============================================================================

// detectLanguage detects language from text content
// Returns LangChinese if text contains Chinese characters, otherwise LangEnglish
func detectLanguage(text string) Language {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return LangChinese
		}
	}
	return LangEnglish
}
