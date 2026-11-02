package grid

import (
	"testing"
	"vergex/internal/market"
)

func TestDetectBoxBreakout(t *testing.T) {
	box := &BoxData{
		ShortUpper:   100,
		ShortLower:   90,
		MidUpper:     105,
		MidLower:     85,
		LongUpper:    110,
		LongLower:    80,
		CurrentPrice: 95,
	}

	// No breakout
	level, direction := DetectBoxBreakout(box)
	if level != BreakoutNone {
		t.Errorf("Expected no breakout, got %v", level)
	}

	// Short breakout up
	box.CurrentPrice = 101
	level, direction = DetectBoxBreakout(box)
	if level != BreakoutShort || direction != "up" {
		t.Errorf("Expected short breakout up, got %v %v", level, direction)
	}

	// Mid breakout down
	box.CurrentPrice = 84
	level, direction = DetectBoxBreakout(box)
	if level != BreakoutMid || direction != "down" {
		t.Errorf("Expected mid breakout down, got %v %v", level, direction)
	}

	// Long breakout up
	box.CurrentPrice = 112
	level, direction = DetectBoxBreakout(box)
	if level != BreakoutLong || direction != "up" {
		t.Errorf("Expected long breakout up, got %v %v", level, direction)
	}
}

func TestBreakoutConfirmation(t *testing.T) {
	state := &BreakoutState{
		Level:        BreakoutNone,
		Direction:    "",
		ConfirmCount: 0,
	}

	// First detection
	confirmed := ConfirmBreakout(state, BreakoutShort, "up")
	if confirmed || state.ConfirmCount != 1 {
		t.Errorf("Expected not confirmed, count=1, got confirmed=%v count=%d", confirmed, state.ConfirmCount)
	}

	// Second confirmation
	confirmed = ConfirmBreakout(state, BreakoutShort, "up")
	if confirmed || state.ConfirmCount != 2 {
		t.Errorf("Expected not confirmed, count=2, got confirmed=%v count=%d", confirmed, state.ConfirmCount)
	}

	// Third confirmation - should confirm
	confirmed = ConfirmBreakout(state, BreakoutShort, "up")
	if !confirmed || state.ConfirmCount != 3 {
		t.Errorf("Expected confirmed, count=3, got confirmed=%v count=%d", confirmed, state.ConfirmCount)
	}

	// Reset on price return
	state.ConfirmCount = 2
	confirmed = ConfirmBreakout(state, BreakoutNone, "")
	if state.ConfirmCount != 0 {
		t.Errorf("Expected count reset to 0, got %d", state.ConfirmCount)
	}
}

func TestGetBreakoutAction(t *testing.T) {
	tests := []struct {
		level    BreakoutLevel
		expected BreakoutAction
	}{
		{BreakoutNone, BreakoutActionNone},
		{BreakoutShort, BreakoutActionReducePosition},
		{BreakoutMid, BreakoutActionPauseGrid},
		{BreakoutLong, BreakoutActionCloseAll},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			action := ActionForBreakout(tt.level)
			if action != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, action)
			}
		})
	}
}

// ============================================================================
// Grid Direction Tests
// ============================================================================

func TestGetBuySellRatio(t *testing.T) {
	tests := []struct {
		name      string
		direction Direction
		biasRatio float64
		wantBuy   float64
		wantSell  float64
	}{
		{"neutral", DirectionNeutral, 0.7, 0.5, 0.5},
		{"long", DirectionLong, 0.7, 1.0, 0.0},
		{"short", DirectionShort, 0.7, 0.0, 1.0},
		{"long_bias_default", DirectionLongBias, 0.7, 0.7, 0.3},
		{"short_bias_default", DirectionShortBias, 0.7, 0.3, 0.7},
		{"long_bias_custom", DirectionLongBias, 0.8, 0.8, 0.2},
		{"short_bias_custom", DirectionShortBias, 0.8, 0.2, 0.8},
		{"invalid_bias_uses_default", DirectionLongBias, 0, 0.7, 0.3},
		{"negative_bias_uses_default", DirectionLongBias, -1, 0.7, 0.3},
	}

	const tolerance = 0.0001
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buy, sell := tt.direction.GetBuySellRatio(tt.biasRatio)
			buyDiff := buy - tt.wantBuy
			sellDiff := sell - tt.wantSell
			if buyDiff < -tolerance || buyDiff > tolerance || sellDiff < -tolerance || sellDiff > tolerance {
				t.Errorf("GetBuySellRatio(%v, %v) = (%v, %v), want (%v, %v)",
					tt.direction, tt.biasRatio, buy, sell, tt.wantBuy, tt.wantSell)
			}
		})
	}
}

