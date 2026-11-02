package kind

import (
	"fmt"
	"strings"
)

// Bounds of an agent trader's config, which keep requests to the model a reasonable
// size and its risk limits sane.
const (
	MaxCandidateCoins = 10
	MaxOIPoolSize     = 100
	MaxPositions      = 8
	MaxTimeframes     = 4
	MinKlineCount     = 10
	MaxKlineCount     = 30
	MinLeverage       = 1
	MaxBTCETHLeverage = 20
	MaxAltLeverage    = 20
	MinPositionRatio  = 0.5
	MaxPositionRatio  = 10.0
	MinRiskReward     = 1.0
	MaxRiskReward     = 10.0
	MinMarginUsage    = 0.1
	MaxMarginUsage    = 1.0
	MinPositionSize   = 10.0
	MaxPositionSize   = 1000.0
	MinConfidence     = 50
	MaxConfidence     = 100
)

// AgentConfig is what an agent trader decides by: which coins, what it reads about
// them, its risk limits and its prompts.
type AgentConfig struct {
	CoinSource     CoinSource     `json:"coin_source"`
	Indicators     Indicators     `json:"indicators"`
	CustomPrompt   string         `json:"custom_prompt,omitempty"`
	RiskControl    RiskControl    `json:"risk_control"`
	PromptSections PromptSections `json:"prompt_sections,omitempty"`
}

// PromptSections are the editable parts of the system prompt.
type PromptSections struct {
	RoleDefinition   string `json:"role_definition,omitempty"`
	TradingFrequency string `json:"trading_frequency,omitempty"`
	EntryStandards   string `json:"entry_standards,omitempty"`
	DecisionProcess  string `json:"decision_process,omitempty"`
}

// CoinSource says which coins a trader considers.
type CoinSource struct {
	// SourceType is one of static, mixed, oi_top, oi_low, volume_top, gainers, losers.
	SourceType string `json:"source_type"`
	// StaticCoins is the list for static, and is added to mixed.
	StaticCoins []string `json:"static_coins,omitempty"`
	// ExcludedCoins are filtered out of every source.
	ExcludedCoins []string `json:"excluded_coins,omitempty"`
	// OI Top: largest open interest increase over OIWindow (suitable for long positions)
	UseOITop   bool `json:"use_oi_top"`
	OITopLimit int  `json:"oi_top_limit,omitempty"`
	// OI Low: largest open interest decrease over OIWindow (suitable for short positions)
	UseOILow   bool `json:"use_oi_low"`
	OILowLimit int  `json:"oi_low_limit,omitempty"`
	// Top N by 24h quote volume
	UseVolumeTop   bool `json:"use_volume_top,omitempty"`
	VolumeTopLimit int  `json:"volume_top_limit,omitempty"`
	// Top 24h price gainers
	UseGainers   bool `json:"use_gainers,omitempty"`
	GainersLimit int  `json:"gainers_limit,omitempty"`
	// Top 24h price losers
	UseLosers   bool `json:"use_losers,omitempty"`
	LosersLimit int  `json:"losers_limit,omitempty"`
	// OI change window: "1h" | "4h" | "24h" (default 4h)
	OIWindow string `json:"oi_window,omitempty"`
	// Number of highest-volume symbols evaluated for OI change (default 50)
	OIPoolSize int `json:"oi_pool_size,omitempty"`
	// Minimum 24h quote volume (USDT) for ranking-based sources; 0 = no floor
	MinQuoteVolume float64 `json:"min_quote_volume,omitempty"`
}

// Indicators says what market data a trader reads.
type Indicators struct {
	Klines Klines `json:"klines"`
	// raw kline data (OHLCV) - always enabled, required for agent analysis
	EnableRawKlines bool `json:"enable_raw_klines"`
	// technical indicator switches
	EnableEMA         bool `json:"enable_ema"`
	EnableMACD        bool `json:"enable_macd"`
	EnableRSI         bool `json:"enable_rsi"`
	EnableATR         bool `json:"enable_atr"`
	EnableBOLL        bool `json:"enable_boll"` // Bollinger Bands
	EnableVolume      bool `json:"enable_volume"`
	EnableOI          bool `json:"enable_oi"`           // open interest
	EnableFundingRate bool `json:"enable_funding_rate"` // funding rate
	// EMA period configuration
	EMAPeriods []int `json:"ema_periods,omitempty"` // default [20, 50]
	// RSI period configuration
	RSIPeriods []int `json:"rsi_periods,omitempty"` // default [7, 14]
	// ATR period configuration
	ATRPeriods []int `json:"atr_periods,omitempty"` // default [14]
	// BOLL period configuration (period, standard deviation multiplier is fixed at 2)
	BOLLPeriods []int `json:"boll_periods,omitempty"` // default [20] - can select multiple timeframes
}

