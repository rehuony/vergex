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
		s.route(api, "GET", "/agent/providers", "List supported model providers", s.handleGetSupportedModels)

		// System config (no authentication required, for frontend to determine admin mode/registration status)
		s.route(api, "GET", "/system/config", "Get system configuration", s.handleGetSystemConfig)

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
		s.route(authRoutes, "POST", "/register", "Register new user", s.handleRegister)
		s.route(authRoutes, "POST", "/login", "User login, returns JWT token", s.handleLogin)
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
		{
			// Logout (add to blacklist)
			s.route(protected, "POST", "/logout", "Logout (blacklist token)", s.handleLogout)

			// User account management
			s.routeWithSchema(protected, "PUT", "/user/password", "Change current user password",
				`Body: {"current_password":"<string>","new_password":"<string, min 8 chars>"}`,
				s.handleChangePassword)
			s.route(protected, "GET", "/user/profile", "Current user's email, display name and avatar", s.handleGetProfile)
			s.routeWithSchema(protected, "PUT", "/user/display-name", "Set the current user's display name",
				`Body: {"display_name":"<string, 1-32 characters>"}`,
				s.handleUpdateDisplayName)
			s.routeWithSchema(protected, "PUT", "/user/avatar", "Set the current user's avatar",
				`multipart/form-data, field "avatar": PNG, JPEG or WebP up to 2 MB and 4096 px a side; stored redrawn as a 256 px square`,
				s.handleUpdateAvatar)
			s.route(protected, "DELETE", "/user/avatar", "Remove the current user's avatar", s.handleRemoveAvatar)

			// Server IP query (requires authentication, for whitelist configuration)
			s.route(protected, "GET", "/system/ip", "Get server public IP (for exchange whitelist)", s.handleGetServerIP)

			// trader management
			s.routeWithSchema(protected, "GET", "/dashboard/overview", "Read-only overview of accounts, traders, services and positions",
				`Returns accounts, models and their hosts, traders, active Futures plans, services, what market data follows, the forecast strategy in use, whether Telegram is bound, positions (normalized), realized P/L today (UTC) and what needs attention. Cached for 15 seconds, because it reads from the exchanges.`,
				s.handleOverview)
			s.routeWithSchema(protected, "GET", "/dashboard/events", "Stream of model calls, orders, money and market reads",
				`Server-sent events. The first message has type "snapshot" with the recent events; then "events" with new ones, and "totals" every 20 seconds. Each carries {"seq","at","events":[...],"totals":{"<from>><to>":{"h1":{...},"h24":{...}}}}. Reads the database only.`,
				s.handleActivityStream)
			s.routeWithSchema(protected, "GET", "/traders/metrics", "Stored equity and trade totals of each trader",
				`Returns: {"<trader_id>":{"equity":<float>,"equity_at":<unix seconds>,"return_pct":<float>,"total_trades":<int>,"win_rate":<float>,"max_drawdown_pct":<float>}}. A value is absent when no data was recorded yet. Reads the database only.`,
				s.handleTraderMetrics)
			s.routeWithSchema(protected, "GET", "/traders", "List user's traders with status",
				`Returns: [{"trader_id":"<EXACT id — use this as trader_id in every /api/traders/:id path>","trader_name":"<string>","is_running":<bool>}]
NOTE: The id field is "trader_id" (NOT "id"). Always read trader_id from this endpoint before querying data.`,
				s.handleTraderList)
			s.routeWithSchema(protected, "GET", "/traders/:id/config", "Get full trader configuration",
				`:id = trader_id from GET /api/traders`,
				s.handleGetTraderConfig)
			s.routeWithSchema(protected, "POST", "/traders", "Create a new trader",
				`Body: {"name":"<string, required>","agent_model_id":"<EXACT id field from GET /api/agent/models — e.g. 'abc123_deepseek', NOT the provider name 'deepseek'>","account_id":"<EXACT id field from GET /api/accounts — e.g. '05785d3b-841e-...', NOT the type name>","kind":"agent|grid, default agent; cannot change later","preset":"<id from GET /api/traders/presets, default the kind's first>","config":<partial config applied on the preset, optional>,"scan_interval_minutes":<int, default 15, minimum 3>}
IMPORTANT: agent_model_id and account_id must be the full "id" value from the Account State, not the provider/type name.`,
				s.handleCreateTrader)
			s.routeWithSchema(protected, "PUT", "/traders/:id", "Update trader configuration",
				`:id = trader_id from GET /api/traders
Body: {"name":"<string>","agent_model_id":"<EXACT id from GET /api/agent/models>","account_id":"<EXACT id from GET /api/accounts>","config":<partial config, merged by section>,"scan_interval_minutes":<int, min 3>,"is_cross_margin":<bool>}
Only include fields you want to change. A change to only the config of a running agent trader takes effect on its next cycle; any other change restarts a running trader.`,
				s.handleUpdateTrader)
			s.routeWithSchema(protected, "DELETE", "/traders/:id", "Delete trader",
				`:id = trader_id from GET /api/traders. Stops and permanently removes the trader and all its data.`,
				s.handleDeleteTrader)
			s.routeWithSchema(protected, "POST", "/traders/:id/start", "Start trader — begins live trading",
				`:id = trader_id from GET /api/traders. No request body needed. The trader must have a valid exchange and model configured.
Runs launch preflight first and returns 400 with {"error_key":"trader.start.preflight_failed","preflight":{...}} when checks fail. Append ?force=true to skip balance gates.`,
				s.handleStartTrader)
			s.routeWithSchema(protected, "GET", "/traders/:id/preflight", "Run launch readiness checks for a trader",
				`:id = trader_id from GET /api/traders.
Returns: {"ready":<bool>,"checks":[{"id":"agent_model|config|account_config|account_state|account_funds","status":"ok|failed|warning|skipped","code":"<string>","message":"<string>","required":<number>,"actual":<number>,"asset":"<string>","address":"<string>"}],"min_trading_usdc":<number>}`,
				s.handleTraderPreflight)
			s.routeWithSchema(protected, "POST", "/traders/preflight", "Run launch readiness checks before creating a trader",
				`Body: {"agent_model_id":"<EXACT id from GET /api/agent/models>","account_id":"<EXACT id from GET /api/accounts>","kind":"agent|grid","config":<optional, checked when sent>}
Returns the same shape as GET /api/traders/:id/preflight. Use this before POST /api/traders to surface every missing prerequisite at once.`,
				s.handleLaunchPreflight)
			s.routeWithSchema(protected, "GET", "/traders/presets", "Starting configs of a trader kind",
				`Query: ?kind=agent|grid (default agent). Returns: {"kind":"<string>","presets":[{"id":"<string>","config":<whole config>}]}, the default first.
Agent config sections:
  coin_source.source_type: "static" (default) | "oi_top" | "oi_low" | "volume_top" | "gainers" | "losers" | "mixed"
    static: fixed list; oi_top/oi_low: largest open-interest increase/decrease among the highest-volume Binance USDT perpetuals;
    volume_top: top 24h quote volume; gainers/losers: top 24h price change; mixed: combine use_* sources with static_coins
  coin_source.static_coins: e.g. ["BTCUSDT","ETHUSDT","SOLUSDT"] (max 10)
  coin_source.excluded_coins: symbols never traded
  coin_source.oi_top_limit / oi_low_limit / volume_top_limit / gainers_limit / losers_limit: candidates per source (default 3, max 10)
  coin_source.use_oi_top / use_oi_low / use_volume_top / use_gainers / use_losers: sources enabled in "mixed"
  coin_source.oi_window: "1h" | "4h" (default) | "24h"; coin_source.oi_pool_size: symbols evaluated for OI change (default 50)
  coin_source.min_quote_volume: minimum 24h quote volume in USDT for ranking sources (default 0)
  indicators.klines.primary_timeframe: "1m"|"3m"|"5m"|"15m"|"1h"|"4h"
  indicators.klines.primary_count: number of candles (10-30)
  indicators.klines.selected_timeframes: e.g. ["5m","15m","1h","4h"] (max 4)
  indicators.enable_ema / enable_macd / enable_rsi / enable_boll / enable_atr / enable_volume / enable_oi / enable_funding_rate
  indicators.ema_periods / rsi_periods / atr_periods / boll_periods
  risk_control.max_positions (1-8), btc_eth_max_leverage / altcoin_max_leverage (1-20)
  risk_control.btc_eth_max_position_value_ratio / altcoin_max_position_value_ratio (0.5-10)
  risk_control.max_margin_usage (0.1-1), min_position_size (10-1000 USDT), min_risk_reward_ratio (1-10), min_confidence (50-100)
  prompt_sections.role_definition / trading_frequency / entry_standards / decision_process; custom_prompt
Grid config fields: symbol, grid_count (5-50), total_investment, leverage (1-20), upper_price, lower_price, use_atr_bounds, atr_multiplier, distribution ("uniform"|"gaussian"|"pyramid"), max_drawdown_pct, stop_loss_pct, daily_loss_limit_pct, use_maker_only, enable_direction_adjust, direction_bias_ratio`,
				s.handleTraderPresets)
			s.route(protected, "POST", "/traders/estimate-tokens", "Estimate token usage of an agent config", s.handleEstimateTokens)
			s.routeWithSchema(protected, "POST", "/traders/dry-run", "Build the prompts a config would send, without placing an order",
				`Body: {"kind":"agent|grid, default agent","config":<whole config>,"agent_model_id":"<optional: also send the prompts to this model>"}
Returns: {"system_prompt","user_prompt","candidate_count","candidates":[{"symbol","sources"}],"agent_response","agent_error"}. Reads live public data; places no order.`,
				s.handleTraderDryRun)
			s.routeWithSchema(protected, "POST", "/traders/:id/duplicate", "Create a stopped copy of a trader",
				`:id = trader_id from GET /api/traders. Body: {"name":"<string, required>"}. Copies the kind, config, model, account, interval and margin mode. Its starting balance is read from the exchange again.`,
				s.handleDuplicateTrader)
			s.routeWithSchema(protected, "POST", "/traders/:id/stop", "Stop trader — halts live trading",
				`:id = trader_id from GET /api/traders. No request body needed. Gracefully stops the trading loop.`,
				s.handleStopTrader)
			s.routeWithSchema(protected, "POST", "/traders/:id/sync-balance", "Sync account balance from exchange",
				`:id = trader_id from GET /api/traders. No request body needed. Refreshes initial_balance from the exchange.`,
				s.handleSyncBalance)
			s.routeWithSchema(protected, "POST", "/traders/:id/close-position", "Force-close an open position",
				`:id = trader_id from GET /api/traders.
Body: {"symbol":"<string, e.g. BTCUSDT — must match an open position symbol from GET /api/traders/:id/open-positions>"}`,
				s.handleClosePosition)
			s.routeWithSchema(protected, "GET", "/traders/:id/grid-risk", "Get grid trading risk info",
				`:id = trader_id from GET /api/traders. Returns null while no grid is running, as for a stopped trader.`,
				s.handleGetGridRiskInfo)

			// Agent cost tracking
			s.route(protected, "GET", "/agent/costs", "Get model call costs for a trader (?trader_id=xxx&period=today)", s.handleGetAgentCosts)
			s.route(protected, "GET", "/agent/costs/summary", "Get agent cost summary (?period=today, optional &trader_id=xxx)", s.handleGetAgentCostsSummary)

			// model configuration
			s.routeWithSchema(protected, "GET", "/agent/models", "List model configs",
				`Returns: [{"id":"<EXACT id — use this as agent_model_id when creating/updating a trader>","name":"<display name>","provider":"<short provider name — NOT a valid id>","enabled":<bool>}]
CRITICAL: The "id" field (a UUID) is what you must use for agent_model_id. The "provider" field ("deepseek") is NOT valid as an id.`,
				s.handleGetModelConfigs)
			s.routeWithSchema(protected, "PUT", "/agent/models", "Configure a model provider",
				`Body: {"models":{"<model_id>":{"enabled":<bool>,"api_key":"<string>","custom_api_url":"<string, leave empty to use provider default>","custom_model_name":"<string, leave empty to use provider default>"}}}
model_id values: "openai","deepseek","qwen","kimi","grok","gemini","claude"
Defaults when custom fields empty: openai→api.openai.com/v1, deepseek→api.deepseek.com, qwen→dashscope.aliyuncs.com/compatible-mode/v1, kimi→api.moonshot.ai/v1, grok→api.x.ai/v1, gemini→generativelanguage.googleapis.com/v1beta/openai, claude→api.anthropic.com/v1`,
				s.handleUpdateModelConfigs)
			s.routeWithSchema(protected, "POST", "/agent/models/test", "Check that a model answers with the given settings",
				`Body: {"agent_model_id":"<id from GET /api/agent/models, or a provider id for a new model>","api_key":"<string, blank uses the saved key>","custom_api_url":"<string>","custom_model_name":"<string>"}
Returns: {"ok":<bool>,"model":"<model name>","latency_ms":<number, when ok>,"error":"<provider message, when not ok>"}. Sends one short prompt; nothing is saved.`,
				s.handleTestModelConfig)

			// Exchange configuration
			s.routeWithSchema(protected, "GET", "/accounts", "List exchange accounts",
				`Returns: [{"id":"<EXACT id — use this as account_id when creating/updating a trader>","venue":"<e.g. okx, binance>","name":"<user label>","enabled":<bool>}]
CRITICAL: Always use the "id" field for account_id. Do not use "venue" as an id.`,
				s.handleGetAccountConfigs)
			s.routeWithSchema(protected, "GET", "/accounts/state", "Get connection and balance state for each exchange account",
				`Returns: {"states":{"<account_id>":{"status":"ok|missing_credentials|invalid_credentials|permission_denied|unavailable","display_balance":"<string>","total_equity":<number>,"available_balance":<number>,"asset":"USDT","checked_at":"<RFC3339>","error_code":"<string>","error_message":"<string>"}}}
Use this endpoint to show balance and health in the exchange list without depending on traders.`,
				s.handleGetAccountStates)
			s.routeWithSchema(protected, "POST", "/accounts/:id/verify", "Check one exchange account now",
				`:id = EXACT id from GET /api/accounts. Read-only: fetches the balance past the 30 s cache and returns that account's state (same shape as one entry of GET /api/accounts/state).`,
				s.handleVerifyAccount)
			s.routeWithSchema(protected, "POST", "/accounts", "Create a new exchange account",
				`Body: {"venue":"<string>","name":"<string, user label>","enabled":true,"api_key":"<string>","secret_key":"<string>","passphrase":"<string, required for okx>"}
venue values: "binance","okx" (USDT perpetuals)
Required fields by exchange:
  binance: api_key + secret_key
  okx: api_key + secret_key + passphrase`,
				s.handleCreateAccount)
			s.routeWithSchema(protected, "PUT", "/accounts", "Update an existing exchange account configuration",
				`Body: {"id":"<EXACT id from GET /api/accounts>","venue":"<string>","name":"<string>","enabled":<bool>,"api_key":"<string>","secret_key":"<string>","passphrase":"<string, for okx>"}
Use this to enable/disable an exchange or update API credentials. The "id" field is required to identify which exchange to update.`,
				s.handleUpdateAccountConfigs)
			s.route(protected, "GET", "/accounts/:id", "One account with its connection and balance state", s.handleGetAccount)
			s.route(protected, "GET", "/accounts/:id/positions", "Positions open on an account now", s.handleAccountPositions)
			s.routeWithSchema(protected, "GET", "/accounts/:id/open-orders", "Orders open on an account now",
				`Query: ?symbol=<string, required>`, s.handleAccountOpenOrders)
			s.routeWithSchema(protected, "GET", "/accounts/:id/orders", "Orders in the ledger of an account, whoever placed them",
				`Query: ?symbol=<string>&status=<string>&limit=<int, default 100>`, s.handleAccountOrders)
			s.routeWithSchema(protected, "GET", "/accounts/:id/fills", "Fills in the ledger of an account, newest first",
				`Query: ?symbol=<string>&limit=<int, default 100>`, s.handleAccountFills)
			s.routeWithSchema(protected, "DELETE", "/accounts/:id", "Delete exchange account",
				`:id = EXACT id from GET /api/accounts. Permanently removes the exchange account and disconnects any traders using it.`,
				s.handleDeleteAccount)

			// Telegram bot configuration
			s.routeWithSchema(protected, "GET", "/telegram", "Get Telegram bot configuration",
				`Returns: {"bot_token":"<string>","agent_model_id":"<EXACT id of configured model>","chat_id":"<bound Telegram chat id, empty if not bound>"}`,
				s.handleGetTelegramConfig)
			s.routeWithSchema(protected, "POST", "/telegram", "Set Telegram bot token and model",
				`Body: {"bot_token":"<string — Telegram BotFather token>","agent_model_id":"<EXACT id from GET /api/agent/models>"}
Both fields are required. After saving, the user must send /start in Telegram to bind their account.`,
				s.handleUpdateTelegramConfig)
			s.routeWithSchema(protected, "POST", "/telegram/model", "Update Telegram bot model only",
				`Body: {"agent_model_id":"<EXACT id from GET /api/agent/models>"}`,
				s.handleUpdateTelegramModel)
			// Local market data
			s.route(protected, "GET", "/market/status", "Get market data collection status", s.handleMarketDataStatus)
			s.route(protected, "GET", "/market/settings", "Get market data settings", s.handleGetMarketDataSettings)
			s.route(protected, "PUT", "/market/settings", "Update market data settings", s.handleUpdateMarketDataSettings)
			s.route(protected, "POST", "/market/backfill", "Load deep history from Binance's published files", s.handleMarketDataBackfill)

			// Forecast
			s.route(protected, "GET", "/forecast/signals", "List the latest forecast of each tracked coin (?interval=1h)", s.handleForecastCoins)
			s.route(protected, "GET", "/forecast/detail/:symbol", "Get one coin's forecast, factors and record (?interval=1h)", s.handleForecastDetail)
			s.route(protected, "GET", "/forecast/overlay/:symbol", "Get what a chart draws of a coin's forecast (?interval=1h)", s.handleForecastOverlay)
			s.route(protected, "GET", "/forecast/strategies", "List forecast strategies", s.handleForecastStrategies)
			s.route(protected, "POST", "/forecast/strategies", "Save a new forecast strategy", s.handleCreateForecastStrategy)
			s.route(protected, "PUT", "/forecast/strategies/:id", "Rename a forecast strategy", s.handleRenameForecastStrategy)
			s.route(protected, "DELETE", "/forecast/strategies/:id", "Delete a forecast strategy not in use", s.handleDeleteForecastStrategy)
			s.route(protected, "POST", "/forecast/strategies/:id/activate", "Use a forecast strategy", s.handleActivateForecastStrategy)
			s.route(protected, "POST", "/forecast/strategies/:id/deactivate", "Stop using a forecast strategy", s.handleDeactivateForecastStrategy)
			s.route(protected, "POST", "/forecast/strategies/:id/revive", "Use a retired forecast strategy again, clearing what it recorded", s.handleReviveForecastStrategy)
			s.route(protected, "GET", "/forecast/strategies/:id/performance", "Get how a forecast strategy has done live", s.handleForecastPerformance)
			s.route(protected, "GET", "/forecast/settings", "Get forecast settings", s.handleGetForecastSettings)
			s.route(protected, "PUT", "/forecast/settings", "Update forecast settings", s.handleUpdateForecastSettings)
			s.route(protected, "GET", "/forecast/coverage", "Report local history (?symbols=&intervals=)", s.handleForecastCoverage)
			s.route(protected, "GET", "/forecast/backtests", "List forecast backtests, each with its overall result", s.handleForecastBacktests)
			s.route(protected, "POST", "/forecast/backtests", "Run a forecast backtest on local history", s.handleCreateForecastBacktest)
			s.route(protected, "GET", "/forecast/backtests/:id", "Get one forecast backtest with its whole report", s.handleForecastBacktest)
			s.route(protected, "PUT", "/forecast/backtests/:id", "Rename a forecast backtest", s.handleRenameForecastBacktest)
			s.route(protected, "DELETE", "/forecast/backtests/:id", "Delete a forecast backtest that is not running", s.handleDeleteForecastBacktest)
			s.route(protected, "POST", "/forecast/backtests/:id/cancel", "Stop a running forecast backtest", s.handleCancelForecastBacktest)

			s.route(protected, "PUT", "/telegram/alerts", "Turn Telegram alert kinds on or off", s.handleUpdateTelegramAlerts)
			s.route(protected, "POST", "/telegram/alerts/test", "Send a Telegram test alert", s.handleTestTelegramAlert)
			s.routeWithSchema(protected, "DELETE", "/telegram/binding", "Unbind Telegram account",
				`No body needed. Clears the Telegram chat_id binding so the user can re-bind with /start.`,
				s.handleUnbindTelegram)

			// Data of one trader. :id is the EXACT "trader_id" field from GET /api/traders.
			s.routeWithSchema(protected, "GET", "/traders/:id/status", "Trader running status",
				`:id = trader_id from GET /api/traders.
Returns: {"is_running":<bool>,"trader_id":"<string>"}`,
				s.handleStatus)
			s.routeWithSchema(protected, "GET", "/traders/:id/account", "Account balance and equity",
				`:id = trader_id from GET /api/traders.
Returns: {"balance":<float>,"equity":<float>,"unrealized_pnl":<float>,"initial_balance":<float>,"total_return_pct":<float>}`,
				s.handleAccount)
			s.routeWithSchema(protected, "GET", "/traders/:id/open-positions", "Current open positions",
				`:id = trader_id from GET /api/traders.
Returns: [{"symbol":"<string>","side":"long|short","size":<float>,"entry_price":<float>,"mark_price":<float>,"unrealized_pnl":<float>,"leverage":<int>}]`,
				s.handlePositions)
			s.routeWithSchema(protected, "GET", "/traders/:id/positions", "Closed position history",
				`:id = trader_id from GET /api/traders. Query: limit=<int, default 100, max 500>&before=<position id, for older pages, which return only positions>`,
				s.handlePositionHistory)
			s.routeWithSchema(protected, "GET", "/traders/:id/trades", "Trade records",
				`:id = trader_id from GET /api/traders. Query: limit=<int, default 20>`,
				s.handleTrades)
			s.routeWithSchema(protected, "GET", "/traders/:id/orders", "All order records",
				`:id = trader_id from GET /api/traders. Query: limit=<int, default 20>`,
				s.handleOrders)
			s.routeWithSchema(protected, "GET", "/traders/:id/orders/:orderId/fills", "Order fill details",
				`:orderId = order id from GET /api/traders/:id/orders`,
				s.handleOrderFills)
			s.routeWithSchema(protected, "GET", "/traders/:id/open-orders", "Open orders currently on exchange",
				`:id = trader_id from GET /api/traders.`,
				s.handleOpenOrders)
			s.routeWithSchema(protected, "GET", "/traders/:id/decisions", "Agent trading decisions (decision records)",
				`:id = trader_id from GET /api/traders. Query: limit=<int, default 20>
Returns: [{"id":"<string>","symbol":"<string>","action":"open_long|open_short|close_long|close_short|hold","confidence":<int>,"reasoning":"<string>","created_at":"<timestamp>"}]`,
				s.handleDecisions)
			s.routeWithSchema(protected, "GET", "/traders/:id/decisions/latest", "Latest agent decisions (most recent scan results)",
				`:id = trader_id from GET /api/traders.
Returns the most recent agent decision for each symbol analyzed in the last scan cycle.`,
				s.handleLatestDecisions)
			s.routeWithSchema(protected, "GET", "/traders/:id/decisions/summary", "Decisions without prompts and raw responses, newest first",
				`:id = trader_id from GET /api/traders. Query: limit=<int, default 50, max 100>&before=<decision id, for older pages>`,
				s.handleDecisionSummaries)
			s.routeWithSchema(protected, "GET", "/traders/:id/decisions/:decisionId", "One full decision record",
				`:decisionId = id from GET /api/traders/:id/decisions/summary.`,
				s.handleDecisionDetail)
			s.routeWithSchema(protected, "GET", "/traders/:id/equity", "Equity curve",
				`:id = trader_id from GET /api/traders. Query: range=24h|7d|30d|all (default 7d)
Returns: {"points":[{"time":<unix seconds>,"equity":<float>,"unrealized_pnl":<float>}],"initial_balance":<float>}, at most 500 points.`,
				s.handleEquity)
			s.routeWithSchema(protected, "GET", "/traders/:id/statistics", "Trading performance statistics",
				`:id = trader_id from GET /api/traders.
Returns: {"total_trades":<int>,"winning_trades":<int>,"win_rate":<float>,"total_pnl":<float>,"sharpe_ratio":<float>,"max_drawdown":<float>}`,
				s.handleStatistics)
			s.routeWithSchema(protected, "GET", "/traders/:id/statistics/full", "Full trade-quality metrics (win rate, profit factor, Sharpe, max drawdown)",
				`:id = trader_id from GET /api/traders.
Returns: {"total_trades","win_trades","loss_trades","win_rate","profit_factor","sharpe_ratio","total_pnl","total_fee","avg_win","avg_loss","max_drawdown_pct"}`,
				s.handleStatisticsFull)

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
