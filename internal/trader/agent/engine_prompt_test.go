package agent

import (
	"strings"
	"testing"

	"vergex/internal/trader/kind"
)

func TestBuildSystemPromptFallsBackToEnglishWhenSectionsAreChinese(t *testing.T) {
	cfg := kind.DefaultAgent()
	cfg.CoinSource.SourceType = "static"
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT", "ETHUSDT"}
	cfg.PromptSections.RoleDefinition = "# 你是一个中文系统提示"
	cfg.PromptSections.TradingFrequency = "# 高频交易\n每分钟交易一次。"
	cfg.PromptSections.EntryStandards = "# 开仓\n随意开仓。"
	cfg.PromptSections.DecisionProcess = "# 决策\n直接输出。"
	cfg.CustomPrompt = "中文偏好不应进入系统提示。"

	engine := NewEngine(cfg)
	prompt := engine.BuildSystemPrompt(30, "balanced")

	required := []string{
		"Data Dictionary & Trading Rules",
		"You are a professional cryptocurrency perpetual futures trading agent",
		"Trading Frequency Awareness",
		"Entry Standards",
		"Decision Process",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("English fallback prompt missing %q:\n%s", phrase, prompt)
		}
	}
	if containsCJK(prompt) {
		t.Fatalf("system prompt must be English-only, got CJK text:\n%s", prompt)
	}
}

func containsCJK(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