// Klines says which candles a trader reads.
type Klines struct {
	// primary timeframe: "1m", "3m", "5m", "15m", "1h", "4h"
	PrimaryTimeframe string `json:"primary_timeframe"`
	// primary timeframe K-line count
	PrimaryCount int `json:"primary_count"`
	// longer timeframe
	LongerTimeframe string `json:"longer_timeframe,omitempty"`
	// longer timeframe K-line count
	LongerCount int `json:"longer_count,omitempty"`
	// whether to enable multi-timeframe analysis
	EnableMultiTimeframe bool `json:"enable_multi_timeframe"`
	// selected timeframe list
	SelectedTimeframes []string `json:"selected_timeframes,omitempty"`
}

// RiskControl holds an agent trader's limits.
type RiskControl struct {
	// Max number of coins held simultaneously (CODE ENFORCED)
	MaxPositions int `json:"max_positions"`

	// BTC/ETH exchange leverage for opening positions (agent guided)
	BTCETHMaxLeverage int `json:"btc_eth_max_leverage"`
	// Altcoin exchange leverage for opening positions (agent guided)
	AltcoinMaxLeverage int `json:"altcoin_max_leverage"`

	// BTC/ETH single position max value = equity × this ratio (CODE ENFORCED, default: 5)
	BTCETHMaxPositionValueRatio float64 `json:"btc_eth_max_position_value_ratio"`
	// Altcoin single position max value = equity × this ratio (CODE ENFORCED, default: 1)
	AltcoinMaxPositionValueRatio float64 `json:"altcoin_max_position_value_ratio"`

	// Max margin utilization (e.g. 0.9 = 90%) (CODE ENFORCED)
	MaxMarginUsage float64 `json:"max_margin_usage"`
	// Min position size in USDT (CODE ENFORCED)
	MinPositionSize float64 `json:"min_position_size"`

	// Min take_profit / stop_loss ratio (agent guided)
	MinRiskRewardRatio float64 `json:"min_risk_reward_ratio"`
	// Min agent confidence to open position (agent guided)
	MinConfidence int `json:"min_confidence"`
}

// DefaultAgent is the balanced starting config.
func DefaultAgent() *AgentConfig {
	return &AgentConfig{
		CoinSource: CoinSource{
			SourceType:  "static",
			StaticCoins: []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"},
		},
		Indicators: Indicators{
			Klines: Klines{
				PrimaryTimeframe:     "5m",
				PrimaryCount:         20,
				LongerTimeframe:      "4h",
				LongerCount:          10,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"5m", "15m", "1h"},
			},
			EnableRawKlines:   true, // Required - raw OHLCV data for agent analysis
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
			EMAPeriods:        []int{20, 50},
			RSIPeriods:        []int{7, 14},
			ATRPeriods:        []int{14},
			BOLLPeriods:       []int{20},
		},
		RiskControl: RiskControl{
			MaxPositions:                 3,   // Max 3 coins simultaneously (CODE ENFORCED)
			BTCETHMaxLeverage:            5,   // BTC/ETH exchange leverage (agent guided)
			AltcoinMaxLeverage:           5,   // Altcoin exchange leverage (agent guided)
			BTCETHMaxPositionValueRatio:  5.0, // BTC/ETH: max position = 5x equity (CODE ENFORCED)
			AltcoinMaxPositionValueRatio: 1.0, // Altcoin: max position = 1x equity (CODE ENFORCED)
			MaxMarginUsage:               0.9, // Max 90% margin usage (CODE ENFORCED)
			MinPositionSize:              12,  // Min 12 USDT per position (CODE ENFORCED)
			MinRiskRewardRatio:           3.0, // Min 3:1 profit/loss ratio (agent guided)
			MinConfidence:                75,  // Min 75% confidence (agent guided)
		},
		PromptSections: PromptSections{
			RoleDefinition: `# You are a professional cryptocurrency perpetual futures trading agent

Your task is to make trading decisions for USDT-margined perpetual contracts based on the provided market data. You are an experienced quantitative trader skilled in technical analysis and risk management.`,
			TradingFrequency: `# ⏱️ Trading Frequency Awareness

- Excellent trader: 2-4 trades per day ≈ 0.1-0.2 trades per hour
- >2 trades per hour = overtrading
- Single position holding time ≥ 30-60 minutes
If you find yourself trading every cycle → standards are too low; if closing positions in <30 minutes → too impulsive.`,
			EntryStandards: `# 🎯 Entry Standards (Strict)

Only enter positions when multiple signals resonate. Freely use any effective analysis methods, avoid low-quality behaviors such as single indicators, contradictory signals, sideways oscillation, or immediately restarting after closing positions.`,
			DecisionProcess: `# 📋 Decision Process

1. Check positions → whether to take profit/stop loss
2. Scan candidate coins + multi-timeframe → whether strong signals exist
3. Write chain of thought first, then output structured JSON`,
		},
	}
}

