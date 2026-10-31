package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vergex/internal/forecast"

	"github.com/gin-gonic/gin"
)

const maxBacktestSymbols = 300

// requireForecast answers for the handlers below when forecasting is not running.
func (s *Server) requireForecast(c *gin.Context) bool {
	if s.forecast == nil || s.marketData == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "forecast is unavailable"})
		return false
	}
	return true
}

func forecastInterval(c *gin.Context) (string, bool) {
	interval := c.DefaultQuery("interval", "1h")
	for _, known := range forecast.Intervals {
		if known == interval {
			return interval, true
		}
	}
	SafeBadRequest(c, "Unsupported interval")
	return "", false
}

// forecastPrices returns the latest prices. A failure only blanks the prices: the
// forecasts themselves come from local data.
func (s *Server) forecastPrices(ctx context.Context) map[string]float64 {
	prices, err := s.marketData.Prices(ctx)
	if err != nil {
		return nil
	}
	return prices
}

func (s *Server) handleForecastSignals(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	interval, ok := forecastInterval(c)
	if !ok {
		return
	}
	version, err := s.forecast.Store.ActiveVersion(time.Now())
	if err != nil {
		SafeInternalError(c, "Read the forecast version", err)
		return
	}
	signals, err := s.forecast.Signals(version, interval, s.forecastPrices(c.Request.Context()))
	if err != nil {
		SafeInternalError(c, "Read forecast signals", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"version_id": version.ID, "interval": interval, "signals": signals})
}

func (s *Server) handleForecastDetail(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	interval, ok := forecastInterval(c)
	if !ok {
		return
	}
	symbols, ok := cleanSymbols([]string{c.Param("symbol")})
	if !ok {
		SafeBadRequest(c, "Unknown symbol")
		return
	}
	version, err := s.forecast.Store.ActiveVersion(time.Now())
	if err != nil {
		SafeInternalError(c, "Read the forecast version", err)
		return
	}
	detail, err := s.forecast.Detail(version, symbols[0], interval, s.forecastPrices(c.Request.Context()))
	if err != nil {
		SafeInternalError(c, "Read the forecast", err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (s *Server) handleForecastVersions(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	settings, err := s.forecast.Store.LoadSettings(time.Now())
	if err != nil {
		SafeInternalError(c, "Read forecast settings", err)
		return
	}
	versions, err := s.forecast.Store.Versions()
	if err != nil {
		SafeInternalError(c, "Read forecast versions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"active_version_id": settings.ActiveVersionID, "versions": versions, "default_config": forecast.DefaultConfig()})
}

func (s *Server) handleCreateForecastVersion(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	var req struct {
		Config forecast.Config `json:"config" binding:"required"`
		Note   string          `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	version, err := s.forecast.Store.CreateVersion(req.Config, strings.TrimSpace(req.Note), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, version)
}

func (s *Server) handleActivateForecastVersion(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		SafeBadRequest(c, "Invalid version")
		return
	}
	if err := s.forecast.Store.Activate(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "strategy version not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"active_version_id": id})
}

func (s *Server) handleGetForecastSettings(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	settings, err := s.forecast.Store.LoadSettings(time.Now())
	if err != nil {
		SafeInternalError(c, "Read forecast settings", err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (s *Server) handleUpdateForecastSettings(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	var req struct {
		AlertThreshold int `json:"alert_threshold"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if req.AlertThreshold < 51 || req.AlertThreshold > 99 {
		SafeBadRequest(c, "Alert threshold must be between 51 and 99")
		return
	}
	if _, err := s.forecast.Store.LoadSettings(time.Now()); err != nil {
		SafeInternalError(c, "Read forecast settings", err)
		return
	}
	if err := s.forecast.Store.SetAlertThreshold(req.AlertThreshold); err != nil {
		SafeInternalError(c, "Save forecast settings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"alert_threshold": req.AlertThreshold})
}

func (s *Server) handleForecastCoverage(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	symbols, ok := cleanSymbols(strings.Split(c.Query("symbols"), ","))
	if !ok {
		SafeBadRequest(c, "Unknown symbol")
		return
	}
	coverage, err := s.forecast.Coverage(symbols, strings.Split(c.Query("intervals"), ","))
	if err != nil {
		SafeInternalError(c, "Read local history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"coverage": coverage})
}

func (s *Server) handleForecastBacktests(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	runs, err := s.forecast.Store.Backtests(30)
	if err != nil {
		SafeInternalError(c, "Read backtests", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"backtests": runs})
}

// handleCreateForecastBacktest starts a run that reads only local history; it
// keeps going after the request ends, and asks the exchange for nothing.
func (s *Server) handleCreateForecastBacktest(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	var req struct {
		VersionID int64 `json:"version_id"`
		forecast.BacktestOptions
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	symbols, ok := cleanSymbols(req.Symbols)
	if !ok || len(symbols) == 0 || len(symbols) > maxBacktestSymbols {
		SafeBadRequest(c, "Choose up to 300 USDT perpetual symbols")
		return
	}
	if len(req.Intervals) == 0 || req.To <= req.From {
		SafeBadRequest(c, "Choose intervals and a date range")
		return
	}
	for _, interval := range req.Intervals {
		known := false
		for _, candidate := range forecast.Intervals {
			known = known || candidate == interval
		}
		if !known {
			SafeBadRequest(c, "Unsupported interval")
			return
		}
	}
	req.Symbols = symbols
	run, err := s.forecast.StartBacktest(context.Background(), req.VersionID, req.BacktestOptions)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}
