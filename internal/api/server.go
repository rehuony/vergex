package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"vergex/internal/account/syncer"
	"vergex/internal/dashboard"
	"vergex/internal/forecast"
	"vergex/internal/logger"
	"vergex/internal/market/feed"
	"vergex/internal/security/auth"
	"vergex/internal/security/crypto"
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Server HTTP API server
type Server struct {
	futuresOnce       sync.Once
	futuresMu         sync.RWMutex
	futuresClosing    bool
	futures           *futuresServices
	futuresError      error
	router            *gin.Engine
	traderManager     *trader.Manager
	stores            *Stores
	cryptoHandler     *CryptoHandler
	accountStateCache *AccountStateCache
	httpServer        *http.Server
	port              int
	telegramReloadCh  chan<- struct{} // signal Telegram bot to reload
	marketData        *feed.Service
	accountSync       *syncer.Service
	forecast          *forecast.Engine
	activity          *dashboard.Feed
	authLimiter       *ipRateLimiter // per-IP throttle for login/register
	overview          overviewCache
	assistants        sync.Map // user ID → *assistantService
}

// NewServer Creates API server
func NewServer(traderManager *trader.Manager, stores *Stores, cryptoService *crypto.CryptoService, port int) *Server {
	// Set to Release mode (reduce log output)
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	// Enable CORS
	router.Use(corsMiddleware())

	// Create crypto handler
	cryptoHandler := NewCryptoHandler(cryptoService)

	s := &Server{
		router:            router,
		traderManager:     traderManager,
		stores:            stores,
		cryptoHandler:     cryptoHandler,
		accountStateCache: NewAccountStateCache(),
		port:              port,
		// Auth throttle: allow a small burst (typos / page reloads) then ~1
		// attempt every 6s (10/min) sustained per IP. Generous for a human,
		// hostile to online password brute-force.
		authLimiter: newIPRateLimiter(1.0/6.0, 8),
	}

	// Setup routes
	s.setupRoutes()

	return s
}

