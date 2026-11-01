package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"vergex/internal/config"
	"vergex/internal/forecast"
	"vergex/internal/logger"
	"vergex/internal/market/feed"
	"vergex/internal/planning"
	"vergex/internal/security/auth"
	"vergex/internal/security/crypto"
	"vergex/internal/store"
	"vergex/internal/store/database"

	"github.com/joho/godotenv"
	"golang.org/x/term"
	"gorm.io/gorm"
)

// minResetPasswordLen mirrors the minimum enforced on the authenticated
// password-change path (PUT /api/user/password).
const minResetPasswordLen = 8

// runCLISubcommand dispatches local admin subcommands.
//
// SECURITY: account recovery (reset-password, reset-account) is intentionally
// NOT exposed over HTTP. Performing it requires running this binary on the host,
// which in turn requires shell/file access to the server. A remote attacker on a
// public-facing deployment has only the network — they can reach the API but not
// a local process — so recovery cannot be triggered remotely. This is what makes
// the recovery path safe even when Vergex is deployed on the public internet.
//
// Returns true if a subcommand was recognized and handled (caller should exit).
func runCLISubcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "reset-password":
		runResetPassword(args[1:])
		return true
	case "reset-account":
		runResetAccount(args[1:])
		return true
	case "market-backfill":
		runMarketBackfill(args[1:])
		return true
	case "forecast-backtest":
		runForecastBacktest(args[1:])
		return true
	default:
		return false
	}
}

// marketPoolSize bounds the market tables' connection pool, kept apart from the
// main one so bulk candle writes never queue up orders and positions.
const marketPoolSize = 10

func databaseConfig(cfg *config.Config) database.Config {
	return database.Config{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		DBName:   cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	}
}

// openMarketStore opens the market tables on their own connection pool.
func openMarketStore(cfg *config.Config) (*feed.Store, error) {
	gdb, err := database.Open(databaseConfig(cfg))
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(gdb); err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(marketPoolSize)
	return feed.NewStore(gdb), nil
}