func TestDetermineDirection(t *testing.T) {
	box := &BoxData{
		ShortUpper:   100,
		ShortLower:   90,
		MidUpper:     105,
		MidLower:     85,
		LongUpper:    110,
		LongLower:    80,
		CurrentPrice: 95,
	}

	tests := []struct {
		name             string
		currentDirection Direction
		breakoutLevel    BreakoutLevel
		direction        string
		expected         Direction
	}{
		// Short box breakouts
		{
			name:             "short_breakout_up_neutral",
			currentDirection: DirectionNeutral,
			breakoutLevel:    BreakoutShort,
			direction:        "up",
			expected:         DirectionLongBias,
		},
		{
			name:             "short_breakout_down_neutral",
			currentDirection: DirectionNeutral,
			breakoutLevel:    BreakoutShort,
			direction:        "down",
			expected:         DirectionShortBias,
		},
		// Mid box breakouts
		{
			name:             "mid_breakout_up",
			currentDirection: DirectionLongBias,
			breakoutLevel:    BreakoutMid,
			direction:        "up",
			expected:         DirectionLong,
		},
		{
			name:             "mid_breakout_down",
			currentDirection: DirectionShortBias,
			breakoutLevel:    BreakoutMid,
			direction:        "down",
			expected:         DirectionShort,
		},
		// Long box breakout - maintains current (emergency handling)
		{
			name:             "long_breakout_maintains",
			currentDirection: DirectionLong,
			breakoutLevel:    BreakoutLong,
			direction:        "up",
			expected:         DirectionLong,
		},
		// No breakout - tests recovery logic
		{
			name:             "no_breakout_neutral_stays",
			currentDirection: DirectionNeutral,
			breakoutLevel:    BreakoutNone,
			direction:        "",
			expected:         DirectionNeutral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineDirection(box, tt.currentDirection, tt.breakoutLevel, tt.direction)
			if result != tt.expected {
				t.Errorf("DetermineDirection() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDetermineRecoveryDirection(t *testing.T) {
	box := &BoxData{
		ShortUpper:   100,
		ShortLower:   90,
		MidUpper:     105,
		MidLower:     85,
		LongUpper:    110,
		LongLower:    80,
		CurrentPrice: 95, // Inside short box
	}

	tests := []struct {
		name             string
		price            float64
		currentDirection Direction
		expected         Direction
	}{
		// Inside short box - should recover
		{"long_to_long_bias", 95, DirectionLong, DirectionLongBias},
		{"long_bias_to_neutral", 95, DirectionLongBias, DirectionNeutral},
		{"short_to_short_bias", 95, DirectionShort, DirectionShortBias},
		{"short_bias_to_neutral", 95, DirectionShortBias, DirectionNeutral},
		{"neutral_stays_neutral", 95, DirectionNeutral, DirectionNeutral},

		// Outside short box - should maintain
		{"long_outside_stays", 101, DirectionLong, DirectionLong},
		{"short_outside_stays", 89, DirectionShort, DirectionShort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineRecoveryDirection(tt.price, box, tt.currentDirection)
			if result != tt.expected {
				t.Errorf("DetermineRecoveryDirection(%v, %v) = %v, want %v",
					tt.price, tt.currentDirection, result, tt.expected)
			}
		})
	}
}

func TestGetBreakoutActionWithDirection(t *testing.T) {
	tests := []struct {
		name                  string
		level                 BreakoutLevel
		enableDirectionAdjust bool
		expected              BreakoutAction
	}{
		// Direction adjustment disabled - original behavior
		{"short_disabled", BreakoutShort, false, BreakoutActionReducePosition},
		{"mid_disabled", BreakoutMid, false, BreakoutActionPauseGrid},
		{"long_disabled", BreakoutLong, false, BreakoutActionCloseAll},

		// Direction adjustment enabled
		{"short_enabled", BreakoutShort, true, BreakoutActionAdjustDirection},
		{"mid_enabled", BreakoutMid, true, BreakoutActionAdjustDirection},
		{"long_enabled", BreakoutLong, true, BreakoutActionCloseAll}, // Long always triggers emergency
		{"none_enabled", BreakoutNone, true, BreakoutActionNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := BreakoutActionWithDirection(tt.level, tt.enableDirectionAdjust)
			if action != tt.expected {
				t.Errorf("BreakoutActionWithDirection(%v, %v) = %v, want %v",
					tt.level, tt.enableDirectionAdjust, action, tt.expected)
			}
		})
	}
}

func TestShouldRecoverDirection(t *testing.T) {
	box := &BoxData{
		ShortUpper:   100,
		ShortLower:   90,
		MidUpper:     105,
		MidLower:     85,
		LongUpper:    110,
		LongLower:    80,
		CurrentPrice: 95,
	}

	tests := []struct {
		name      string
		price     float64
		direction Direction
		expected  bool
	}{
		{"neutral_inside_no_recovery", 95, DirectionNeutral, false},
		{"long_inside_should_recover", 95, DirectionLong, true},
		{"long_outside_no_recovery", 101, DirectionLong, false},
		{"short_inside_should_recover", 95, DirectionShort, true},
		{"short_outside_no_recovery", 89, DirectionShort, false},
		{"long_bias_inside_should_recover", 95, DirectionLongBias, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			box.CurrentPrice = tt.price
			result := ShouldRecoverDirection(box, tt.direction)
			if result != tt.expected {
				t.Errorf("ShouldRecoverDirection(price=%v, %v) = %v, want %v",
					tt.price, tt.direction, result, tt.expected)
			}
		})
	}
}

// TestCalculateIntradaySeries_VolumeCollection tests Volume data collection
func TestCalculateDonchian(t *testing.T) {
	// Create test klines with known high/low values
	klines := []market.Kline{
		{High: 100, Low: 90},
		{High: 105, Low: 88},
		{High: 102, Low: 92},
		{High: 108, Low: 85},
		{High: 103, Low: 91},
	}

	upper, lower := donchian(klines, 5)

	if upper != 108 {
		t.Errorf("Expected upper = 108, got %v", upper)
	}
	if lower != 85 {
		t.Errorf("Expected lower = 85, got %v", lower)
	}
}

func TestCalculateDonchian_PartialPeriod(t *testing.T) {
	klines := []market.Kline{
		{High: 100, Low: 90},
		{High: 105, Low: 88},
	}

	upper, lower := donchian(klines, 10)

	// Should use all available klines when period > len(klines)
	if upper != 105 {
		t.Errorf("Expected upper = 105, got %v", upper)
	}
	if lower != 88 {
		t.Errorf("Expected lower = 88, got %v", lower)
	}
}

func TestCalculateDonchian_InvalidPeriod(t *testing.T) {
	klines := []market.Kline{
		{High: 100, Low: 90},
	}

	// Zero period should return (0, 0)
	upper, lower := donchian(klines, 0)
	if upper != 0 || lower != 0 {
		t.Errorf("Expected (0, 0) for zero period, got (%v, %v)", upper, lower)
	}

	// Negative period should return (0, 0)
	upper, lower = donchian(klines, -1)
	if upper != 0 || lower != 0 {
		t.Errorf("Expected (0, 0) for negative period, got (%v, %v)", upper, lower)
	}
}

func TestCalculateBoxData(t *testing.T) {
	// Create synthetic kline data
	klines := make([]market.Kline, 500)
	for i := 0; i < 500; i++ {
		basePrice := 100.0
		klines[i] = market.Kline{
			High:  basePrice + float64(i%10),
			Low:   basePrice - float64(i%10),
			Close: basePrice,
		}
	}

	box := BoxFromKlines(klines, 100.0)

	if box.ShortUpper == 0 || box.ShortLower == 0 {
		t.Error("Short box should not be zero")
	}
	if box.MidUpper == 0 || box.MidLower == 0 {
		t.Error("Mid box should not be zero")
	}
	if box.LongUpper == 0 || box.LongLower == 0 {
		t.Error("Long box should not be zero")
	}
	if box.CurrentPrice != 100.0 {
		t.Errorf("Expected CurrentPrice = 100.0, got %v", box.CurrentPrice)
	}
}
