package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/account/syncer"
	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/user"
)

// trendTrader is the trader every alert test starts with.
const trendTrader = "00000000-0000-4000-8000-0000000000a1"

func alertFixture(t *testing.T) (*alertWatcher, *Stores, *[]string, *time.Time) {
	t.Helper()
	st := NewStores(testdb.Open(t))
	if err := st.Users.Create(&user.User{ID: testdb.UserID, Email: "alice@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	testdb.AddTrader(t, st.DB, &testdb.Trader{ID: trendTrader, Name: "Trend"})
	if err := st.Telegram.Save("token", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Telegram.BindUser(42, "alice"); err != nil {
		t.Fatal(err)
	}
	sent := &[]string{}
	now := time.Now()
	clock := &now
	w := &alertWatcher{
		stores: st,
		send:   func(_ context.Context, text string) error { *sent = append(*sent, text); return nil },
		now:    func() time.Time { return *clock },
	}
	return w, st, sent, clock
}

func closePosition(t *testing.T, st *Stores, exit time.Time) {
	t.Helper()
	ledger := accounttest.NewLedger(t, st.DB)
	ledger.Open(trendTrader, "BTCUSDT", account.SideLong, 1, 100, exit.Add(-time.Minute))
	ledger.Close(trendTrader, "BTCUSDT", account.SideLong, 1, 112.5, 12.5, exit)
}

func TestAlertsSkipHistoryAndReportOnce(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	closePosition(t, st, clock.Add(-time.Hour))

	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("first check replayed history: %v", *sent)
	}

	*clock = clock.Add(time.Minute)
	closePosition(t, st, clock.Add(-time.Second))
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Position closed: BTCUSDT long, Trend, P/L +12.50 USDT") {
		t.Fatalf("sent = %v", *sent)
	}

	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	if len(*sent) != 1 {
		t.Fatalf("repeated the alert: %v", *sent)
	}
}

func TestAlertsRetryAfterAFailedSend(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	w.check(context.Background())

	*clock = clock.Add(time.Minute)
	closePosition(t, st, clock.Add(-time.Second))
	working := w.send
	w.send = func(context.Context, string) error { return errors.New("down") }
	w.check(context.Background())

	w.send = working
	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	if len(*sent) != 1 {
		t.Fatalf("the event was lost or doubled: %v", *sent)
	}
}

func TestStandingProblemsAlertOnceAndReArm(t *testing.T) {
	w, _, sent, clock := alertFixture(t)
	safeMode := true
	w.sources.TraderStatus = func(string) map[string]any {
		return map[string]any{"safe_mode": safeMode, "safe_mode_reason": "3 model failures"}
	}
	w.check(context.Background())
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Safe mode: Trend: 3 model failures") {
		t.Fatalf("sent = %v", *sent)
	}

	safeMode = false
	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	safeMode = true
	w.check(context.Background())
	if len(*sent) != 2 {
		t.Fatalf("did not re-arm: %v", *sent)
	}
}

func TestMutedKindsStaySilentWithoutBuildingABacklog(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	w.check(context.Background())
	if err := st.Telegram.SetMutedAlerts([]string{AlertPositionClosed}); err != nil {
		t.Fatal(err)
	}

	*clock = clock.Add(time.Minute)
	closePosition(t, st, clock.Add(-time.Second))
	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("muted kind was sent: %v", *sent)
	}

	if err := st.Telegram.SetMutedAlerts(nil); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("turning it back on replayed: %v", *sent)
	}
}

func TestPlansNeedingAttentionAlertOncePerRun(t *testing.T) {
	w, _, sent, _ := alertFixture(t)
	runs := []PlanRun{{ID: "r1", Symbol: "ETHUSDT", State: "needs_attention", Error: "Service restarted"}, {ID: "r2", Symbol: "BTCUSDT", State: "running"}}
	w.sources.PlanRuns = func(context.Context, string) ([]PlanRun, error) { return runs, nil }
	w.check(context.Background())
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Futures plan needs attention: ETHUSDT") {
		t.Fatalf("sent = %v", *sent)
	}
}

