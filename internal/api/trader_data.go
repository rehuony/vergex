package api

import (
	"net/http"
	"strconv"
	"vergex/internal/account"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"

	"github.com/gin-gonic/gin"
)

// handleTraderList Trader list
func (s *Server) handleTraderList(c *gin.Context) {
	userID := c.GetString("user_id")
	traders, err := s.stores.Traders.List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get trader list", err)
		return
	}

	result := make([]map[string]interface{}, 0, len(traders))
	for _, trader := range traders {
		// Get real-time running status
		isRunning := trader.Running
		if at, err := s.traderManager.GetTrader(trader.ID); err == nil {
			status := at.GetStatus()
			if running, ok := status["running"].(bool); ok {
				isRunning = running
			}
		}

		// The frontend needs the model's ID to verify it exists (consistent with handleGetTraderConfig)
		result = append(result, map[string]interface{}{
			"trader_id":             trader.ID,
			"trader_name":           trader.Name,
			"agent_model":           trader.AgentModelID, // Use complete ID
			"account_id":            trader.AccountID,
			"running":               isRunning,
			"initial_balance":       trader.InitialBalance,
			"kind":                  trader.Kind,
			"scan_interval_minutes": trader.ScanIntervalMinutes,
		})
	}

	c.JSON(http.StatusOK, result)
}

// handleGetTraderConfig Get trader detailed configuration
func (s *Server) handleGetTraderConfig(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	if traderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Trader ID cannot be empty"})
		return
	}

	fullCfg, err := s.stores.Traders.GetFullConfig(userID, traderID)
	if err != nil {
		SafeNotFound(c, "Trader config")
		return
	}
	traderConfig := fullCfg.Trader

	// Get real-time running status
	isRunning := traderConfig.Running
	if at, err := s.traderManager.GetTrader(traderID); err == nil {
		status := at.GetStatus()
		if running, ok := status["running"].(bool); ok {
			isRunning = running
		}
	}

	// Return complete model ID without conversion, consistent with frontend model list
	agentModelID := traderConfig.AgentModelID

	result := map[string]interface{}{
		"trader_id":             traderConfig.ID,
		"trader_name":           traderConfig.Name,
		"agent_model":           agentModelID,
		"account_id":            traderConfig.AccountID,
		"kind":                  traderConfig.Kind,
		"config":                traderConfig.Config,
		"initial_balance":       traderConfig.InitialBalance,
		"scan_interval_minutes": traderConfig.ScanIntervalMinutes,
		"is_cross_margin":       traderConfig.IsCrossMargin(),
		"running":               isRunning,
	}

	c.JSON(http.StatusOK, result)
}

