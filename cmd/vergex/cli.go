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

	"vergex/internal/auth"
	"vergex/internal/config"
	"vergex/internal/crypto"
	"vergex/internal/forecast"
	"vergex/internal/logger"
	"vergex/internal/marketdata"
	"vergex/internal/store"

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

// openMarketStore opens the market database: its own SQLite file beside the main
// one, or a separate connection pool to the same PostgreSQL database.
func openMarketStore(cfg *config.Config) (*marketdata.Store, error) {
	if cfg.DBType == "postgres" {
		gdb, err := store.InitGormWithConfig(store.DBConfig{
			Type:     store.DBTypePostgres,
			Host:     cfg.DBHost,
			Port:     cfg.DBPort,
			User:     cfg.DBUser,
			Password: cfg.DBPassword,
			DBName:   cfg.DBName,
			SSLMode:  cfg.DBSSLMode,
		})
		if err != nil {
			return nil, err
		}
		return marketdata.NewStore(gdb)
	}
	return marketdata.Open(filepath.Join(filepath.Dir(cfg.DBPath), "market.db"))
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
	options := marketdata.BackfillOptions{
		Symbols:     strings.Split(strings.ToUpper(*symbols), ","),
		Intervals:   strings.Split(*intervals, ","),
		Months:      *months,
		Derivatives: *derivatives,
	}
	err = marketdata.NewService(marketStore).RunBackfill(ctx, options, func(progress marketdata.BackfillProgress) {
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

// runForecastBacktest tests a forecast version on local history and prints a
// report. It only reads the local store. Usage: `vergex forecast-backtest
// --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24`.
func runForecastBacktest(args []string) {
	fs := flag.NewFlagSet("forecast-backtest", flag.ExitOnError)
	symbols := fs.String("symbols", "", "comma-separated symbols (required)")
	intervals := fs.String("intervals", "1h,4h,1d", "comma-separated intervals: 15m, 1h, 4h, 1d")
	months := fs.Int("months", 12, "months back from now")
	versionID := fs.Int64("version", 0, "strategy version; defaults to the active one")
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
	forecasts, err := forecast.NewStore(marketStore.DB())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open forecast tables: %v\n", err)
		os.Exit(1)
	}
	version, err := forecasts.ActiveVersion(time.Now())
	if *versionID != 0 {
		version, err = forecasts.VersionByID(*versionID)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "read strategy version: %v\n", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	engine := forecast.NewEngine(forecasts, marketdata.NewService(marketStore), nil)
	now := time.Now()
	report, err := forecast.RunBacktest(ctx, engine, version.Config, forecast.BacktestOptions{
		Symbols:   strings.Split(strings.ToUpper(*symbols), ","),
		Intervals: strings.Split(*intervals, ","),
		From:      now.AddDate(0, -*months, 0).UnixMilli(),
		To:        now.UnixMilli(),
	}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backtest: %v\n", err)
		os.Exit(1)
	}
	printBacktest(version.ID, report)
}

func printBacktest(versionID int64, report forecast.Report) {
	fmt.Printf("Strategy version %d, %s to %s, fees %.2f%% round trip\n", versionID,
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
func openStoreForCLI(dbPathOverride string) (*store.Store, error) {
	_ = godotenv.Load()
	logger.Init(nil)
	config.MustInit()
	cfg := config.Get()
	if strings.TrimSpace(dbPathOverride) != "" {
		cfg.DBPath = dbPathOverride
	}

	cryptoService, err := crypto.NewCryptoService()
	if err != nil {
		return nil, fmt.Errorf("initialize encryption service: %w", err)
	}
	crypto.SetGlobalCryptoService(cryptoService)

	if cfg.DBType == "sqlite" {
		if dir := filepath.Dir(cfg.DBPath); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create data directory: %w", err)
			}
		}
	}

	dbType := store.DBTypeSQLite
	if cfg.DBType == "postgres" {
		dbType = store.DBTypePostgres
	}
	return store.NewWithConfig(store.DBConfig{
		Type:     dbType,
		Path:     cfg.DBPath,
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		DBName:   cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
}

// runResetPassword resets the password for a single account from the command
// line. Usage: `vergex reset-password --email you@example.com`.
func runResetPassword(args []string) {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	email := fs.String("email", "", "email of the account to reset (required)")
	password := fs.String("password", "", "new password (min 8 chars); omit to enter it interactively")
	dbPath := fs.String("db", "", "override SQLite DB path (defaults to config / DB_PATH)")
	_ = fs.Parse(args)

	if strings.TrimSpace(*email) == "" {
		fmt.Fprintln(os.Stderr, "error: --email is required")
		fmt.Fprintln(os.Stderr, "usage: vergex reset-password --email you@example.com")
		os.Exit(2)
	}

	st, err := openStoreForCLI(*dbPath)
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
	"users", "ai_models", "exchanges", "traders", "strategies",
	"decision_records", "trader_positions", "trader_orders", "trader_fills",
	"trader_equity_snapshots", "ai_charges", "telegram_configs", "telegram_alert_marks",
	"grid_configs", "grid_instances", "grid_levels", "grid_events", "grid_regime_assessments",
	"planning_drafts", "planning_conversations", "planning_preferences",
	"planning_live_runs", "planning_live_leases", "planning_account_closes",
	"system_config",
}

// planningDataDir holds Frontier's per-user entry calculators and data plugin
// settings, relative to the working directory like in api/handler_planning.go.
var planningDataDir = filepath.Join("data", "planning")

const resetAccountWarning = `This wipes Vergex back to a fresh install. It permanently deletes:
  - the account
  - AI models and exchange accounts, API keys included
  - traders, strategies and grid settings
  - decisions, positions, orders, equity history and AI costs
  - Frontier plans, conversations and entry calculators
  - the Telegram bot token and chat binding
Positions and orders on the exchange are not touched.
`

// runResetAccount wipes every Vergex table back to a fresh install. This is the
// destructive "forgot everything" recovery, moved off the public API.
func runResetAccount(args []string) {
	fs := flag.NewFlagSet("reset-account", flag.ExitOnError)
	dbPath := fs.String("db", "", "override SQLite DB path (defaults to config / DB_PATH)")
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

	st, err := openStoreForCLI(*dbPath)
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
			// Frontier creates its tables on first use.
			if !tx.Migrator().HasTable(table) {
				continue
			}
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		return nil
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
