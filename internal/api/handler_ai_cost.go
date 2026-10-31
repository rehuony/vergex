package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleGetAICosts returns AI charges for a specific trader
func (s *Server) handleGetAICosts(c *gin.Context) {
	traderID := c.Query("trader_id")
	period := c.DefaultQuery("period", "today")

	if traderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trader_id is required"})
		return
	}
	// The trader must belong to the caller; never read another account's costs.
	trader, err := s.store.Trader().GetByID(traderID)
	if err != nil || trader.UserID != c.GetString("user_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "trader not found for this account"})
		return
	}

	charges, total, err := s.store.AICharge().GetCharges(traderID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var promptTokens, completionTokens, unpriced int
	for _, charge := range charges {
		promptTokens += charge.PromptTokens
		completionTokens += charge.CompletionTokens
		if charge.Unpriced {
			unpriced++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"charges":           charges,
		"total":             total,
		"count":             len(charges),
		"unpriced_count":    unpriced,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
	})
}

// handleGetAICostsSummary returns AI cost summary across all traders, or for
// one with ?trader_id=.
func (s *Server) handleGetAICostsSummary(c *gin.Context) {
	period := c.DefaultQuery("period", "today")
	traderID := c.Query("trader_id")
	if traderID != "" {
		trader, err := s.store.Trader().GetByID(traderID)
		if err != nil || trader.UserID != c.GetString("user_id") {
			c.JSON(http.StatusNotFound, gin.H{"error": "trader not found for this account"})
			return
		}
	}

	summary, err := s.store.AICharge().GetSummary(traderID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, summary)
}
