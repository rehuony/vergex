package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"vergex/internal/account"
	"vergex/internal/account/syncer"
	"vergex/internal/agent"
	"vergex/internal/agent/pricing"
	_ "vergex/internal/agent/provider"
	"vergex/internal/api"
	"vergex/internal/config"
	"vergex/internal/dashboard"
	"vergex/internal/database"
	"vergex/internal/forecast"
	"vergex/internal/logger"
	"vergex/internal/market"
	"vergex/internal/market/feed"
	"vergex/internal/security/auth"
	"vergex/internal/security/crypto"
	"vergex/internal/service"
	"vergex/internal/telegram"
	"vergex/internal/trader"

	"github.com/joho/godotenv"
)

func main() {
	// Local admin subcommands (account recovery) run directly against the
	// database and never start the HTTP server. Recovery therefore requires
	// shell/file access to the host instead of a network request, which keeps
	// it safe even when Vergex is exposed to the public internet. See cli.go.
	if runCLISubcommand(os.Args[1:]) {
		return
	}

	// Load .env environment variables
	_ = godotenv.Load()

	// Initialize logger
	logger.Init(nil)

	logger.Info("╔════════════════════════════════════════════════════════════╗")
	logger.Info("║          🚀 Vergex - Agent Trading System                  ║")
	logger.Info("╚════════════════════════════════════════════════════════════╝")

	// Initialize global configuration (loaded from .env).
	// MustInit refuses to start under an insecure config (e.g. missing or default JWT_SECRET).
	config.MustInit()
	cfg := config.Get()
	logger.Info("✅ Configuration loaded")

	// Initialize encryption service BEFORE database (so EncryptedString can decrypt on read)
	logger.Info("🔐 Initializing encryption service...")
	cryptoService, err := crypto.NewCryptoService()
	if err != nil {
		logger.Fatalf("❌ Failed to initialize encryption service: %v", err)
	}
	crypto.SetGlobalCryptoService(cryptoService)
	logger.Info("✅ Encryption service initialized successfully")

	// Initialize database from configuration
	logger.Info("📋 Initializing database...")
	db, err := database.OpenMigrated(databaseConfig(cfg))
	if err != nil {
		logger.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer database.Close(db)
	logger.Infof("✅ Database initialized (GORM, PostgreSQL)")

	// Agent cost tracking: record every call to a model in agent.calls, priced from
	// the models.dev catalog cached in the database.
	pricingCtx, stopPricing := context.WithCancel(context.Background())
	defer stopPricing()
	prices := pricing.NewService(db)
	prices.Start(pricingCtx)
	agent.CallCallback = recordAgentCalls(agent.NewCallStore(db), prices)

	// Set JWT secret
	auth.SetJWTSecret(cfg.JWTSecret)
	logger.Info("🔑 JWT secret configured")

	// Market data comes up before any trader can start. A failure here leaves the
	// rest of the system running: trading reads the exchange directly without it.
	marketCtx, stopMarket := context.WithCancel(context.Background())
	defer stopMarket()
	var marketData *feed.Service
	marketStore, err := openMarketStore(cfg)
	if err != nil {
		logger.Errorf("Market data is unavailable: %v", err)
	} else {
		defer marketStore.Close()
		marketData = feed.NewService(marketStore)
		marketData.Start(marketCtx)
		market.SetLiveSource(marketData)
		market.ObservePublicResponses(marketData.Client().Limiter().Observe)
	}

	// Create Manager
	traderStores := trader.NewStores(db)
	traderManager := trader.NewManager(traderStores)

	// Load all traders from database to memory (may auto-start traders with Running=true)
	if err := traderManager.LoadTradersFromStore(); err != nil {
		logger.Fatalf("❌ Failed to load traders: %v", err)
	}

	// Start API server
	server := api.NewServer(traderManager, api.NewStores(db), cryptoService, cfg.APIServerPort)

	// Keep the trading ledger up to date with every account in use.
	accountSyncCtx, stopAccountSync := context.WithCancel(context.Background())
	defer stopAccountSync()
	accountSync := syncer.New(db, traderSyncFeatures(traderStores.Traders))
	accountSync.Start(accountSyncCtx)
	server.SetAccountSync(accountSync)

	telegramStores := telegram.NewStores(db)

	var forecastStore *forecast.Store
	if marketData != nil {
		server.SetMarketData(marketData)

		forecastStore = forecast.NewStore(marketStore.DB())
		engine := forecast.NewEngine(forecastStore, marketData, func(ctx context.Context, alert forecast.Alert) error {
			cfg, err := telegramStores.Telegram.Get()
			if err != nil || cfg.BotToken == "" || cfg.ChatID == 0 || cfg.AlertMuted(telegram.AlertForecast) {
				return nil
			}
			return telegram.NotifyForecast(ctx, telegramStores.Telegram, alert)
		})
		engine.Start(marketCtx)
		server.SetForecast(engine)
	}

	// The Dashboard's event feed only reads the database and the market streams' counters.
	activityCtx, stopActivity := context.WithCancel(context.Background())
	defer stopActivity()
	activitySources := dashboard.Sources{Stores: dashboard.NewStores(db), Forecasts: forecastStore, RefusedOrders: server.RefusedPlanOrders}
	if marketData != nil {
		activitySources.Stream = func() dashboard.StreamCount {
			messages, topics, connected := marketData.StreamCounts()
			return dashboard.StreamCount{Messages: messages, Topics: topics, Connected: connected}
		}
	}
	activityFeed := dashboard.New(activitySources)
	activityFeed.Start(activityCtx)
	server.SetActivity(activityFeed)

	// Telegram bot: saving its token, model or binding in Settings restarts it.
	telegramReload := make(chan struct{}, 1)
	server.SetTelegramReloadCh(telegramReload)
	service.Go("telegram-bot", func() { telegram.Start(cfg, telegramStores, telegramReload) })

	// Telegram alerts only read state; they never touch the trading loop.
	alertsCtx, stopAlerts := context.WithCancel(context.Background())
	defer stopAlerts()
	service.Go("telegram-alerts", func() {
		telegram.StartAlerts(alertsCtx, telegramStores, telegram.AlertSources{
			TraderStatus: func(traderID string) map[string]any {
				trader, err := traderManager.GetTrader(traderID)
				if err != nil {
					return nil
				}
				return trader.GetStatus()
			},
			PlanRuns:        server.PlanRunsForAlerts,
			Problems:        server.ProblemsForAlerts,
			FailingAccounts: accountSync.Failing,
			AccountEquity:   server.AccountEquityForAlerts,
		})
	})

	go func() {
		// Shutdown makes Start return http.ErrServerClosed; only other errors
		// (such as the port being taken) are fatal.
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("❌ Failed to start API server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("✅ System started successfully, waiting for trading commands...")
	logger.Info("📌 Tip: Use Ctrl+C to stop the system")

	<-quit
	logger.Info("📴 Shutdown signal received, closing system...")

	if err := server.Shutdown(); err != nil {
		logger.Warnf("⚠️ HTTP server shutdown error: %v", err)
	}
	logger.Info("✅ HTTP server stopped")

	// Stop all traders
	traderManager.StopAll()
	logger.Info("✅ System shut down safely")
}

// traderSyncFeatures tells the account sync what only traders know: the ledger names
// its features by source and imports none of them.
func traderSyncFeatures(traders *trader.Store) syncer.Features {
	return syncer.Features{
		ActiveAccounts: traders.RunningAccounts,
		Owner: func(accountID string) (account.Owner, error) {
			id, err := traders.RunningOn(accountID)
			if err != nil || id == "" {
				return account.Owner{}, err
			}
			return account.Owner{Source: account.SourceTrader, SourceID: id}, nil
		},
	}
}
