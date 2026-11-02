package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"vergex/internal/agent"
	"vergex/internal/config"
	"vergex/internal/database"
	"vergex/internal/forecast"
	"vergex/internal/futures"
	"vergex/internal/logger"
	"vergex/internal/market/feed"
	"vergex/internal/security/auth"
	"vergex/internal/security/crypto"
	"vergex/internal/user"

	"github.com/joho/godotenv"
	"golang.org/x/term"
	"gorm.io/gorm"
)

// minResetPasswordLen mirrors the minimum enforced on the authenticated
// password-change path (PUT /api/user/password).
const minResetPasswordLen = 8

// runCLISubcommand dispatches local admin subcommands.
//
// SECURITY: user recovery (reset-password, reset-user) is intentionally
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
	case "reset-user":
		runResetUser(args[1:])
		return true
	case "market-backfill":
		runMarketBackfill(args[1:])
		return true
	case "forecast-backtest":
		runForecastBacktest(args[1:])
		return true
	case "prune":
		runPrune(args[1:])
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
		fmt.Fprintf(os.Stderr, "read the strategy: %v\n", err)
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
	fmt.Printf("%-16s %8s %8s %8s %8s %8s %8s %8s %8s %8s %8s\n", "", "samples", "signals", "hit", "chance", "up base", "brier", "lift", "ret", "net ret", "in move")
	rate := func(value *float64) string {
		if value == nil {
			return "—"
		}
		return fmt.Sprintf("%.1f%%", *value*100)
	}
	row := func(name string, m forecast.Metrics) {
		fmt.Printf("%-16s %8d %8d %8s %8s %7.1f%% %8.4f %7.2f%% %7.3f%% %7.3f%% %7.1f%%\n", name, m.Samples, m.Signals, rate(m.HitRate), rate(m.ChanceRate),
			m.BaselineUp*100, m.Brier, m.Lift*100, m.AvgReturnPct, m.AvgReturnAfterFee, m.MoveCoverage*100)
	}
	row("overall", report.Overall)
	fmt.Printf("Derivatives in %.1f%% of samples; Brier %.4f, on candles alone %.4f\n", report.Overall.DerivativeCoverage*100, report.Overall.Brier, report.Overall.BrierCandles)
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
	fmt.Println("\nFactors: how often the sign matched the move, and correlation with it")
	edge := func(e forecast.FactorEdge) string {
		if e.AgreeRate == nil || e.Correlation == nil {
			return "—"
		}
		return fmt.Sprintf("%.1f%% %+.3f", *e.AgreeRate*100, *e.Correlation)
	}
	fmt.Printf("%-16s %16s %16s\n", "", "trending", "ranging")
	for _, factor := range report.Factors {
		fmt.Printf("%-16s %16s %16s\n", factor.Name, edge(factor.Trending), edge(factor.Ranging))
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
func openDatabaseForCLI() (*gorm.DB, error) {
	_ = godotenv.Load()
	logger.Init(nil)
	config.MustInit()

	cryptoService, err := crypto.NewCryptoService()
	if err != nil {
		return nil, fmt.Errorf("initialize encryption service: %w", err)
	}
	crypto.SetGlobalCryptoService(cryptoService)

	return database.OpenMigrated(databaseConfig(config.Get()))
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

	db, err := openDatabaseForCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close(db)
	users := user.NewStore(db)

	owner, err := users.GetByEmail(strings.TrimSpace(*email))
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
	if err := users.UpdatePassword(owner.ID, hash); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to update password: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Password reset for %s. Log in with the new password.\n", owner.Email)
}

// resetTables lists every table Vergex creates. reset-user empties all of
// them; cli_test.go fails when a table is missing here.
var resetTables = []string{
	"futures.account_closes", "futures.run_orders", "futures.runs", "futures.plans", "futures.calculators",
	"trader.decisions", "trader.cycles", "trader.position_peaks", "trader.equity_snapshots", "trader.traders",
	"account.skipped_fills", "account.fills", "account.positions", "account.orders", "account.leases", "core.users", "agent.models", "agent.calls", "agent.conversations", "agent.messages", "account.accounts",
	"telegram.bot", "telegram.alert_marks", "telegram.visitors",
}

const resetUserWarning = `This wipes Vergex back to a fresh install. It permanently deletes:
  - the user
  - models and accounts, API keys included
  - traders and their settings
  - decisions, positions, orders, equity history and agent costs
  - Futures plans, conversations and entry calculators
  - the Telegram bot token and chat binding
Positions and orders on the exchange are not touched.
`

// runResetUser wipes every Vergex table back to a fresh install. This is the
// destructive "forgot everything" recovery, moved off the public API.
func runResetUser(args []string) {
	fs := flag.NewFlagSet("reset-user", flag.ExitOnError)
	yes := fs.Bool("yes", false, "skip the interactive confirmation prompt")
	_ = fs.Parse(args)

	if !*yes {
		fmt.Print(resetUserWarning + "Type 'wipe' to confirm: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "wipe" {
			fmt.Fprintln(os.Stderr, "aborted")
			os.Exit(1)
		}
	}

	db, err := openDatabaseForCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close(db)

	if err := wipeAllData(db); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to reset user: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Vergex wiped. Restart the backend so running traders stop, then open\n" +
		"  the web app to create the user again.")
}

// parseAge reads an age such as 90d, or anything time.ParseDuration takes. It must be at
// least a day, so a mistyped unit cannot empty what is in use.
func parseAge(text string) (time.Duration, error) {
	age, err := time.ParseDuration(text)
	if strings.HasSuffix(text, "d") {
		var days int
		days, err = strconv.Atoi(strings.TrimSuffix(text, "d"))
		age = time.Duration(days) * 24 * time.Hour
	}
	if err != nil {
		return 0, fmt.Errorf("%q is not an age such as 90d", text)
	}
	if age < 24*time.Hour {
		return 0, fmt.Errorf("%q is under a day", text)
	}
	return age, nil
}

// pruneBatch is how many messages one delete takes.
const pruneBatch = 5000

// runPrune removes what the models were sent and said, once it is old. By default it only
// reports. Usage: `vergex prune --before 90d [--yes]`.
func runPrune(args []string) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	before := fs.String("before", "", "how old a message must be, such as 90d")
	yes := fs.Bool("yes", false, "delete them rather than only reporting")
	_ = fs.Parse(args)

	age, err := parseAge(*before)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: --before: %v\n", err)
		os.Exit(1)
	}
	db, err := openDatabaseForCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close(db)

	cutoff := time.Now().Add(-age)
	store := agent.NewConversationStore(db)
	count, size, err := store.MessagesBefore(cutoff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%d messages from before %s, holding %.1f MB of text.\n", count, cutoff.Format("2006-01-02 15:04"), float64(size)/(1<<20))
	if !*yes {
		fmt.Println("Nothing was deleted. Run again with --yes to delete them.")
		return
	}
	deleted, err := store.DeleteMessagesBefore(cutoff, pruneBatch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: stopped after deleting %d messages: %v\n", deleted, err)
		os.Exit(1)
	}
	fmt.Printf("✓ Deleted %d messages. PostgreSQL reuses the space for new ones; the files on disk do not shrink.\n", deleted)
}

// wipeAllData empties every table in resetTables.
func wipeAllData(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range resetTables {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		// Market and forecast data outlive an account, and so do their settings and
		// forecast strategies.
		return tx.Exec("DELETE FROM core.settings WHERE module = ?", futures.SettingsModule).Error
	})
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