// corsMiddleware returns a CORS handler. Origins come from CORS_ALLOWED_ORIGINS
// (comma-separated). The literal value "*" enables permissive mode — DO NOT use
// in production: the JWT is sent via Authorization header so a wildcard ACAO
// makes stolen tokens replayable from any site.
func corsMiddleware() gin.HandlerFunc {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	allowAny := raw == "*"
	var allowlist map[string]struct{}
	if !allowAny {
		allowlist = make(map[string]struct{})
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o == "" {
				continue
			}
			allowlist[o] = struct{}{}
		}
		if len(allowlist) == 0 {
			// Safe defaults for local development.
			for _, o := range []string{
				"http://localhost:8000",
				"http://127.0.0.1:8000",
				"http://localhost:5173",
				"http://127.0.0.1:5173",
			} {
				allowlist[o] = struct{}{}
			}
			logger.Warnf("[CORS] CORS_ALLOWED_ORIGINS not set; defaulting to localhost dev origins only. Set this env var for production.")
		}
		if allowAny {
			logger.Warnf("[CORS] CORS_ALLOWED_ORIGINS=* is INSECURE in production; restrict to your deployment origin(s).")
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			switch {
			case allowAny:
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Vary", "Origin")
			default:
				if _, ok := allowlist[origin]; ok {
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
					c.Writer.Header().Set("Vary", "Origin")
				}
				// Unknown origin: do not set ACAO; the browser will block.
			}
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Max-Age", "600")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	}
}

// setupRoutes Setup routes
func (s *Server) setupRoutes() {
	// Ensure the auth throttle exists even when the Server was constructed
	// directly (e.g. in tests) rather than via NewServer.
	if s.authLimiter == nil {
		s.authLimiter = newIPRateLimiter(1.0/6.0, 8)
	}

	// API route group
	api := s.router.Group("/api")
	{
		// Health check
		api.Any("/health", s.handleHealth)

		// Admin login (used in admin mode, public)

		// System supported models and exchanges (no authentication required)
		api.GET("/agent/providers", s.handleGetSupportedModels)

		// System config (no authentication required, for frontend to determine admin mode/registration status)
		api.GET("/system/config", s.handleGetSystemConfig)

		// Crypto related endpoints (no authentication required, not exposed to bot).
		// SECURITY: only the config + public-key endpoints are exposed. Transport
		// encryption is one-directional (client encrypts to the server's public key;
		// the server decrypts internally on the authenticated config-update handlers).
		// A public POST /crypto/decrypt would be a decryption oracle: any
		// unauthenticated caller could replay a captured ciphertext and get the
		// plaintext (exchange/API credentials) back. It is intentionally NOT
		// registered. See crypto_handler.go.
		api.GET("/crypto/config", s.cryptoHandler.HandleGetCryptoConfig)
		api.GET("/crypto/public-key", s.cryptoHandler.HandleGetPublicKey)

		// Authentication related routes (no authentication required).
		// These are throttled per-IP to blunt online password brute-force; see
		// ratelimit.go. Everything else in the public block is read-only or
		// idempotent, so the throttle is scoped to the credential endpoints.
		authRoutes := api.Group("/", rateLimitMiddleware(s.authLimiter))
		authRoutes.POST("/register", s.handleRegister)
		authRoutes.POST("/login", s.handleLogin)
		// SECURITY: password/account recovery is NOT exposed over HTTP. An
		// unauthenticated recovery endpoint is a remote auth-bypass on any
		// public-facing deployment (the confirm phrase is in the frontend and
		// returned by the API, so it is friction, not authentication). Recovery
		// is now a local CLI run on the host — `vergex reset-password` /
		// `vergex reset-user` — which requires shell access the attacker lacks.
		// See cli.go.

		// Routes requiring authentication
		protected := api.Group("/", s.authMiddleware())
		s.registerFutures(api, protected)
		s.registerAssistant(protected)
		s.registerConversations(protected)
		{
			// Logout (add to blacklist)
			protected.POST("/logout", s.handleLogout)

			// User account management
			protected.PUT("/user/password", s.handleChangePassword)
			protected.GET("/user/profile", s.handleGetProfile)
			protected.PUT("/user/display-name", s.handleUpdateDisplayName)
			protected.PUT("/user/avatar", s.handleUpdateAvatar)
			protected.DELETE("/user/avatar", s.handleRemoveAvatar)

			// Server IP query (requires authentication, for whitelist configuration)
			protected.GET("/system/ip", s.handleGetServerIP)

			// trader management
			protected.GET("/dashboard/overview", s.handleOverview)
			protected.GET("/dashboard/events", s.handleActivityStream)
			protected.GET("/traders/metrics", s.handleTraderMetrics)
			protected.GET("/traders", s.handleTraderList)
			protected.GET("/traders/:id/config", s.handleGetTraderConfig)
			protected.POST("/traders", s.handleCreateTrader)
			protected.PUT("/traders/:id", s.handleUpdateTrader)
			protected.DELETE("/traders/:id", s.handleDeleteTrader)
			protected.POST("/traders/:id/start", s.handleStartTrader)
			protected.GET("/traders/:id/preflight", s.handleTraderPreflight)
			protected.POST("/traders/preflight", s.handleLaunchPreflight)
			protected.GET("/traders/presets", s.handleTraderPresets)
			protected.POST("/traders/estimate-tokens", s.handleEstimateTokens)
			protected.POST("/traders/dry-run", s.handleTraderDryRun)
			protected.POST("/traders/:id/duplicate", s.handleDuplicateTrader)
			protected.POST("/traders/:id/stop", s.handleStopTrader)
			protected.POST("/traders/:id/resume", s.handleResumeTrader)
			protected.POST("/traders/:id/sync-balance", s.handleSyncBalance)
			protected.POST("/traders/:id/close-position", s.handleClosePosition)
			protected.GET("/traders/:id/grid-risk", s.handleGetGridRiskInfo)

			// Agent cost tracking
			protected.GET("/agent/costs", s.handleGetAgentCosts)
			protected.GET("/agent/costs/summary", s.handleGetAgentCostsSummary)
			protected.GET("/agent/storage", s.handleGetAgentStorage)

			// model configuration
			protected.GET("/agent/models", s.handleGetModelConfigs)
			protected.PUT("/agent/models", s.handleUpdateModelConfigs)
			protected.DELETE("/agent/models/:id", s.handleDeleteModel)
			protected.POST("/agent/models/test", s.handleTestModelConfig)
			protected.POST("/agent/models/available", s.handleModelFormList)
			protected.GET("/agent/models/:id/available", s.handleSavedModelList)

			// Exchange configuration
			protected.GET("/accounts", s.handleGetAccountConfigs)
			protected.GET("/accounts/state", s.handleGetAccountStates)
			protected.POST("/accounts/:id/verify", s.handleVerifyAccount)
			protected.POST("/accounts", s.handleCreateAccount)
			protected.PUT("/accounts", s.handleUpdateAccountConfigs)
			protected.GET("/accounts/:id", s.handleGetAccount)
			protected.PUT("/accounts/:id/name", s.handleRenameAccount)
			protected.GET("/accounts/:id/positions", s.handleAccountPositions)
			protected.GET("/accounts/:id/marks", s.handleAccountMarks)
			protected.GET("/accounts/:id/open-orders", s.handleAccountOpenOrders)
			protected.GET("/accounts/:id/orders", s.handleAccountOrders)
			protected.GET("/accounts/:id/fills", s.handleAccountFills)
			protected.DELETE("/accounts/:id", s.handleDeleteAccount)

			// Telegram bot configuration
			protected.GET("/telegram", s.handleGetTelegramConfig)
			protected.POST("/telegram", s.handleUpdateTelegramConfig)
			protected.DELETE("/telegram/token", s.handleClearTelegramToken)
			protected.POST("/telegram/model", s.handleUpdateTelegramModel)
			protected.POST("/telegram/bind-code", s.handleTelegramBindCode)
			protected.GET("/telegram/visitors", s.handleTelegramVisitors)
			// Local market data
			protected.GET("/market/status", s.handleMarketDataStatus)
			protected.GET("/market/settings", s.handleGetMarketDataSettings)
			protected.PUT("/market/settings", s.handleUpdateMarketDataSettings)
			protected.POST("/market/backfill", s.handleMarketDataBackfill)

			// Forecast
			protected.GET("/forecast/signals", s.handleForecastCoins)
			protected.GET("/forecast/detail/:symbol", s.handleForecastDetail)
			protected.GET("/forecast/overlay/:symbol", s.handleForecastOverlay)
			protected.GET("/forecast/strategies", s.handleForecastStrategies)
			protected.POST("/forecast/strategies", s.handleCreateForecastStrategy)
			protected.PUT("/forecast/strategies/:id", s.handleRenameForecastStrategy)
			protected.DELETE("/forecast/strategies/:id", s.handleDeleteForecastStrategy)
			protected.POST("/forecast/strategies/:id/activate", s.handleActivateForecastStrategy)
			protected.POST("/forecast/strategies/:id/deactivate", s.handleDeactivateForecastStrategy)
			protected.POST("/forecast/strategies/:id/revive", s.handleReviveForecastStrategy)
			protected.GET("/forecast/strategies/:id/performance", s.handleForecastPerformance)
			protected.GET("/forecast/settings", s.handleGetForecastSettings)
			protected.PUT("/forecast/settings", s.handleUpdateForecastSettings)
			protected.GET("/forecast/coverage", s.handleForecastCoverage)
			protected.GET("/forecast/backtests", s.handleForecastBacktests)
			protected.POST("/forecast/backtests", s.handleCreateForecastBacktest)
			protected.GET("/forecast/backtests/:id", s.handleForecastBacktest)
			protected.PUT("/forecast/backtests/:id", s.handleRenameForecastBacktest)
			protected.DELETE("/forecast/backtests/:id", s.handleDeleteForecastBacktest)
			protected.POST("/forecast/backtests/:id/cancel", s.handleCancelForecastBacktest)

			protected.PUT("/telegram/alerts", s.handleUpdateTelegramAlerts)
			protected.POST("/telegram/alerts/test", s.handleTestTelegramAlert)
			protected.DELETE("/telegram/binding", s.handleUnbindTelegram)

			// Data of one trader. :id is the EXACT "trader_id" field from GET /api/traders.
			protected.GET("/traders/:id/status", s.handleStatus)
			protected.GET("/traders/:id/account", s.handleAccount)
			protected.GET("/traders/:id/open-positions", s.handlePositions)
			protected.GET("/traders/:id/positions", s.handlePositionHistory)
			protected.GET("/traders/:id/trades", s.handleTrades)
			protected.GET("/traders/:id/orders", s.handleOrders)
			protected.GET("/traders/:id/orders/:orderId/fills", s.handleOrderFills)
			protected.GET("/traders/:id/open-orders", s.handleOpenOrders)
			protected.GET("/traders/:id/decisions", s.handleDecisions)
			protected.GET("/traders/:id/decisions/latest", s.handleLatestDecisions)
			protected.GET("/traders/:id/decisions/summary", s.handleDecisionSummaries)
			protected.GET("/traders/:id/decisions/:decisionId", s.handleDecisionDetail)
			protected.GET("/traders/:id/equity", s.handleEquity)
			protected.GET("/traders/:id/statistics", s.handleStatistics)
			protected.GET("/traders/:id/statistics/full", s.handleStatisticsFull)

		}
	}
}

// handleHealth Health check
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   c.Request.Context().Value("time"),
	})
}