// handleStatus A trader's running status
func (s *Server) handleStatus(c *gin.Context) {
	traderID, err := s.loadedTraderID(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	status := trader.GetStatus()
	c.JSON(http.StatusOK, status)
}

// handleAccount A trader's balance and P/L
func (s *Server) handleAccount(c *gin.Context) {
	traderID, err := s.loadedTraderID(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	logger.Infof("📊 Received account info request [%s]", trader.GetName())
	account, err := trader.GetAccountInfo()
	if err != nil {
		SafeInternalError(c, "Get account info", err)
		return
	}

	logger.Infof("✓ Returning account info [%s]: equity=%.2f, available=%.2f",
		trader.GetName(),
		account["total_equity"],
		account["available_balance"])
	c.JSON(http.StatusOK, account)
}

// handlePositions Positions open on a trader's account
func (s *Server) handlePositions(c *gin.Context) {
	reader, err := s.accountReadClient(c.GetString("user_id"), c.Param("id"))
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	positions, err := reader.GetPositions()
	if err != nil {
		positionsUnreadable(c, err)
		return
	}
	// A trader runs on a whole account, so say which of these are not its own.
	marked := make([]map[string]interface{}, 0, len(positions))
	for _, position := range positions {
		row := make(map[string]interface{}, len(position)+1)
		for key, value := range position {
			row[key] = value
		}
		row["foreign"] = !s.traderHolds(c.Param("id"), position)
		marked = append(marked, row)
	}
	c.JSON(http.StatusOK, marked)
}

// handleAccountPositions Positions open on an account
func (s *Server) handleAccountPositions(c *gin.Context) {
	reader, err := s.accountReadClient(c.GetString("user_id"), "", c.Param("id"))
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	s.writePositions(c, reader)
}

func (s *Server) writePositions(c *gin.Context, reader accountReader) {
	positions, err := reader.GetPositions()
	if err != nil {
		positionsUnreadable(c, err)
		return
	}

	if positions == nil {
		positions = make([]map[string]interface{}, 0)
	}
	c.JSON(http.StatusOK, positions)
}

// positionsUnreadable says the exchange would not give the positions, and why. Dead keys
// and a venue that is down are not faults in this server.
func positionsUnreadable(c *gin.Context, err error) {
	logger.Warnf("Read positions: %v", err)
	status, code, message := classifyAccountProbeError(err)
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": message, "error_key": "accountError." + code, "status": status})
}

// handlePositionHistory Historical closed positions with statistics
func (s *Server) handlePositionHistory(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	// Get optional query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	// An older page is only more of what was already read.
	if before, err := strconv.ParseInt(c.Query("before"), 10, 64); err == nil && before > 0 {
		positions, err := s.stores.Positions.GetClosedPositionsBefore(trader.ID, before, limit)
		if err != nil {
			SafeBadRequest(c, "Invalid position ID")
			return
		}
		c.JSON(http.StatusOK, gin.H{"positions": positions})
		return
	}

	// Refresh from the exchange when the trader is loaded; the closed positions
	// already in the database are returned either way.
	if _, loadErr := s.traderManager.GetTrader(trader.ID); loadErr == nil {
		userID := c.GetString("user_id")
		if fullCfg, cfgErr := s.stores.Traders.GetFullConfig(userID, trader.ID); cfgErr == nil && fullCfg.Account != nil {
			if syncErr := s.syncAccount(fullCfg.Account.ID); syncErr != nil {
				logger.Infof("⚠️ Position history refresh sync skipped: %v", syncErr)
			}
		}
	}

	// Get closed positions
	positions, err := s.stores.Positions.GetClosedPositions(trader.ID, limit)
	if err != nil {
		SafeInternalError(c, "Get position history", err)
		return
	}

	// Get statistics
	stats, _ := s.stores.Positions.GetFullStats(trader.ID, s.initialBalance(trader))

	// Get symbol stats
	symbolStats, _ := s.stores.Positions.GetSymbolStats(trader.ID, 10)

	// Get direction stats
	directionStats, _ := s.stores.Positions.GetDirectionStats(trader.ID)

	c.JSON(http.StatusOK, gin.H{
		"positions":       positions,
		"stats":           stats,
		"symbol_stats":    symbolStats,
		"direction_stats": directionStats,
	})
}

// handleTrades Historical trades list
func (s *Server) handleTrades(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	// Get optional query parameters
	symbol := c.Query("symbol")
	limitStr := c.DefaultQuery("limit", "100")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	// Normalize symbol (add USDT suffix if not present)
	if symbol != "" {
		symbol = instrument.Normalize(symbol)
	}

	allTrades, err := s.stores.Positions.GetRecentTrades(trader.ID, limit)
	if err != nil {
		SafeInternalError(c, "Get trades", err)
		return
	}

	// Filter by symbol if specified
	if symbol != "" {
		var result []interface{}
		for _, trade := range allTrades {
			if trade.Symbol == symbol {
				result = append(result, trade)
			}
		}
		c.JSON(http.StatusOK, result)
		return
	}

	c.JSON(http.StatusOK, allTrades)
}

// handleOrders Order list (all orders including open, close, stop loss, take profit, etc.)
func (s *Server) handleOrders(c *gin.Context) {
	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	// Get optional query parameters
	symbol := c.Query("symbol")
	statusFilter := c.Query("status") // NEW, FILLED, CANCELED, etc.
	limitStr := c.DefaultQuery("limit", "100")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	// Normalize symbol (add USDT suffix if not present)
	if symbol != "" {
		symbol = instrument.Normalize(symbol)
	}

	// Get orders with filters applied at database level
	orders, err := s.stores.Orders.GetTraderOrdersFiltered(trader.ID, symbol, statusFilter, limit)
	if err != nil {
		SafeInternalError(c, "Get orders", err)
		return
	}

	c.JSON(http.StatusOK, orders)
}

// handleOrderFills Order fill details (all fills for a specific order)
func (s *Server) handleOrderFills(c *gin.Context) {
	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	trader, err := s.ownedTrader(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	// Get fills for this order, scoped to the trader (ownership boundary).
	fills, err := s.stores.Orders.GetOrderFills(trader.ID, orderID)
	if err != nil {
		SafeInternalError(c, "Get order fills", err)
		return
	}

	c.JSON(http.StatusOK, fills)
}

// handleOpenOrders Get a trader's open orders (pending SL/TP) from exchange
func (s *Server) handleOpenOrders(c *gin.Context) {
	reader, err := s.accountReadClient(c.GetString("user_id"), c.Param("id"))
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	s.writeOpenOrders(c, reader)
}

// handleAccountOpenOrders Get an account's open orders from exchange
func (s *Server) handleAccountOpenOrders(c *gin.Context) {
	reader, err := s.accountReadClient(c.GetString("user_id"), "", c.Param("id"))
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	s.writeOpenOrders(c, reader)
}

func (s *Server) writeOpenOrders(c *gin.Context, reader accountReader) {
	// Get symbol parameter (required for exchange query)
	symbol := c.Query("symbol")
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol parameter is required"})
		return
	}

	// Normalize symbol
	symbol = instrument.Normalize(symbol)

	// Get open orders from exchange
	openOrders, err := reader.GetOpenOrders(symbol)
	if err != nil {
		SafeInternalError(c, "Get open orders", err)
		return
	}

	if openOrders == nil {
		openOrders = make([]account.OpenOrder, 0)
	}
	c.JSON(http.StatusOK, openOrders)
}
