// Package grid is the pure calculation behind a grid trader: boxes from candles, how
// ranging or trending a market is, breakouts and which way to lean.
package grid

import (
	"fmt"
	"time"
	"vergex/internal/market"
	"vergex/internal/market/instrument"
)

// BoxData represents multi-period Donchian channel (box) data
type BoxData struct {
	// Short-term box (72 1h candles = 3 days)
	ShortUpper float64 `json:"short_upper"`
	ShortLower float64 `json:"short_lower"`

	// Mid-term box (240 1h candles = 10 days)
	MidUpper float64 `json:"mid_upper"`
	MidLower float64 `json:"mid_lower"`

	// Long-term box (500 1h candles = ~21 days)
	LongUpper float64 `json:"long_upper"`
	LongLower float64 `json:"long_lower"`

	// Current price position relative to boxes
	CurrentPrice float64 `json:"current_price"`
}

// RegimeLevel represents the ranging classification level
type RegimeLevel string

const (
	RegimeLevelNarrow   RegimeLevel = "narrow"   // narrow range oscillation
	RegimeLevelStandard RegimeLevel = "standard" // standard oscillation
	RegimeLevelWide     RegimeLevel = "wide"     // wide range oscillation
	RegimeLevelVolatile RegimeLevel = "volatile" // extreme volatility
	RegimeLevelTrending RegimeLevel = "trending" // trending
)

// BreakoutLevel represents which box level has been broken
type BreakoutLevel string

const (
	BreakoutNone  BreakoutLevel = "none"
	BreakoutShort BreakoutLevel = "short"
	BreakoutMid   BreakoutLevel = "mid"
	BreakoutLong  BreakoutLevel = "long"
)

// Direction represents the current grid trading direction bias
type Direction string

const (
	DirectionNeutral   Direction = "neutral"    // 50% buy + 50% sell
	DirectionLong      Direction = "long"       // 100% buy
	DirectionShort     Direction = "short"      // 100% sell
	DirectionLongBias  Direction = "long_bias"  // 70% buy + 30% sell (default)
	DirectionShortBias Direction = "short_bias" // 30% buy + 70% sell (default)
)

// GetBuySellRatio returns the buy and sell ratio for this direction
// biasRatio is the ratio for biased directions (default 0.7 means 70%/30%)
func (d Direction) GetBuySellRatio(biasRatio float64) (buyRatio, sellRatio float64) {
	if biasRatio <= 0 || biasRatio > 1 {
		biasRatio = 0.7 // Default 70%/30%
	}

	switch d {
	case DirectionNeutral:
		return 0.5, 0.5
	case DirectionLong:
		return 1.0, 0.0
	case DirectionShort:
		return 0.0, 1.0
	case DirectionLongBias:
		return biasRatio, 1.0 - biasRatio
	case DirectionShortBias:
		return 1.0 - biasRatio, biasRatio
	default:
		return 0.5, 0.5
	}
}

// donchian calculates Donchian channel (highest high, lowest low) for given period
func donchian(klines []market.Kline, period int) (upper, lower float64) {
	if len(klines) == 0 || period <= 0 {
		return 0, 0
	}

	// Use all available klines if period > len(klines)
	start := len(klines) - period
	if start < 0 {
		start = 0
	}

	upper = klines[start].High
	lower = klines[start].Low

	for i := start + 1; i < len(klines); i++ {
		if klines[i].High > upper {
			upper = klines[i].High
		}
		if klines[i].Low < lower {
			lower = klines[i].Low
		}
	}

	return upper, lower
}

// Box period constants (in 1h candles)
const (
	ShortBoxPeriod = 72  // 3 days of 1h candles
	MidBoxPeriod   = 240 // 10 days of 1h candles
	LongBoxPeriod  = 500 // ~21 days of 1h candles
)

// BoxFromKlines calculates multi-period box data from klines
func BoxFromKlines(klines []market.Kline, currentPrice float64) *BoxData {
	box := &BoxData{
		CurrentPrice: currentPrice,
	}

	if len(klines) == 0 {
		return box
	}

	box.ShortUpper, box.ShortLower = donchian(klines, ShortBoxPeriod)
	box.MidUpper, box.MidLower = donchian(klines, MidBoxPeriod)
	box.LongUpper, box.LongLower = donchian(klines, LongBoxPeriod)

	return box
}

// FetchBox fetches 1h klines and calculates box data for a symbol
func FetchBox(symbol string) (*BoxData, error) {
	symbol = instrument.Normalize(symbol)

	// Fetch 500 1h klines
	var klines []market.Kline
	var err error

	klines, err = market.Klines(symbol, "1h", "binance", LongBoxPeriod)

	if err != nil {
		return nil, fmt.Errorf("failed to get 1h klines: %w", err)
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("no kline data available")
	}

	currentPrice := klines[len(klines)-1].Close

	return BoxFromKlines(klines, currentPrice), nil
}

// ============================================================================
// Task 6: Regime Level Classification
// ============================================================================

// ============================================================================
// Task 7: Breakout Detection
// ============================================================================

// DetectBoxBreakout checks if price has broken out of any box level
// Returns the highest breakout level and direction
func DetectBoxBreakout(box *BoxData) (BreakoutLevel, string) {
	if box == nil {
		return BreakoutNone, ""
	}

	price := box.CurrentPrice

	// Check long box first (highest priority)
	if price > box.LongUpper {
		return BreakoutLong, "up"
	}
	if price < box.LongLower {
		return BreakoutLong, "down"
	}

	// Check mid box
	if price > box.MidUpper {
		return BreakoutMid, "up"
	}
	if price < box.MidLower {
		return BreakoutMid, "down"
	}

	// Check short box
	if price > box.ShortUpper {
		return BreakoutShort, "up"
	}
	if price < box.ShortLower {
		return BreakoutShort, "down"
	}

	return BreakoutNone, ""
}

