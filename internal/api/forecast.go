package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"vergex/internal/forecast"
	"vergex/internal/market/feed"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
func (s *Server) forecastPrices(ctx context.Context) map[string]feed.Quote {
	prices, err := s.marketData.Prices(ctx)
	if err != nil {
		return nil
	}
	return prices
}

func (s *Server) handleForecastCoins(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	interval, ok := forecastInterval(c)
	if !ok {
		return
	}
	strategy, err := s.forecast.Store.ActiveStrategy(time.Now())
	if errors.Is(err, forecast.ErrNoStrategyInUse) {
		c.JSON(http.StatusOK, gin.H{"strategy_id": "", "interval": interval, "coins": []forecast.Coin{}})
		return
	}
	if err != nil {
		SafeInternalError(c, "Read the forecast strategy", err)
		return
	}
	coins, err := s.forecast.Coins(strategy, interval, s.forecastPrices(c.Request.Context()))
	if err != nil {
		SafeInternalError(c, "Read forecast signals", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"strategy_id": strategy.ID, "interval": interval, "coins": coins})
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
	strategy, err := s.forecast.Store.ActiveStrategy(time.Now())
	if errors.Is(err, forecast.ErrNoStrategyInUse) {
		refuseForecast(c, http.StatusConflict, err)
		return
	}
	if err != nil {
		SafeInternalError(c, "Read the forecast strategy", err)
		return
	}
	options := forecast.DetailOptions{Indicators: c.Query("indicators") == "1"}
	detail, err := s.forecast.Detail(strategy, symbols[0], interval, s.forecastPrices(c.Request.Context()), options)
	if err != nil {
		SafeInternalError(c, "Read the forecast", err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// handleForecastOverlay is what the Futures chart reads: the coin's forecast under the
// strategy in use, and nothing else of Forecast.
func (s *Server) handleForecastOverlay(c *gin.Context) {
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
	strategy, err := s.forecast.Store.ActiveStrategy(time.Now())
	if errors.Is(err, forecast.ErrNoStrategyInUse) {
		c.JSON(http.StatusOK, forecast.Overlay{Symbol: symbols[0], Interval: interval, Pending: forecast.PendingNoStrategy})
		return
	}
	if err != nil {
		SafeInternalError(c, "Read the forecast strategy", err)
		return
	}
	overlay, err := s.forecast.OverlayFor(strategy, symbols[0], interval)
	if err != nil {
		SafeInternalError(c, "Read the forecast", err)
		return
	}
	c.JSON(http.StatusOK, overlay)
}

// forecastRefusals names the web's wording for what the forecast store refuses.
var forecastRefusals = map[error]string{
	forecast.ErrStrategyInUse:     "inUse",
	forecast.ErrLastStrategy:      "last",
	forecast.ErrStrategyBusy:      "busy",
	forecast.ErrStrategyAbsent:    "absent",
	forecast.ErrStrategyRetired:   "retired",
	forecast.ErrStrategyName:      "name",
	forecast.ErrStrategyNameTaken: "nameTaken",
	forecast.ErrBacktestRunning:   "backtestRunning",
	forecast.ErrBacktestName:      "backtestName",
	forecast.ErrBacktestAbsent:    "backtestAbsent",
}

func refuseForecast(c *gin.Context, status int, err error) {
	body := gin.H{"error": err.Error()}
	for refusal, key := range forecastRefusals {
		if errors.Is(err, refusal) {
			body["error_key"] = "forecast.refusal." + key
		}
	}
	c.JSON(status, body)
}

func (s *Server) handleForecastStrategies(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	settings, err := s.forecast.Store.LoadSettings(time.Now())
	if err != nil {
		SafeInternalError(c, "Read forecast settings", err)
		return
	}
	strategies, err := s.forecast.Store.Strategies()
	if err != nil {
		SafeInternalError(c, "Read forecast strategies", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"active_strategy_id": settings.ActiveStrategyID, "strategies": strategies, "horizon_limits": forecast.HorizonLimits, "default_config": forecast.DefaultConfig()})
}

// handleForecastPerformance is how a strategy has done live.
func (s *Server) handleForecastPerformance(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id := c.Param("id")
	if _, err := s.forecast.Store.StrategyByID(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": forecast.ErrStrategyAbsent.Error()})
		return
	}
	performance, err := s.forecast.Store.Performance(id)
	if err != nil {
		SafeInternalError(c, "Read the strategy's performance", err)
		return
	}
	c.JSON(http.StatusOK, performance)
}

func (s *Server) handleCreateForecastStrategy(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	var req struct {
		Config forecast.Config `json:"config" binding:"required"`
		Name   string          `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	strategy, err := s.forecast.Store.CreateStrategy(req.Config, req.Name, time.Now())
	if err != nil {
		refuseForecast(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, strategy)
}

func (s *Server) handleRenameForecastStrategy(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	switch err := s.forecast.Store.RenameStrategy(c.Param("id"), req.Name); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"renamed": c.Param("id")})
	case errors.Is(err, forecast.ErrStrategyAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrStrategyName):
		refuseForecast(c, http.StatusBadRequest, err)
	case errors.Is(err, forecast.ErrStrategyNameTaken):
		refuseForecast(c, http.StatusConflict, err)
	default:
		SafeInternalError(c, "Rename the forecast strategy", err)
	}
}

func (s *Server) handleActivateForecastStrategy(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id := c.Param("id")
	switch err := s.forecast.Store.Activate(id); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"active_strategy_id": id})
	case errors.Is(err, forecast.ErrStrategyAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrStrategyRetired):
		refuseForecast(c, http.StatusConflict, err)
	default:
		SafeInternalError(c, "Use the forecast strategy", err)
	}
}

func (s *Server) handleDeactivateForecastStrategy(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	switch err := s.forecast.Store.Deactivate(c.Param("id")); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"active_strategy_id": ""})
	case errors.Is(err, forecast.ErrStrategyAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	default:
		SafeInternalError(c, "Stop the forecast strategy", err)
	}
}

func (s *Server) handleReviveForecastStrategy(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id := c.Param("id")
	switch err := s.forecast.Store.Revive(id); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"active_strategy_id": id})
	case errors.Is(err, forecast.ErrStrategyAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrStrategyBusy):
		refuseForecast(c, http.StatusConflict, err)
	default:
		SafeInternalError(c, "Revive the forecast strategy", err)
	}
}

func (s *Server) handleDeleteForecastStrategy(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id := c.Param("id")
	switch err := s.forecast.Store.DeleteStrategy(id, time.Now()); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"deleted": id})
	case errors.Is(err, forecast.ErrStrategyAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrStrategyInUse), errors.Is(err, forecast.ErrLastStrategy), errors.Is(err, forecast.ErrStrategyBusy):
		refuseForecast(c, http.StatusConflict, err)
	default:
		SafeInternalError(c, "Delete the forecast strategy", err)
	}
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
		// AlertIntervals stays as it is when left out, and an empty list means none.
		AlertIntervals []string `json:"alert_intervals"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if req.AlertThreshold < 51 || req.AlertThreshold > 99 {
		SafeBadRequest(c, "Alert threshold must be between 51 and 99")
		return
	}
	for _, interval := range req.AlertIntervals {
		if !slices.Contains(forecast.Intervals, interval) {
			SafeBadRequest(c, "Unsupported interval")
			return
		}
	}
	current, err := s.forecast.Store.LoadSettings(time.Now())
	if err != nil {
		SafeInternalError(c, "Read forecast settings", err)
		return
	}
	if req.AlertIntervals == nil {
		req.AlertIntervals = current.AlertIntervals
	}
	if err := s.forecast.Store.SetAlerts(req.AlertThreshold, req.AlertIntervals); err != nil {
		SafeInternalError(c, "Save forecast settings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"alert_threshold": req.AlertThreshold, "alert_intervals": req.AlertIntervals})
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
	runs, err := s.forecast.Store.Backtests(100)
	if err != nil {
		SafeInternalError(c, "Read backtests", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"backtests": runs})
}

// handleForecastBacktest returns one run with its whole report.
func (s *Server) handleForecastBacktest(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		SafeBadRequest(c, "Invalid backtest")
		return
	}
	run, err := s.forecast.Store.BacktestByID(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "backtest not found"})
		return
	}
	if err != nil {
		SafeInternalError(c, "Read the backtest", err)
		return
	}
	c.JSON(http.StatusOK, run)
}

func (s *Server) handleRenameForecastBacktest(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		SafeBadRequest(c, "Invalid backtest")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	switch err := s.forecast.Store.RenameBacktest(id, req.Name); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"renamed": id})
	case errors.Is(err, forecast.ErrBacktestAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrBacktestName):
		refuseForecast(c, http.StatusBadRequest, err)
	default:
		SafeInternalError(c, "Rename the backtest", err)
	}
}

