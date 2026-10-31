// Package forecast scores where a coin may go next from local market data,
// records each call and checks it later. It is an experiment to find out whether
// such signals have value, and nothing trades on them.
//
// Evaluate is a pure function of a strategy version and the data up to a moment,
// so live forecasts and backtests give the same answer for the same candle.
package forecast

import (
	"errors"
	"fmt"
	"sort"
)

// EngineVersion goes up whenever factor code changes what a version computes,
// so results from different code are not compared as if they were the same.
const EngineVersion = 1

// ScorerWeighted combines factors by weight, the only scorer so far. Strategies
// carry the field so other ways of scoring can use the same versions, records,
// verification and backtests.
const ScorerWeighted = "weighted"

// Factor names.
const (
	FactorTrend        = "trend"
	FactorMomentum     = "momentum"
	FactorRSI          = "rsi"
	FactorBollinger    = "bollinger"
	FactorReturns      = "returns"
	FactorVolume       = "volume"
	FactorCandle       = "candle"
	FactorStructure    = "structure"
	FactorStochastic   = "stochastic"
	FactorVWAP         = "vwap"
	FactorDivergence   = "divergence"
	FactorFunding      = "funding"
	FactorOpenInterest = "open_interest"
	FactorLongShort    = "long_short"
)

// CandleFactors need only candles; DerivativeFactors need derivatives data too.
var (
	CandleFactors = []string{
		FactorTrend, FactorMomentum, FactorRSI, FactorBollinger, FactorReturns, FactorVolume,
		FactorCandle, FactorStructure, FactorStochastic, FactorVWAP, FactorDivergence,
	}
	DerivativeFactors = []string{FactorFunding, FactorOpenInterest, FactorLongShort}
)

// Intervals are the intervals forecasts are made for.
var Intervals = []string{"15m", "1h", "4h", "1d"}

// FactorConfig is how one factor counts. A negative weight reads it the other
// way round: an overbought RSI supports a trend but argues against a range.
type FactorConfig struct {
	Enabled  bool    `json:"enabled"`
	Trending float64 `json:"trending"`
	Ranging  float64 `json:"ranging"`
}

// Config is everything a strategy version decides.
type Config struct {
	Scorer  string                  `json:"scorer"`
	Factors map[string]FactorConfig `json:"factors"`
	// ADXThreshold splits trending from ranging markets.
	ADXThreshold float64 `json:"adx_threshold"`
	// Slope turns the combined score into a probability.
	Slope float64 `json:"slope"`
	// DirectionThreshold is how far from 50% a probability must be to call a direction.
	DirectionThreshold float64 `json:"direction_threshold"`
	// Horizons is how many bars ahead each interval forecasts.
	Horizons map[string]int `json:"horizons"`
}

// DefaultConfig is the starting strategy. Its weights are a reasonable first
// guess, not a tested result; the backtests are there to judge them.
func DefaultConfig() Config {
	return Config{
		Scorer: ScorerWeighted,
		Factors: map[string]FactorConfig{
			FactorTrend:        {Enabled: true, Trending: 1.5, Ranging: 0.2},
			FactorMomentum:     {Enabled: true, Trending: 1.0, Ranging: 0.2},
			FactorRSI:          {Enabled: true, Trending: 0.4, Ranging: -1.0},
			FactorBollinger:    {Enabled: true, Trending: 0.3, Ranging: -1.0},
			FactorReturns:      {Enabled: true, Trending: 0.8, Ranging: -0.3},
			FactorVolume:       {Enabled: true, Trending: 0.6, Ranging: 0.4},
			FactorCandle:       {Enabled: true, Trending: 0.3, Ranging: 0.3},
			FactorStructure:    {Enabled: true, Trending: 1.0, Ranging: 0.2},
			FactorStochastic:   {Enabled: true, Trending: 0.2, Ranging: -0.8},
			FactorVWAP:         {Enabled: true, Trending: 0.4, Ranging: -0.6},
			FactorDivergence:   {Enabled: true, Trending: 0.6, Ranging: 0.8},
			FactorFunding:      {Enabled: true, Trending: 0.4, Ranging: 0.5},
			FactorOpenInterest: {Enabled: true, Trending: 0.6, Ranging: 0.3},
			FactorLongShort:    {Enabled: true, Trending: 0.3, Ranging: 0.4},
		},
		ADXThreshold:       25,
		Slope:              3,
		DirectionThreshold: 0.05,
		Horizons:           map[string]int{"15m": 4, "1h": 4, "4h": 4, "1d": 4},
	}
}

// Validate refuses a config that cannot produce a sensible forecast.
func (c Config) Validate() error {
	if c.Scorer != ScorerWeighted {
		return fmt.Errorf("unknown scorer %q", c.Scorer)
	}
	known := map[string]bool{}
	for _, name := range append(append([]string{}, CandleFactors...), DerivativeFactors...) {
		known[name] = true
	}
	anyOn := false
	for name, factor := range c.Factors {
		if !known[name] {
			return fmt.Errorf("unknown factor %q", name)
		}
		if factor.Trending < -5 || factor.Trending > 5 || factor.Ranging < -5 || factor.Ranging > 5 {
			return fmt.Errorf("factor %s: weights must be between -5 and 5", name)
		}
		if factor.Enabled && (factor.Trending != 0 || factor.Ranging != 0) {
			anyOn = true
		}
	}
	if !anyOn {
		return errors.New("turn on at least one factor")
	}
	if c.ADXThreshold < 5 || c.ADXThreshold > 60 {
		return errors.New("ADX threshold must be between 5 and 60")
	}
	if c.Slope <= 0 || c.Slope > 20 {
		return errors.New("slope must be above 0 and at most 20")
	}
	if c.DirectionThreshold < 0 || c.DirectionThreshold >= 0.5 {
		return errors.New("direction threshold must be from 0 to under 0.5")
	}
	for _, interval := range Intervals {
		if bars := c.Horizons[interval]; bars < 1 || bars > 96 {
			return fmt.Errorf("horizon for %s must be between 1 and 96 bars", interval)
		}
	}
	return nil
}

// sortedFactors lists a config's factors in a fixed order.
func sortedFactors(c Config) []string {
	names := make([]string, 0, len(c.Factors))
	for name := range c.Factors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