// ============================================================================
// Task 8: Breakout Confirmation Logic
// ============================================================================

const BreakoutConfirmRequired = 3 // 3 candles to confirm breakout

// BreakoutState tracks the current breakout state
type BreakoutState struct {
	Level        BreakoutLevel
	Direction    string
	ConfirmCount int
	StartTime    time.Time
}

// ConfirmBreakout updates breakout state and returns true if breakout is confirmed
func ConfirmBreakout(state *BreakoutState, currentLevel BreakoutLevel, direction string) bool {
	// If price returned to box, reset state
	if currentLevel == BreakoutNone {
		state.ConfirmCount = 0
		state.Level = BreakoutNone
		state.Direction = ""
		return false
	}

	// If same breakout continues, increment count
	if state.Level == currentLevel && state.Direction == direction {
		state.ConfirmCount++
	} else {
		// New breakout, reset count
		state.Level = currentLevel
		state.Direction = direction
		state.ConfirmCount = 1
		state.StartTime = time.Now()
	}

	return state.ConfirmCount >= BreakoutConfirmRequired
}

// ============================================================================
// Task 9: Breakout Handler
// ============================================================================

// BreakoutAction represents the action to take on breakout
type BreakoutAction int

const (
	BreakoutActionNone           BreakoutAction = iota
	BreakoutActionReducePosition                // Short box breakout: reduce to 50%
	BreakoutActionPauseGrid                     // Mid box breakout: pause grid + cancel orders
	BreakoutActionCloseAll                      // Long box breakout: pause + cancel + close all
)

// ActionForBreakout returns the appropriate action for a breakout level
func ActionForBreakout(level BreakoutLevel) BreakoutAction {
	switch level {
	case BreakoutShort:
		return BreakoutActionReducePosition
	case BreakoutMid:
		return BreakoutActionPauseGrid
	case BreakoutLong:
		return BreakoutActionCloseAll
	default:
		return BreakoutActionNone
	}
}

// ============================================================================
// Task 10: Grid Direction Adjustment
// ============================================================================

const (
	// BreakoutActionAdjustDirection adjusts grid direction based on breakout
	BreakoutActionAdjustDirection BreakoutAction = 4
)

// DetermineDirection determines the new grid direction based on box breakout
// currentDirection: the current grid direction
// breakoutLevel: which box level has been broken (short/mid/long)
// direction: breakout direction ("up" or "down")
// Returns: the new grid direction
func DetermineDirection(box *BoxData, currentDirection Direction, breakoutLevel BreakoutLevel, direction string) Direction {
	if box == nil {
		return currentDirection
	}

	price := box.CurrentPrice

	switch breakoutLevel {
	case BreakoutShort:
		// Short box breakout: bias direction
		// Still within mid box, so not a full trend yet
		if direction == "up" {
			return DirectionLongBias
		}
		return DirectionShortBias

	case BreakoutMid:
		// Mid box breakout: full direction
		// More significant move, commit fully
		if direction == "up" {
			return DirectionLong
		}
		return DirectionShort

	case BreakoutLong:
		// Long box breakout: handled by existing emergency logic
		// Return current direction, let existing handlers take over
		return currentDirection

	case BreakoutNone:
		// No breakout - check if we should recover toward neutral
		return DetermineRecoveryDirection(price, box, currentDirection)

	default:
		return currentDirection
	}
}

// DetermineRecoveryDirection determines if grid direction should recover toward neutral
// This implements the gradual recovery logic: long → long_bias → neutral ← short_bias ← short
func DetermineRecoveryDirection(price float64, box *BoxData, currentDirection Direction) Direction {
	// Check if price is back inside the short box
	insideShortBox := price >= box.ShortLower && price <= box.ShortUpper

	if !insideShortBox {
		// Still outside short box, maintain current direction
		return currentDirection
	}

	// Price is inside short box, start recovery toward neutral
	switch currentDirection {
	case DirectionLong:
		// Full long → bias long
		return DirectionLongBias
	case DirectionLongBias:
		// Bias long → neutral
		return DirectionNeutral
	case DirectionShort:
		// Full short → bias short
		return DirectionShortBias
	case DirectionShortBias:
		// Bias short → neutral
		return DirectionNeutral
	default:
		return currentDirection
	}
}

// BreakoutActionWithDirection returns the appropriate action for a breakout level
// when direction adjustment is enabled
func BreakoutActionWithDirection(level BreakoutLevel, enableDirectionAdjust bool) BreakoutAction {
	if !enableDirectionAdjust {
		// Fall back to original behavior
		return ActionForBreakout(level)
	}

	switch level {
	case BreakoutShort:
		// Short box breakout with direction adjustment: adjust direction instead of reducing position
		return BreakoutActionAdjustDirection
	case BreakoutMid:
		// Mid box breakout with direction adjustment: adjust to full direction
		return BreakoutActionAdjustDirection
	case BreakoutLong:
		// Long box breakout: always trigger emergency handling
		return BreakoutActionCloseAll
	default:
		return BreakoutActionNone
	}
}

// ShouldRecoverDirection checks if the current grid direction should start recovering toward neutral
func ShouldRecoverDirection(box *BoxData, currentDirection Direction) bool {
	if box == nil || currentDirection == DirectionNeutral {
		return false
	}

	price := box.CurrentPrice
	// Check if price is back inside the short box
	return price >= box.ShortLower && price <= box.ShortUpper
}
