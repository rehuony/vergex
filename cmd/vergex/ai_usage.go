package main

import (
	"vergex/internal/llm"
	"vergex/internal/logger"
	"vergex/internal/pricing"
	"vergex/internal/store"
)

// recordAIUsage returns a token usage callback that stores every AI call in
// the local ai_charges table with a cost estimated from models.dev prices.
func recordAIUsage(st *store.Store, prices *pricing.Service) func(llm.TokenUsage) {
	return func(usage llm.TokenUsage) {
		cost, priced := prices.Estimate(usage.Provider, usage.BaseURL, usage.Model, pricing.Usage{
			PromptTokens:       usage.PromptTokens,
			CachedPromptTokens: usage.CachedPromptTokens,
			CompletionTokens:   usage.CompletionTokens,
		})
		err := st.AICharge().Record(&store.AICharge{
			TraderID:         usage.Owner,
			Model:            usage.Model,
			Provider:         usage.Provider,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			CostUSD:          cost,
			Unpriced:         !priced,
		})
		if err != nil {
			logger.Warnf("⚠️ Failed to record AI usage: %v", err)
		}
	}
}