func agentPresets() []Preset {
	careful := DefaultAgent()
	careful.Indicators.Klines.PrimaryTimeframe = "1h"
	careful.Indicators.Klines.PrimaryCount = 30
	careful.RiskControl.MaxPositions = 1
	careful.RiskControl.BTCETHMaxLeverage = 3
	careful.RiskControl.AltcoinMaxLeverage = 3
	careful.RiskControl.MinConfidence = 82

	active := DefaultAgent()
	active.Indicators.Klines.PrimaryCount = 30
	active.RiskControl.MaxPositions = 5
	active.RiskControl.BTCETHMaxLeverage = 10
	active.RiskControl.AltcoinMaxLeverage = 10
	active.RiskControl.MinConfidence = 68

	return []Preset{
		{ID: "balanced", Config: DefaultAgent()},
		{ID: "careful", Config: careful},
		{ID: "active", Config: active},
	}
}

// RiskFields implements Config.
func (c *AgentConfig) RiskFields() []string { return []string{"risk_control"} }

// agentLimits is what the code enforces on an agent trader's orders.
type agentLimits struct {
	MaxPositions                 int     `json:"max_positions"`
	BTCETHMaxPositionValueRatio  float64 `json:"btc_eth_max_position_value_ratio"`
	AltcoinMaxPositionValueRatio float64 `json:"altcoin_max_position_value_ratio"`
	MaxMarginUsage               float64 `json:"max_margin_usage"`
	MinPositionSize              float64 `json:"min_position_size"`
	BTCETHMaxLeverage            int     `json:"btc_eth_max_leverage"`
	AltcoinMaxLeverage           int     `json:"altcoin_max_leverage"`
}

// Limits implements Config.
func (c *AgentConfig) Limits() any {
	risk := c.RiskControl
	return agentLimits{
		MaxPositions:                 risk.MaxPositions,
		BTCETHMaxPositionValueRatio:  risk.BTCETHMaxPositionValueRatio,
		AltcoinMaxPositionValueRatio: risk.AltcoinMaxPositionValueRatio,
		MaxMarginUsage:               risk.MaxMarginUsage,
		MinPositionSize:              risk.MinPositionSize,
		BTCETHMaxLeverage:            risk.BTCETHMaxLeverage,
		AltcoinMaxLeverage:           risk.AltcoinMaxLeverage,
	}
}

// Validate implements Config. A wrong choice is an error for whoever sent it to correct.
func (c *AgentConfig) Validate() error {
	if err := c.CoinSource.validate(); err != nil {
		return err
	}
	if err := c.Indicators.Klines.validate(); err != nil {
		return err
	}
	c.RiskControl.clamp()
	return nil
}

func (src *CoinSource) validate() error {
	src.StaticCoins = cleanSymbols(src.StaticCoins)
	src.ExcludedCoins = cleanSymbols(src.ExcludedCoins)

	single := func(flag *bool, limit *int) {
		src.UseOITop, src.UseOILow, src.UseVolumeTop, src.UseGainers, src.UseLosers = false, false, false, false, false
		if flag != nil {
			*flag = true
		}
		if limit != nil && *limit <= 0 {
			*limit = 3
		}
	}
	switch src.SourceType {
	case "oi_top":
		single(&src.UseOITop, &src.OITopLimit)
	case "oi_low":
		single(&src.UseOILow, &src.OILowLimit)
	case "volume_top":
		single(&src.UseVolumeTop, &src.VolumeTopLimit)
	case "gainers":
		single(&src.UseGainers, &src.GainersLimit)
	case "losers":
		single(&src.UseLosers, &src.LosersLimit)
	case "mixed":
		// keep the selected sources as configured
	case "static":
		single(nil, nil)
	default:
		return fmt.Errorf("coin_source.source_type %q is not one of static, mixed, oi_top, oi_low, volume_top, gainers, losers", src.SourceType)
	}
	switch src.OIWindow {
	case "", "1h", "4h", "24h":
	default:
		return fmt.Errorf("coin_source.oi_window %q is not one of 1h, 4h, 24h", src.OIWindow)
	}

	for _, limit := range []*int{&src.OITopLimit, &src.OILowLimit, &src.VolumeTopLimit, &src.GainersLimit, &src.LosersLimit} {
		*limit = min(*limit, MaxCandidateCoins)
	}
	src.OIPoolSize = min(src.OIPoolSize, MaxOIPoolSize)
	if len(src.StaticCoins) > MaxCandidateCoins {
		src.StaticCoins = src.StaticCoins[:MaxCandidateCoins]
	}
	return nil
}

