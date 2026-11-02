package kind

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeRefusesWrongChoices(t *testing.T) {
	agent := func(patch map[string]any) error {
		_, _, err := Merge(Agent, DefaultAgent(), patch)
		return err
	}
	cases := map[string]map[string]any{
		"label instead of a source": {"coin_source": map[string]any{"source_type": "OI Top"}},
		"unknown window":            {"coin_source": map[string]any{"oi_window": "2h"}},
		"unknown timeframe":         {"indicators": map[string]any{"klines": map[string]any{"primary_timeframe": "1 hour"}}},
		"unknown selected":          {"indicators": map[string]any{"klines": map[string]any{"selected_timeframes": []any{"5m", "7m"}}}},
	}
	for name, patch := range cases {
		if err := agent(patch); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, _, err := Merge(Grid, DefaultGrid(), map[string]any{"distribution": "curvy"}); err == nil {
		t.Error("unknown distribution: want an error")
	}
	if _, _, err := Merge(Grid, DefaultGrid(), map[string]any{"symbol": " "}); err == nil {
		t.Error("blank symbol: want an error")
	}
	if _, err := Decode("signal", []byte(`{}`)); err == nil {
		t.Error("unknown kind: want an error")
	}
}

func TestValidateBringsNumbersWithinBounds(t *testing.T) {
	config, adjusted, err := Merge(Agent, DefaultAgent(), map[string]any{"risk_control": map[string]any{
		"max_positions": 99, "btc_eth_max_leverage": 0, "max_margin_usage": 3.0, "min_confidence": 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	risk := config.(*AgentConfig).RiskControl
	if risk.MaxPositions != MaxPositions || risk.BTCETHMaxLeverage != MinLeverage || risk.MaxMarginUsage != MaxMarginUsage || risk.MinConfidence != MinConfidence {
		t.Fatalf("not clamped: %+v", risk)
	}
	if risk.AltcoinMaxLeverage != DefaultAgent().RiskControl.AltcoinMaxLeverage {
		t.Fatalf("a field the patch left out changed: %+v", risk)
	}
	if len(adjusted) != 4 {
		t.Fatalf("adjusted = %v", adjusted)
	}

	grid, _, err := Merge(Grid, DefaultGrid(), map[string]any{"grid_count": 500, "leverage": 500})
	if err != nil {
		t.Fatal(err)
	}
	if c := grid.(*GridConfig); c.GridCount != MaxGridCount || c.Leverage != MaxGridLeverage {
		t.Fatalf("not clamped: %+v", c)
	}
}

func TestPresetsAreValidAndDefaultFirst(t *testing.T) {
	for _, name := range []string{Agent, Grid} {
		presets, err := Presets(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, preset := range presets {
			before, _ := json.Marshal(preset.Config)
			if err := preset.Config.Validate(); err != nil {
				t.Errorf("%s/%s: %v", name, preset.ID, err)
			}
			if after, _ := json.Marshal(preset.Config); string(before) != string(after) {
				t.Errorf("%s/%s changes when validated:\n%s\n%s", name, preset.ID, before, after)
			}
		}
	}
	first, err := FromPreset(Agent, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.(*AgentConfig).RiskControl.MaxPositions != DefaultAgent().RiskControl.MaxPositions {
		t.Fatal("the default preset is not the default config")
	}
	if _, err := FromPreset(Agent, "reckless"); err == nil {
		t.Fatal("unknown preset: want an error")
	}
}

func TestTouchesRisk(t *testing.T) {
	cases := []struct {
		kind  string
		patch map[string]any
		want  bool
	}{
		{Agent, map[string]any{"risk_control": map[string]any{"max_positions": 2}}, true},
		{Agent, map[string]any{"coin_source": map[string]any{"static_coins": []any{"BTCUSDT"}}}, false},
		{Agent, map[string]any{"custom_prompt": "be calm"}, false},
		{Grid, map[string]any{"leverage": 2}, true},
		{Grid, map[string]any{"use_maker_only": false}, false},
	}
	for _, c := range cases {
		if got := TouchesRisk(c.kind, c.patch); got != c.want {
			t.Errorf("%s %v: got %v", c.kind, c.patch, got)
		}
	}
}

func TestLimitsAreSmallAndNamed(t *testing.T) {
	raw, err := json.Marshal(DefaultAgent().Limits())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"max_positions", "btc_eth_max_position_value_ratio", "max_margin_usage", "min_position_size", "altcoin_max_leverage"} {
		if !strings.Contains(string(raw), name) {
			t.Errorf("%s missing from %s", name, raw)
		}
	}
	raw, err = json.Marshal(DefaultGrid().Limits())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"total_investment", "leverage", "upper_price", "stop_loss_pct"} {
		if !strings.Contains(string(raw), name) {
			t.Errorf("%s missing from %s", name, raw)
		}
	}
}

func TestEstimateTokensCountsWhatItReads(t *testing.T) {
	small := DefaultAgent()
	large := DefaultAgent()
	large.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "ADAUSDT"}
	large.Indicators.EnableMACD = true
	estimate := small.EstimateTokens()
	if estimate.Total <= 0 || len(estimate.ModelLimits) != len(ModelContextLimits) {
		t.Fatalf("estimate: %+v", estimate)
	}
	if large.EstimateTokens().Total <= estimate.Total {
		t.Fatal("more coins and indicators should cost more")
	}
}

func TestLossLimitsStayInRangeAndZeroIsOff(t *testing.T) {
	for _, tt := range []struct{ in, want float64 }{{0, 0}, {-3, 0}, {0.1, MinLossLimitPct}, {5, 5}, {400, MaxLossLimitPct}} {
		config := DefaultAgent()
		config.RiskControl.MaxDailyLossPct, config.RiskControl.MaxDrawdownPct = tt.in, tt.in
		if err := config.Validate(); err != nil {
			t.Fatal(err)
		}
		if config.RiskControl.MaxDailyLossPct != tt.want || config.RiskControl.MaxDrawdownPct != tt.want {
			t.Fatalf("%v became %v and %v", tt.in, config.RiskControl.MaxDailyLossPct, config.RiskControl.MaxDrawdownPct)
		}
	}
	missing, err := Decode(Agent, []byte(`{"coin_source":{"source_type":"static","static_coins":["BTCUSDT"]},"risk_control":{"max_positions":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	if risk := missing.(*AgentConfig).RiskControl; risk.MaxDailyLossPct != 0 || risk.MaxDrawdownPct != 0 {
		t.Fatalf("a config without them: %+v", risk)
	}
}
