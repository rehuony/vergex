package main

import (
	"vergex/internal/agent"
	"vergex/internal/agent/pricing"
	"vergex/internal/logger"
)

// recordAgentCalls returns a callback that stores every call to a model in
// agent.calls, with a cost estimated from models.dev prices.
func recordAgentCalls(calls *agent.CallStore, prices *pricing.Service) func(agent.CallReport) {
	return func(report agent.CallReport) {
		call := agent.Call{
			Feature:            report.Feature,
			SourceID:           report.SourceID,
			ConversationID:     report.ConversationID,
			Model:              report.Model,
			Provider:           report.Provider,
			Host:               agent.BaseHost(report.BaseURL),
			PromptTokens:       report.PromptTokens,
			CachedPromptTokens: report.CachedPromptTokens,
			CompletionTokens:   report.CompletionTokens,
			DurationMs:         int(report.Duration.Milliseconds()),
			Status:             agent.CallOK,
			FinishReason:       report.FinishReason,
		}
		if report.Error != "" {
			call.Status = agent.CallFailed
			call.Error = report.Error
		}
		// A call that used no tokens costs nothing, priced or not.
		if report.TotalTokens > 0 {
			cost, priced := prices.Estimate(report.Provider, report.BaseURL, report.Model, pricing.Usage{
				PromptTokens:       report.PromptTokens,
				CachedPromptTokens: report.CachedPromptTokens,
				CompletionTokens:   report.CompletionTokens,
			})
			call.CostUSD, call.Unpriced = cost, !priced
		}
		if err := calls.Record(&call); err != nil {
			logger.Warnf("⚠️ Failed to record model call: %v", err)
		}
	}
}