// handleGetSystemConfig Get system configuration (configuration that client needs to know)
func (s *Server) handleGetSystemConfig(c *gin.Context) {
	userCount, _ := s.stores.Users.Count()
	c.JSON(http.StatusOK, gin.H{
		"initialized": userCount > 0,
	})
}

// handleGetServerIP Get server IP address (for whitelist configuration)
func (s *Server) handleGetServerIP(c *gin.Context) {
	// Try to get public IP via third-party API
	publicIP := getPublicIPFromAPI()

	// If third-party API fails, get first public IP from network interface
	if publicIP == "" {
		publicIP = getPublicIPFromInterface()
	}

	// If still cannot get it, return error
	if publicIP == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to get public IP address"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"public_ip": publicIP,
		"message":   "Please add this IP address to the whitelist",
	})
}

// getPublicIPFromAPI Get public IP via third-party API (IPv4 only)
func getPublicIPFromAPI() string {
	// Try multiple public IP query services (IPv4-only endpoints)
	services := []string{
		"https://api4.ipify.org?format=text", // IPv4 only
		"https://ipv4.icanhazip.com",         // IPv4 only
		"https://v4.ident.me",                // IPv4 only
		"https://api.ipify.org?format=text",  // May return IPv4 or IPv6
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body := make([]byte, 128)
			n, err := resp.Body.Read(body)
			if err != nil && err.Error() != "EOF" {
				continue
			}

			ip := strings.TrimSpace(string(body[:n]))
			parsedIP := net.ParseIP(ip)
			// Verify if it's a valid IPv4 address (not containing ":")
			if parsedIP != nil && parsedIP.To4() != nil {
				return ip
			}
		}
	}

	return ""
}

