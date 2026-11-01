package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"vergex/internal/market/feed"

	"github.com/gin-gonic/gin"
)

const (
	maxWatchlistSymbols = 100
	maxTopSymbols       = 400
	maxBackfillMonths   = 120
)

var marketSymbol = regexp.MustCompile(`^[A-Z0-9]+USDT$`)

// requireMarketData answers for the handlers below when the market database
// could not be opened.
func (s *Server) requireMarketData(c *gin.Context) bool {
	if s.marketData == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "market data is unavailable"})
		return false
	}
	return true
}

func cleanSymbols(raw []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, symbol := range raw {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if !marketSymbol.MatchString(symbol) {
			return nil, false
		}
		if !seen[symbol] {
			seen[symbol] = true
			out = append(out, symbol)
		}
	}
	return out, true
}

func (s *Server) handleMarketDataStatus(c *gin.Context) {
	if !s.requireMarketData(c) {
		return
	}
	status, err := s.marketData.Status()
	if err != nil {
		SafeInternalError(c, "Read market data status", err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) handleGetMarketDataSettings(c *gin.Context) {
	if !s.requireMarketData(c) {
		return
	}
	settings, err := s.marketData.Store.LoadSettings()
	if err != nil {
		SafeInternalError(c, "Read market data settings", err)
		return
	}
	watchlist := settings.Watchlist
	if watchlist == nil {
		watchlist = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"enabled": settings.Enabled, "watchlist": watchlist, "top_n": settings.TopN})
}

func (s *Server) handleUpdateMarketDataSettings(c *gin.Context) {
	if !s.requireMarketData(c) {
		return
	}
	var req struct {
		Enabled   bool     `json:"enabled"`
		Watchlist []string `json:"watchlist"`
		TopN      int      `json:"top_n"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	watchlist, ok := cleanSymbols(req.Watchlist)
	if !ok || len(watchlist) > maxWatchlistSymbols {
		SafeBadRequest(c, "Watchlist must hold up to 100 USDT perpetual symbols")
		return
	}
	if req.TopN < 0 || req.TopN > maxTopSymbols {
		SafeBadRequest(c, "Top coins must be between 0 and 400")
		return
	}
	if err := s.marketData.SaveSettings(req.Enabled, watchlist, req.TopN); err != nil {
		SafeInternalError(c, "Save market data settings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": req.Enabled, "watchlist": watchlist, "top_n": req.TopN})
}

// handleMarketDataBackfill starts loading history in the background. It keeps
// going after the request ends; progress shows in the status.
func (s *Server) handleMarketDataBackfill(c *gin.Context) {
	if !s.requireMarketData(c) {
		return
	}
	var req feed.BackfillOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	symbols, ok := cleanSymbols(req.Symbols)
	if !ok || len(symbols) == 0 {
		SafeBadRequest(c, "Choose at least one USDT perpetual symbol")
		return
	}
	for _, interval := range req.Intervals {
		if !feed.IsHistoryInterval(interval) {
			SafeBadRequest(c, "Unsupported interval")
			return
		}
	}
	if len(req.Intervals) == 0 || req.Months <= 0 || req.Months > maxBackfillMonths {
		SafeBadRequest(c, "Choose intervals and up to 120 months")
		return
	}
	req.Symbols = symbols
	if err := s.marketData.StartBackfill(context.Background(), req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true})
}