func TestProblemsAlertOnceWhileTheyLast(t *testing.T) {
	w, _, sent, clock := alertFixture(t)
	problems := []Problem{{Key: "service_failed:forecast", Kind: ProblemService, Name: "forecast", Detail: "no strategy"}}
	w.sources.Problems = func(string) []Problem { return problems }
	w.check(context.Background())
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Service failed: forecast: no strategy") {
		t.Fatalf("sent = %v", *sent)
	}

	// Once it has cleared, it is told again if it comes back.
	problems = nil
	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	problems = []Problem{{Key: "service_failed:forecast", Kind: ProblemService, Name: "forecast"}}
	w.check(context.Background())
	if len(*sent) != 2 {
		t.Fatalf("did not re-arm: %v", *sent)
	}
}

func TestAccountsAlertOnlyOnceTheyHaveKeptFailing(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	id := testdb.AddAccount(t, st.DB, testdb.UserID, "binance", "Main")
	failing := map[string]syncer.Failure{id: {Since: *clock, Reason: "timeout"}}
	w.sources.FailingAccounts = func() map[string]syncer.Failure { return failing }

	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("told of a passing failure: %v", *sent)
	}
	*clock = clock.Add(accountFailingFor)
	w.check(context.Background())
	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Account unreadable: Main: timeout") {
		t.Fatalf("sent = %v", *sent)
	}

	// Muting the kind keeps it quiet.
	failing = map[string]syncer.Failure{}
	w.check(context.Background())
	if err := st.Telegram.SetMutedAlerts([]string{AlertSystem}); err != nil {
		t.Fatal(err)
	}
	failing = map[string]syncer.Failure{id: {Since: clock.Add(-time.Hour), Reason: "timeout"}}
	w.check(context.Background())
	if len(*sent) != 1 {
		t.Fatalf("muted kind spoke: %v", *sent)
	}
}

func TestDailyReportGoesOutOnceADayForRealAccounts(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	*clock = time.Date(2026, 11, 2, 23, 50, 0, 0, time.UTC)
	ledger := accounttest.NewLedger(t, st.DB)
	testnet, err := st.Accounts.Create(testdb.UserID, "binance", "Practice", "testnet-key", "secret", "", true)
	if err != nil {
		t.Fatal(err)
	}
	w.sources.AccountEquity = func(_, id string) (float64, bool) {
		switch id {
		case testnet:
			t.Error("asked a testnet account for its equity")
		case ledger.Account:
			return 1234.5, true
		}
		return 0, true
	}
	if err := st.DB.Create(&agent.Call{Feature: agent.FeatureTrader, SourceID: trendTrader, Model: "m", Provider: "p", Host: "h", CostUSD: 0.42, CreatedAt: clock.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}

	// The first check ever only starts the clock.
	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("first check reported: %v", *sent)
	}

	ledger.Open(trendTrader, "BTCUSDT", account.SideLong, 1, 100, clock.Add(-2*time.Hour))
	ledger.Close(trendTrader, "BTCUSDT", account.SideLong, 1, 110, 10, clock.Add(-time.Hour))
	ledger.Open(trendTrader, "ETHUSDT", account.SideShort, 1, 100, clock.Add(-time.Hour))
	// A close on test money counts for nothing.
	if err := st.DB.Exec("INSERT INTO account.positions (account_id, instrument_id, source, side, status, entry_quantity, quantity, entry_price, realized_pnl, fee, opened_at, closed_at) SELECT ?, instrument_id, source, side, 'closed', 1, 0, 100, 500, 0, opened_at, closed_at FROM account.positions WHERE status = 'closed' LIMIT 1", testnet).Error; err != nil {
		t.Fatal(err)
	}

	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("reported within the day: %v", *sent)
	}
	*clock = clock.Add(20 * time.Minute)
	w.check(context.Background())
	if len(*sent) != 1 {
		t.Fatalf("sent = %v", *sent)
	}
	for _, want := range []string{"Daily report, 2026-11-02 UTC", "Equity: 1234.50 USDT", "Realized: +10.00 USDT", "Open positions: 1 (ETHUSDT short)", "Model cost: $0.42"} {
		if !strings.Contains((*sent)[0], want) {
			t.Errorf("no %q in %q", want, (*sent)[0])
		}
	}

	// The same day does not go out again, even after a restart, which reads the same mark.
	w.check(context.Background())
	restarted := &alertWatcher{stores: st, sources: w.sources, send: w.send, now: w.now}
	restarted.check(context.Background())
	if len(*sent) != 1 {
		t.Fatalf("repeated: %v", *sent)
	}

	// A failed send goes out next time, and days away report the last day only.
	*clock = clock.Add(72 * time.Hour)
	fail := true
	send := w.send
	w.send = func(ctx context.Context, text string) error {
		if fail {
			return errors.New("offline")
		}
		return send(ctx, text)
	}
	w.check(context.Background())
	fail = false
	w.check(context.Background())
	if len(*sent) != 2 || !strings.Contains((*sent)[1], "2026-11-05 UTC") {
		t.Fatalf("sent = %v", *sent)
	}

	// Turned off, it says nothing and leaves nothing to catch up on.
	if err := st.Telegram.SetMutedAlerts([]string{AlertDailyReport}); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(24 * time.Hour)
	w.check(context.Background())
	if err := st.Telegram.SetMutedAlerts(nil); err != nil {
		t.Fatal(err)
	}
	w.check(context.Background())
	if len(*sent) != 2 {
		t.Fatalf("muted report spoke: %v", *sent)
	}
}