func (s *Server) handleDeleteForecastBacktest(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		SafeBadRequest(c, "Invalid backtest")
		return
	}
	switch err := s.forecast.Store.DeleteBacktest(id); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"deleted": id})
	case errors.Is(err, forecast.ErrBacktestAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrBacktestRunning):
		refuseForecast(c, http.StatusConflict, err)
	default:
		SafeInternalError(c, "Delete the backtest", err)
	}
}

func (s *Server) handleCancelForecastBacktest(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		SafeBadRequest(c, "Invalid backtest")
		return
	}
	if !s.forecast.CancelBacktest(id) {
		c.JSON(http.StatusConflict, gin.H{"error": "that backtest is not running"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"canceled": id})
}

// handleCreateForecastBacktest starts a run that reads only local history; it
// keeps going after the request ends, and asks the exchange for nothing.
func (s *Server) handleCreateForecastBacktest(c *gin.Context) {
	if !s.requireForecast(c) {
		return
	}
	var req struct {
		StrategyID string `json:"strategy_id"`
		Name       string `json:"name"`
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
	run, err := s.forecast.StartBacktest(context.Background(), req.StrategyID, req.Name, req.BacktestOptions)
	switch {
	case err == nil:
		c.JSON(http.StatusAccepted, run)
	case errors.Is(err, forecast.ErrStrategyAbsent):
		refuseForecast(c, http.StatusNotFound, err)
	case errors.Is(err, forecast.ErrStrategyRetired), errors.Is(err, forecast.ErrBacktestRunning):
		refuseForecast(c, http.StatusConflict, err)
	case errors.Is(err, forecast.ErrBacktestName):
		refuseForecast(c, http.StatusBadRequest, err)
	default:
		SafeInternalError(c, "Start the backtest", err)
	}
}