// getPublicIPFromInterface Get first public IP from network interface
func getPublicIPFromInterface() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		// Skip disabled interfaces and loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Only consider IPv4 addresses
			if ip.To4() != nil {
				ipStr := ip.String()
				// Exclude private IP address ranges
				if !isPrivateIP(ip) {
					return ipStr
				}
			}
		}
	}

	return ""
}

// isPrivateIP Determine if it's a private IP address
func isPrivateIP(ip net.IP) bool {
	// Private IP address ranges:
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// loadedTraderID resolves the trader in the path, strictly scoped to the authenticated
// caller, after making sure the caller's traders are loaded into memory. Handlers that
// only read the database use ownedTrader instead.
func (s *Server) loadedTraderID(c *gin.Context) (string, error) {
	userID := c.GetString("user_id")

	// Ensure user's traders are loaded into memory. A trader whose load just
	// failed is left alone for a while: loading can call the exchange, and
	// these endpoints are polled.
	if err := s.traderManager.EnsureUserTradersLoaded(userID); err != nil {
		logger.Infof("⚠️ Failed to load traders for user %s: %v", userID, err)
	}

	t, err := s.ownedTrader(c)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// ownedTrader resolves the trader in the path against the caller's own trader list in
// the store. It loads nothing into memory, so history kept in the database stays readable
// while the exchange is not.
//
// Ownership is always enforced against the caller's own trader list in the store. We
// deliberately never fall back to the global in-memory trader map (Manager holds every
// account's traders): returning an entry from it for a trader the caller does not own is
// a cross-tenant data leak (IDOR).
func (s *Server) ownedTrader(c *gin.Context) (*trader.Trader, error) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	userTraders, err := s.stores.Traders.List(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load traders for this account: %w", err)
	}
	for _, t := range userTraders {
		if t.ID == traderID {
			return t, nil
		}
	}
	return nil, fmt.Errorf("trader not found for this account")
}

// initialBalance is the balance P&L is measured against: the loaded trader's,
// else the stored one.
func (s *Server) initialBalance(t *trader.Trader) float64 {
	if at, err := s.traderManager.GetTrader(t.ID); err == nil {
		return at.GetInitialBalance()
	}
	return t.InitialBalance
}

// authMiddleware JWT authentication middleware
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
			c.Abort()
			return
		}

		// Check Bearer token format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization format"})
			c.Abort()
			return
		}

		tokenString := tokenParts[1]

		// Blacklist check
		if auth.IsTokenBlacklisted(tokenString) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token expired, please login again"})
			c.Abort()
			return
		}

		// Validate JWT token
		claims, err := auth.ValidateJWT(tokenString)
		if err != nil {
			logger.Errorf("[Auth] Invalid token: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// A token outlives its user after reset-user or a new database, and
		// would otherwise open an empty session for a user that does not exist.
		if _, err := s.stores.Users.GetByID(claims.UserID); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Account no longer exists, please login again"})
			c.Abort()
			return
		}

		// Store user information in context
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}

// Start Start server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	logger.Infof("🌐 API server starting at http://localhost%s", addr)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown Gracefully shutdown server
func (s *Server) Shutdown() error {
	s.futuresMu.Lock()
	s.futuresClosing = true
	planning := s.futures
	s.futuresMu.Unlock()
	if planning != nil {
		planning.controller.Shutdown()
	}
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// SetMarketData attaches local market data; without it those endpoints report
// that it is unavailable.
func (s *Server) SetMarketData(service *feed.Service) {
	s.marketData = service
}

// SetForecast attaches the forecast engine; without it those endpoints report
// that it is unavailable.
func (s *Server) SetForecast(engine *forecast.Engine) {
	s.forecast = engine
}

// SetAccountSync attaches what keeps the trading ledger up to date.
func (s *Server) SetAccountSync(service *syncer.Service) {
	s.accountSync = service
}

// SetActivity attaches the Dashboard's event feed.
func (s *Server) SetActivity(feed *dashboard.Feed) {
	s.activity = feed
}

// SetTelegramReloadCh sets the channel used to signal the Telegram bot to reload
func (s *Server) SetTelegramReloadCh(ch chan<- struct{}) {
	s.telegramReloadCh = ch
}