func TestDailyReportNeedsARealAccount(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	w.check(context.Background())
	if err := st.DB.Exec("UPDATE account.accounts SET testnet = true").Error; err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(25 * time.Hour)
	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("reported on test money only: %v", *sent)
	}
}

func TestFuturesPositionsAlertWhenTheyFillAndClose(t *testing.T) {
	w, st, sent, clock := alertFixture(t)
	w.check(context.Background())
	asFutures := func() {
		t.Helper()
		if err := st.DB.Exec("UPDATE account.positions SET source = ?", account.SourceFutures).Error; err != nil {
			t.Fatal(err)
		}
	}

	ledger := accounttest.NewLedger(t, st.DB)
	*clock = clock.Add(time.Minute)
	ledger.Open("run", "BTCUSDT", account.SideLong, 0.5, 100, clock.Add(-time.Second))
	asFutures()
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Entry filled: BTCUSDT long, 0.5 at 100, Futures") {
		t.Fatalf("sent = %v", *sent)
	}

	*clock = clock.Add(time.Minute)
	ledger.Close("run", "BTCUSDT", account.SideLong, 0.5, 112.5, -3.25, clock.Add(-time.Second))
	w.check(context.Background())
	if len(*sent) != 2 || !strings.Contains((*sent)[1], "Position closed: BTCUSDT long, Futures, P/L -3.25 USDT") {
		t.Fatalf("sent = %v", *sent)
	}

	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	if len(*sent) != 2 {
		t.Fatalf("repeated the alerts: %v", *sent)
	}
}

func TestSplitMessageKeepsLinesWithinTheLimit(t *testing.T) {
	line := strings.Repeat("x", 2000)
	parts := splitMessage(line + "\n" + line + "\n" + line)
	if len(parts) != 3 {
		t.Fatalf("parts = %d", len(parts))
	}
}

func TestDailyReportSaysWhenAnAccountCannotBeRead(t *testing.T) {
	w, _, _, clock := alertFixture(t)
	w.sources.AccountEquity = func(_, _ string) (float64, bool) { return 0, false }
	day := clock.UTC().Truncate(24 * time.Hour)
	report, ok := w.dailyReport(testdb.UserID, day, day.Add(24*time.Hour), "en")
	if !ok || !strings.Contains(report, "Equity: —") {
		t.Fatalf("report = %q, %v", report, ok)
	}
}
