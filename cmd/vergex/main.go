package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"vergex/internal/activity"
	"vergex/internal/api"
	"vergex/internal/auth"
	"vergex/internal/config"
	"vergex/internal/crypto"
	"vergex/internal/forecast"
	"vergex/internal/llm"
	_ "vergex/internal/llm/provider"
	"vergex/internal/logger"
	"vergex/internal/manager"
	"vergex/internal/market"
	"vergex/internal/marketdata"
	"vergex/internal/pricing"
	"vergex/internal/safe"
	"vergex/internal/store"
	"vergex/internal/telegram"

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
	logger.Info("║          🚀 Vergex - AI-Powered Trading System             ║")
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
	st, err := store.New(databaseConfig(cfg))
	if err != nil {
		logger.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer st.Close()

	// AI cost tracking: record token usage of every AI call in ai_charges,
	// priced from the models.dev catalog cached in the data directory.
	pricingCtx, stopPricing := context.WithCancel(context.Background())
	defer stopPricing()
	prices := pricing.NewService("data")
	prices.Start(pricingCtx)
	llm.TokenUsageCallback = recordAIUsage(st, prices)

	// Set JWT secret
	auth.SetJWTSecret(cfg.JWTSecret)
	logger.Info("🔑 JWT secret configured")

	// Market data comes up before any trader can start. A failure here leaves the
	// rest of the system running: trading reads the exchange directly without it.
	marketCtx, stopMarket := context.WithCancel(context.Background())
	defer stopMarket()
	var marketData *marketdata.Service
	marketStore, err := openMarketStore(cfg)
	if err != nil {
		logger.Errorf("Market data is unavailable: %v", err)
	} else {
		defer marketStore.Close()
		marketData = marketdata.NewService(marketStore)
		marketData.Start(marketCtx)
		market.SetLiveSource(marketData)
	}

	// Create TraderManager
	traderManager := manager.NewTraderManager()

	// Load all traders from database to memory (may auto-start traders with IsRunning=true)
	if err := traderManager.LoadTradersFromStore(st); err != nil {
		logger.Fatalf("❌ Failed to load traders: %v", err)
	}

	// Display loaded trader information
	traders, err := st.Trader().List("default")
	if err != nil {
		logger.Fatalf("❌ Failed to get trader list: %v", err)
	}

	logger.Info("🤖 AI Trader Configurations in Database:")
	if len(traders) == 0 {
		logger.Info("  (No trader configurations, please create via Web interface)")
	} else {
		for _, t := range traders {
			status := "❌ Stopped"
			if t.IsRunning {
				status = "✅ Running"
			}
			idShort := t.ID
			if len(idShort) > 8 {
				idShort = idShort[:8]
			}
			logger.Infof("  • %s [%s] %s - AI Model: %s, Exchange: %s",
				t.Name, idShort, status, t.AIModelID, t.ExchangeID)
		}
	}

	// Start API server
	server := api.NewServer(traderManager, st, cryptoService, cfg.APIServerPort)

	var forecastStore *forecast.Store
	if marketData != nil {
		server.SetMarketData(marketData)

		forecasts, err := forecast.NewStore(marketStore.DB())
		if err != nil {
			logger.Errorf("Forecast is unavailable: %v", err)
		} else {
			engine := forecast.NewEngine(forecasts, marketData, func(ctx context.Context, text string) error {
				cfg, err := st.TelegramConfig().Get()
				if err != nil || cfg.AlertMuted(telegram.AlertForecast) {
					return nil
				}
				return telegram.Notify(ctx, st, text)
			})
			engine.Start(marketCtx)
			server.SetForecast(engine)
			forecastStore = forecasts
		}
	}

	// The Dashboard's event feed only reads the database.
	activityCtx, stopActivity := context.WithCancel(context.Background())
	defer stopActivity()
	feed := activity.New(activity.Sources{Store: st, Forecasts: forecastStore, PlanRuns: server.PlanRunsChanged})
	feed.Start(activityCtx)
	server.SetActivity(feed)

	// Telegram bot: saving its token, model or binding in Settings restarts it.
	telegramReload := make(chan struct{}, 1)
	server.SetTelegramReloadCh(telegramReload)
	safe.GoNamed("telegram-bot", func() { telegram.Start(cfg, st, telegramReload) })

	// Telegram alerts only read state; they never touch the trading loop.
	alertsCtx, stopAlerts := context.WithCancel(context.Background())
	defer stopAlerts()
	safe.GoNamed("telegram-alerts", func() {
		telegram.StartAlerts(alertsCtx, st, telegram.AlertSources{
			TraderStatus: func(traderID string) map[string]any {
				trader, err := traderManager.GetTrader(traderID)
				if err != nil {
					return nil
				}
				return trader.GetStatus()
			},
			PlanRuns: server.PlanRunsForAlerts,
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
