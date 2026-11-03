package kind

import (
	"fmt"
	"strings"
)

// Bounds of a grid trader's config, the ranges its editor offers.
const (
	MinGridCount    = 5
	MaxGridCount    = 50
	MaxGridLeverage = 20
)

// GridConfig is what a grid trader decides by.
type GridConfig struct {
	// Trading pair (e.g., "BTCUSDT")
	Symbol string `json:"symbol"`
	// Number of grid levels (5-50)
	GridCount int `json:"grid_count"`
	// Total investment in USDT
	TotalInvestment float64 `json:"total_investment"`
	// Leverage (1-20)
	Leverage int `json:"leverage"`
	// Upper price boundary (0 = auto-calculate from ATR)
	UpperPrice float64 `json:"upper_price"`
	// Lower price boundary (0 = auto-calculate from ATR)
	LowerPrice float64 `json:"lower_price"`
	// Use ATR to auto-calculate bounds
	UseATRBounds bool `json:"use_atr_bounds"`
	// ATR multiplier for bound calculation (default 2.0)
	ATRMultiplier float64 `json:"atr_multiplier"`
	// Position distribution: "uniform" | "gaussian" | "pyramid"
	Distribution string `json:"distribution"`
	// Maximum drawdown percentage before emergency exit
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
	// Stop loss percentage per position
	StopLossPct float64 `json:"stop_loss_pct"`
	// Daily loss limit percentage
	DailyLossLimitPct float64 `json:"daily_loss_limit_pct"`
	// Use maker-only orders for lower fees
	UseMakerOnly bool `json:"use_maker_only"`
	// Enable automatic grid direction adjustment based on box breakouts
	EnableDirectionAdjust bool `json:"enable_direction_adjust"`
	// Direction bias ratio for long_bias/short_bias modes (default 0.7 = 70%/30%)
	DirectionBiasRatio float64 `json:"direction_bias_ratio"`
}

// DefaultGrid is the starting config of a grid trader.
func DefaultGrid() *GridConfig {
	return &GridConfig{
		Symbol:                "BTCUSDT",
		GridCount:             10,
		TotalInvestment:       1000,
		Leverage:              5,
		UseATRBounds:          true,
		ATRMultiplier:         2.0,
		Distribution:          "gaussian",
		MaxDrawdownPct:        15,
		StopLossPct:           5,
		DailyLossLimitPct:     10,
		UseMakerOnly:          true,
		EnableDirectionAdjust: false,
		DirectionBiasRatio:    0.7,
	}
}

// RiskFields implements Config.
func (c *GridConfig) RiskFields() []string {
	return []string{
		"symbol", "grid_count", "total_investment", "leverage", "upper_price", "lower_price",
		"use_atr_bounds", "atr_multiplier", "distribution", "max_drawdown_pct", "stop_loss_pct",
		"daily_loss_limit_pct", "enable_direction_adjust", "direction_bias_ratio",
	}
}

// Validate implements Config.
func (c *GridConfig) Validate() error {
	c.Symbol = strings.ToUpper(strings.TrimSpace(c.Symbol))
	if c.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	switch c.Distribution {
	case "uniform", "gaussian", "pyramid":
	default:
		return fmt.Errorf("distribution %q is not one of uniform, gaussian, pyramid", c.Distribution)
	}
	c.GridCount = max(min(c.GridCount, MaxGridCount), MinGridCount)
	c.Leverage = max(min(c.Leverage, MaxGridLeverage), MinLeverage)
	return nil
}
