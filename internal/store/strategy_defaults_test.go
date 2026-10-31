package store

import "testing"

func TestDefaultStrategyIsStaticCryptoPool(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		cfg := GetDefaultStrategyConfig(lang)
		if cfg.CoinSource.SourceType != "static" || len(cfg.CoinSource.StaticCoins) == 0 {
			t.Fatalf("%s: default coin source = %+v, want a static coin list", lang, cfg.CoinSource)
		}
		if !cfg.Indicators.EnableRawKlines {
			t.Fatalf("%s: raw klines must stay enabled", lang)
		}
		if cfg.PromptSections.RoleDefinition == "" {
			t.Fatalf("%s: default strategy must ship built-in prompt sections", lang)
		}
	}
}

func TestDefaultStrategySurvivesClampAndNormalize(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	want := append([]string(nil), cfg.CoinSource.StaticCoins...)
	cfg.ClampLimits()
	if cfg.CoinSource.SourceType != "static" || len(cfg.CoinSource.StaticCoins) != len(want) {
		t.Fatalf("clamp changed the default coin source: %+v", cfg.CoinSource)
	}
}