func (k *Klines) validate() error {
	for _, timeframe := range []string{k.PrimaryTimeframe, k.LongerTimeframe} {
		if timeframe != "" && !validTimeframes[timeframe] {
			return fmt.Errorf("timeframe %q is not one of 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w", timeframe)
		}
	}
	seen := make(map[string]bool, len(k.SelectedTimeframes))
	selected := make([]string, 0, len(k.SelectedTimeframes))
	for _, timeframe := range k.SelectedTimeframes {
		if !validTimeframes[timeframe] {
			return fmt.Errorf("timeframe %q is not one of 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 6h, 8h, 12h, 1d, 3d, 1w", timeframe)
		}
		if !seen[timeframe] {
			seen[timeframe] = true
			selected = append(selected, timeframe)
		}
	}
	k.SelectedTimeframes = selected
	if len(k.SelectedTimeframes) > 0 {
		k.EnableMultiTimeframe = true
	}
	if len(k.SelectedTimeframes) > MaxTimeframes {
		k.SelectedTimeframes = k.SelectedTimeframes[:MaxTimeframes]
	}
	k.PrimaryCount = max(min(k.PrimaryCount, MaxKlineCount), MinKlineCount)
	k.LongerCount = min(k.LongerCount, MaxKlineCount)
	return nil
}

func (r *RiskControl) clamp() {
	r.MaxPositions = max(min(r.MaxPositions, MaxPositions), 1)
	r.BTCETHMaxLeverage = max(min(r.BTCETHMaxLeverage, MaxBTCETHLeverage), MinLeverage)
	r.AltcoinMaxLeverage = max(min(r.AltcoinMaxLeverage, MaxAltLeverage), MinLeverage)
	r.BTCETHMaxPositionValueRatio = max(min(r.BTCETHMaxPositionValueRatio, MaxPositionRatio), MinPositionRatio)
	r.AltcoinMaxPositionValueRatio = max(min(r.AltcoinMaxPositionValueRatio, MaxPositionRatio), MinPositionRatio)
	r.MinRiskRewardRatio = max(min(r.MinRiskRewardRatio, MaxRiskReward), MinRiskReward)
	r.MaxMarginUsage = max(min(r.MaxMarginUsage, MaxMarginUsage), MinMarginUsage)
	r.MinPositionSize = max(min(r.MinPositionSize, MaxPositionSize), MinPositionSize)
	r.MinConfidence = max(min(r.MinConfidence, MaxConfidence), MinConfidence)
}

func (r RiskControl) adjustments(after RiskControl) []string {
	var out []string
	note := func(name string, from, to float64) {
		if from != to {
			out = append(out, fmt.Sprintf("%s adjusted from %g to %g", name, from, to))
		}
	}
	note("max_positions", float64(r.MaxPositions), float64(after.MaxPositions))
	note("btc_eth_max_leverage", float64(r.BTCETHMaxLeverage), float64(after.BTCETHMaxLeverage))
	note("altcoin_max_leverage", float64(r.AltcoinMaxLeverage), float64(after.AltcoinMaxLeverage))
	note("btc_eth_max_position_value_ratio", r.BTCETHMaxPositionValueRatio, after.BTCETHMaxPositionValueRatio)
	note("altcoin_max_position_value_ratio", r.AltcoinMaxPositionValueRatio, after.AltcoinMaxPositionValueRatio)
	note("min_risk_reward_ratio", r.MinRiskRewardRatio, after.MinRiskRewardRatio)
	note("max_margin_usage", r.MaxMarginUsage, after.MaxMarginUsage)
	note("min_position_size", r.MinPositionSize, after.MinPositionSize)
	note("min_confidence", float64(r.MinConfidence), float64(after.MinConfidence))
	return out
}

var validTimeframes = map[string]bool{
	"1m": true, "3m": true, "5m": true, "15m": true, "30m": true,
	"1h": true, "2h": true, "4h": true, "6h": true, "8h": true, "12h": true,
	"1d": true, "3d": true, "1w": true,
}

// cleanSymbols writes symbols in capitals and drops blanks and repeats.
func cleanSymbols(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
