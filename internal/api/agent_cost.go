package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleGetAgentCosts returns a trader's calls to its model
func (s *Server) handleGetAgentCosts(c *gin.Context) {
	traderID := c.Query("trader_id")
	period := c.DefaultQuery("period", "today")

	if traderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trader_id is required"})
		return
	}
	// The trader must belong to the caller; never read another account's costs.
	trader, err := s.stores.Traders.GetByID(traderID)
	if err != nil || trader.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "trader not found for this account"})
		return
	}

	calls, total, err := s.stores.Calls.ForTrader(traderID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var promptTokens, completionTokens, unpriced int
	for _, call := range calls {
		promptTokens += call.PromptTokens
		completionTokens += call.CompletionTokens
		if call.Unpriced {
			unpriced++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"calls":             calls,
		"total":             total,
		"count":             len(calls),
		"unpriced_count":    unpriced,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
	})
}

// handleGetAgentStorage says how much disk what the models were sent and said takes up.
func (s *Server) handleGetAgentStorage(c *gin.Context) {
	size, err := s.stores.Conversations.MessageStorage()
	if err != nil {
		SafeInternalError(c, "Read agent storage", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"bytes": size})
}

// handleGetAgentCostsSummary returns agent cost summary across all traders, or for
// one with ?trader_id=.
func (s *Server) handleGetAgentCostsSummary(c *gin.Context) {
	period := c.DefaultQuery("period", "today")
	traderID := c.Query("trader_id")
	if traderID != "" {
		trader, err := s.stores.Traders.GetByID(traderID)
		if err != nil || trader.UserID != c.GetString("user_id") {
			c.JSON(http.StatusNotFound, gin.H{"error": "trader not found for this account"})
			return
		}
	}

	summary, err := s.stores.Calls.GetSummary(traderID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, summary)
}