// runMarketBackfill loads deep history from Binance's published files. Usage:
// `vergex market-backfill --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24`.
func runMarketBackfill(args []string) {
	fs := flag.NewFlagSet("market-backfill", flag.ExitOnError)
	symbols := fs.String("symbols", "", "comma-separated symbols (required)")
	intervals := fs.String("intervals", "1h,4h,1d", "comma-separated intervals: 15m, 1h, 4h, 1d")
	months := fs.Int("months", 12, "whole months to load")
	derivatives := fs.Bool("derivatives", false, "also load funding, open interest and long/short history")
	_ = fs.Parse(args)
	if strings.TrimSpace(*symbols) == "" {
		fs.Usage()
		os.Exit(2)
	}

	_ = godotenv.Load()
	logger.Init(nil)
	config.MustInit()
	marketStore, err := openMarketStore(config.Get())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open market database: %v\n", err)
		os.Exit(1)
	}
	defer marketStore.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	options := feed.BackfillOptions{
		Symbols:     strings.Split(strings.ToUpper(*symbols), ","),
		Intervals:   strings.Split(*intervals, ","),
		Months:      *months,
		Derivatives: *derivatives,
	}
	err = feed.NewService(marketStore).RunBackfill(ctx, options, func(progress feed.BackfillProgress) {
		if progress.Current != "" {
			fmt.Printf("[%d/%d] %s\n", progress.Done, progress.Total, progress.Current)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "backfill: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}

// runForecastBacktest tests a forecast strategy on local history and prints a
// report. It only reads the local store. Usage: `vergex forecast-backtest
// --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24`.
func runForecastBacktest(args []string) {
	fs := flag.NewFlagSet("forecast-backtest", flag.ExitOnError)
	symbols := fs.String("symbols", "", "comma-separated symbols (required)")
	intervals := fs.String("intervals", "1h,4h,1d", "comma-separated intervals: 15m, 1h, 4h, 1d")
	months := fs.Int("months", 12, "months back from now")
	strategyID := fs.String("strategy", "", "strategy id; defaults to the one in use")
	_ = fs.Parse(args)
	if strings.TrimSpace(*symbols) == "" {
		fs.Usage()
		os.Exit(2)
	}

	_ = godotenv.Load()
	logger.Init(nil)
	config.MustInit()
	marketStore, err := openMarketStore(config.Get())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open market database: %v\n", err)
		os.Exit(1)
	}
	defer marketStore.Close()
	forecasts := forecast.NewStore(marketStore.DB())
	strategy, err := forecasts.ActiveStrategy(time.Now())
	if *strategyID != "" {
		strategy, err = forecasts.StrategyByID(*strategyID)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "read strategy strategy: %v\n", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	engine := forecast.NewEngine(forecasts, feed.NewService(marketStore), nil)
	now := time.Now()
	report, err := forecast.RunBacktest(ctx, engine, strategy.Config, forecast.BacktestOptions{
		Symbols:   strings.Split(strings.ToUpper(*symbols), ","),
		Intervals: strings.Split(*intervals, ","),
		From:      now.AddDate(0, -*months, 0).UnixMilli(),
		To:        now.UnixMilli(),
	}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backtest: %v\n", err)
		os.Exit(1)
	}
	printBacktest(strategy.ID, report)
}

func printBacktest(strategyID string, report forecast.Report) {
	fmt.Printf("Strategy %s, %s to %s, fees %.2f%% round trip\n", strategyID,
		time.UnixMilli(report.FirstTime).UTC().Format("2006-01-02"), time.UnixMilli(report.LastTime).UTC().Format("2006-01-02"), report.FeePct)
	fmt.Printf("%-16s %8s %8s %8s %8s %8s %8s %8s %8s %8s\n", "", "samples", "signals", "hit", "up base", "brier", "lift", "ret", "net ret", "in move")
	row := func(name string, m forecast.Metrics) {
		hit := "—"
		if m.HitRate != nil {
			hit = fmt.Sprintf("%.1f%%", *m.HitRate*100)
		}
		fmt.Printf("%-16s %8d %8d %8s %7.1f%% %8.4f %7.2f%% %7.3f%% %7.3f%% %7.1f%%\n", name, m.Samples, m.Signals, hit,
			m.BaselineUp*100, m.Brier, m.Lift*100, m.AvgReturnPct, m.AvgReturnAfterFee, m.MoveCoverage*100)
	}
	row("overall", report.Overall)
	for _, group := range []map[string]forecast.Metrics{report.ByInterval, report.ByRegime, report.BySymbol} {
		fmt.Println()
		names := make([]string, 0, len(group))
		for name := range group {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			row(name, group[name])
		}
	}
	fmt.Println("\nReliability")
	for _, bucket := range report.Reliability {
		fmt.Printf("%.1f-%.1f %8d samples, said %.1f%%, rose %.1f%%\n", bucket.From, bucket.To, bucket.Samples, bucket.AvgProb*100, bucket.UpRate*100)
	}
	for _, skipped := range report.Skipped {
		fmt.Println("Skipped:", skipped)
	}
}

// openStoreForCLI loads config + encryption and opens the same database the
// server uses, so subcommands operate on the live data.
func openStoreForCLI() (*store.Store, error) {
	_ = godotenv.Load()
	logger.Init(nil)
	config.MustInit()

	cryptoService, err := crypto.NewCryptoService()
	if err != nil {
		return nil, fmt.Errorf("initialize encryption service: %w", err)
	}
	crypto.SetGlobalCryptoService(cryptoService)

	return store.New(databaseConfig(config.Get()))
}

// runResetPassword resets the password for a single account from the command
// line. Usage: `vergex reset-password --email you@example.com`.
func runResetPassword(args []string) {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	email := fs.String("email", "", "email of the account to reset (required)")
	password := fs.String("password", "", "new password (min 8 chars); omit to enter it interactively")
	_ = fs.Parse(args)

	if strings.TrimSpace(*email) == "" {
		fmt.Fprintln(os.Stderr, "error: --email is required")
		fmt.Fprintln(os.Stderr, "usage: vergex reset-password --email you@example.com")
		os.Exit(2)
	}

	st, err := openStoreForCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	user, err := st.User().GetByEmail(strings.TrimSpace(*email))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: no account found for %q\n", strings.TrimSpace(*email))
		os.Exit(1)
	}

	newPassword, err := resolveNewPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to hash password: %v\n", err)
		os.Exit(1)
	}
	if err := st.User().UpdatePassword(user.ID, hash); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to update password: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Password reset for %s. Log in with the new password.\n", user.Email)
}

// resetTables lists every table Vergex creates. reset-account empties all of
// them; cli_test.go fails when a table is missing here.
var resetTables = []string{
	"frontier.account_closes", "frontier.run_orders", "frontier.runs", "frontier.plans",
	"trader.decisions", "trader.cycles", "trader.equity_snapshots", "trader.traders",
	"trading.skipped_fills", "trading.fills", "trading.positions", "trading.orders", "trading.account_leases", "core.users", "agent.models", "agent.calls", "agent.conversations", "agent.messages", "trading.accounts",
	"telegram.bot", "telegram.alert_marks",
}

// planningDataDir holds Frontier's per-user entry calculators and data plugin
// settings, relative to the working directory like in api/handler_planning.go.
var planningDataDir = filepath.Join("data", "planning")

const resetAccountWarning = `This wipes Vergex back to a fresh install. It permanently deletes:
  - the account
  - models and exchange accounts, API keys included
  - traders, strategies and grid settings
  - decisions, positions, orders, equity history and agent costs
  - Frontier plans, conversations and entry calculators
  - the Telegram bot token and chat binding
Positions and orders on the exchange are not touched.
`

// runResetAccount wipes every Vergex table back to a fresh install. This is the
// destructive "forgot everything" recovery, moved off the public API.
func runResetAccount(args []string) {
	fs := flag.NewFlagSet("reset-account", flag.ExitOnError)
	yes := fs.Bool("yes", false, "skip the interactive confirmation prompt")
	_ = fs.Parse(args)

	if !*yes {
		fmt.Print(resetAccountWarning + "Type 'wipe' to confirm: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "wipe" {
			fmt.Fprintln(os.Stderr, "aborted")
			os.Exit(1)
		}
	}

	st, err := openStoreForCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := wipeAllData(st, planningDataDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to reset account: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Vergex wiped. Restart the backend so running traders stop, then open\n" +
		"  the web app to create the account again.")
}

// wipeAllData empties every table in resetTables and removes Frontier's files.
func wipeAllData(st *store.Store, planningDir string) error {
	err := st.Transaction(func(tx *gorm.DB) error {
		for _, table := range resetTables {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		// Market and forecast data outlive an account, and so do their settings and
		// forecast strategies. Trading strategies went with the user they belong to.
		return tx.Exec("DELETE FROM core.settings WHERE module = ?", planning.SettingsModule).Error
	})
	if err != nil {
		return err
	}
	if err := os.RemoveAll(planningDir); err != nil {
		return fmt.Errorf("remove %s: %w", planningDir, err)
	}
	return nil
}

// resolveNewPassword returns the new password from the --password flag, or
// prompts for it (hidden) on a TTY, or reads a single line from piped stdin.
func resolveNewPassword(flagValue string) (string, error) {
	if flagValue != "" {
		if len(flagValue) < minResetPasswordLen {
			return "", fmt.Errorf("password must be at least %d characters", minResetPasswordLen)
		}
		return flagValue, nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Printf("New password (min %d chars): ", minResetPasswordLen)
		first, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Print("Confirm new password: ")
		second, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		if string(first) != string(second) {
			return "", errors.New("passwords do not match")
		}
		if len(first) < minResetPasswordLen {
			return "", fmt.Errorf("password must be at least %d characters", minResetPasswordLen)
		}
		return string(first), nil
	}

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", fmt.Errorf("no password provided on stdin: %w", err)
	}
	if len(password) < minResetPasswordLen {
		return "", fmt.Errorf("password must be at least %d characters", minResetPasswordLen)
	}
	return password, nil
}
