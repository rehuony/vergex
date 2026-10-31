package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// handleDecisions Decision log list
func (s *Server) handleDecisions(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	// Get all historical decision records (unlimited)
	records, err := s.store.Decision().GetLatestRecords(trader.ID, 10000)
	if err != nil {
		SafeInternalError(c, "Get decision log", err)
		return
	}

	c.JSON(http.StatusOK, records)
}

// handleLatestDecisions Latest decision logs (newest first, supports limit parameter)
func (s *Server) handleLatestDecisions(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	// Get limit from query parameter, default to 5
	limit := 5
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100 // Max 100 to prevent abuse
			}
		}
	}

	records, err := s.store.Decision().GetLatestRecords(trader.ID, limit)
	if err != nil {
		SafeInternalError(c, "Get decision log", err)
		return
	}

	// Reverse array to put newest first (for list display)
	// GetLatestRecords returns oldest to newest (for charts), here we need newest to oldest
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	c.JSON(http.StatusOK, records)
}

// handleDecisionSummaries lists a trader's decisions newest first, without the
// prompts and raw responses (see handleDecisionDetail). before pages to older ones.
func (s *Server) handleDecisionSummaries(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	limit := 50
	if parsed, err := strconv.Atoi(c.Query("limit")); err == nil && parsed > 0 {
		limit = parsed
		if limit > 100 {
			limit = 100
		}
	}
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)

	records, err := s.store.Decision().GetSummaries(trader.ID, limit, before)
	if err != nil {
		SafeInternalError(c, "Get decision summaries", err)
		return
	}
	c.JSON(http.StatusOK, records)
}

// handleDecisionDetail returns one full decision of the trader.
func (s *Server) handleDecisionDetail(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		SafeBadRequest(c, "Invalid decision ID")
		return
	}
	record, err := s.store.Decision().GetRecord(trader.ID, id)
	if err != nil {
		SafeInternalError(c, "Get decision", err)
		return
	}
	if record == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Decision not found"})
		return
	}
	c.JSON(http.StatusOK, record)
}

// equityRanges maps the chart's range choices to how far back they reach; "all"
// has no limit.
var equityRanges = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"all": 0,
}

// maxEquityPoints is well above what a chart can show across a phone or desktop.
const maxEquityPoints = 500

// handleEquity returns a trader's equity curve for the chosen range.
func (s *Server) handleEquity(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}
	span, ok := equityRanges[c.DefaultQuery("range", "7d")]
	if !ok {
		SafeBadRequest(c, "Invalid range")
		return
	}
	var since time.Time
	if span > 0 {
		since = time.Now().UTC().Add(-span)
	}
	snapshots, err := s.store.Equity().GetSeries(trader.ID, since, maxEquityPoints)
	if err != nil {
		SafeInternalError(c, "Get equity", err)
		return
	}
	points := make([]gin.H, len(snapshots))
	for i, p := range snapshots {
		points[i] = gin.H{"time": p.Timestamp.Unix(), "equity": p.TotalEquity, "unrealized_pnl": p.UnrealizedPnL}
	}
	c.JSON(http.StatusOK, gin.H{"points": points, "initial_balance": s.initialBalance(trader)})
}

// handleStatistics Statistics information
func (s *Server) handleStatistics(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	stats, err := s.store.Decision().GetStatistics(trader.ID)
	if err != nil {
		SafeInternalError(c, "Get statistics", err)
		return
	}

	c.JSON(http.StatusOK, stats)
}
