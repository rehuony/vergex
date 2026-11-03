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

// sentAlert is one part the watcher sent, as its plain text reads.
type sentAlert struct {
	text   string
	silent bool
}

func (a sentAlert) String() string { return a.text }

func alertFixture(t *testing.T) (*alertWatcher, *Stores, *[]sentAlert, *time.Time) {
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
	sent := &[]sentAlert{}
	now := time.Now()
	clock := &now
	w := &alertWatcher{
		stores: st,
		send: func(_ context.Context, part messagePart, silent bool) error {
			*sent = append(*sent, sentAlert{part.Plain, silent})
			return nil
		},
		now: func() time.Time { return *clock },
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
	if len(*sent) != 1 || !strings.Contains((*sent)[0].text, "Position closed · BTCUSDT long\nTrend · Real money\n• P/L: +12.50") {
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
	w.send = func(context.Context, messagePart, bool) error { return errors.New("down") }
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
	if len(*sent) != 1 || !strings.Contains((*sent)[0].text, "🚨 Needs attention\n• Safe mode · Trend: 3 model failures") {
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
	runs := []PlanRun{{ID: "r1", Symbol: "ETHUSDT", Status: "needs_attention", Error: "Service restarted"}, {ID: "r2", Symbol: "BTCUSDT", Status: "running"}}
	w.sources.PlanRuns = func(context.Context, string) ([]PlanRun, error) { return runs, nil }
	w.check(context.Background())
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0].text, "• Futures plan needs attention · ETHUSDT: Service restarted") {
		t.Fatalf("sent = %v", *sent)
	}
}

func TestProblemsAlertOnceWhileTheyLast(t *testing.T) {
	w, _, sent, clock := alertFixture(t)
	problems := []Problem{{Key: "service_failed:forecast", Kind: ProblemService, Name: "forecast", Detail: "no strategy"}}
	w.sources.Problems = func(string) []Problem { return problems }
	w.check(context.Background())
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0].text, "• Service failed · forecast: no strategy") {
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
	if len(*sent) != 1 || !strings.Contains((*sent)[0].text, "• Account unreadable · Main: timeout") {
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
	for _, want := range []string{"🗓 Daily report · 11-02 · +10.00 USDT", "Real money only · UTC", "• Equity: 1.2K", "• Realized: +10.00", "• Open positions: 1 (ETHUSDT short)", "• Model cost, all calls: $0.42"} {
		if !strings.Contains((*sent)[0].text, want) {
			t.Errorf("no %q in %q", want, (*sent)[0].text)
		}
	}
	if !(*sent)[0].silent {
		t.Error("the daily report should arrive silently")
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
	w.send = func(ctx context.Context, part messagePart, silent bool) error {
		if fail {
			return errors.New("offline")
		}
		return send(ctx, part, silent)
	}
	w.check(context.Background())
	fail = false
	w.check(context.Background())
	if len(*sent) != 2 || !strings.Contains((*sent)[1].text, "Daily report · 11-05") {
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
	if len(*sent) != 1 || !strings.Contains((*sent)[0].text, "📥 Entry filled · BTCUSDT long\nFutures · Real money\n• Filled: 0.5 @ 100") {
		t.Fatalf("sent = %v", *sent)
	}

	*clock = clock.Add(time.Minute)
	ledger.Close("run", "BTCUSDT", account.SideLong, 0.5, 112.5, -3.25, clock.Add(-time.Second))
	w.check(context.Background())
	if len(*sent) != 2 || !strings.Contains((*sent)[1].text, "📤 Position closed · BTCUSDT long\nFutures · Real money\n• P/L: -3.25") {
		t.Fatalf("sent = %v", *sent)
	}

	*clock = clock.Add(time.Minute)
	w.check(context.Background())
	if len(*sent) != 2 {
		t.Fatalf("repeated the alerts: %v", *sent)
	}
}

func TestADigestPutsEachKindInASectionOfItsOwn(t *testing.T) {
	p := &pending{lang: "en"}
	p.attention = []text{attentionLine("en", "Safe mode", "Trend", "")}
	p.orders = []orderEvent{{symbol: "BTCUSDT", what: "buy", reason: "margin too low", origin: origin{name: "Trend"}}}
	p.closes = []positionEvent{
		{symbol: "BTCUSDT", side: "long", pnl: 12.3, origin: origin{name: "Trend"}},
		{symbol: "SOLUSDT", side: "short", pnl: -3.1, origin: origin{name: "Futures", testnet: true}, late: "11-02 09:00 UTC"},
	}
	parts := p.digest().parts()
	if len(parts) != 1 {
		t.Fatalf("parts = %d", len(parts))
	}
	got := parts[0].Plain
	for _, want := range []string{
		"🚨 Needs attention\n• Safe mode · Trend\n\n❌ Order failed · BTCUSDT buy\nTrend · Real money\nmargin too low",
		"📤 2 positions closed\nSince 11-02 09:00 UTC\nClosed\nSymbol",
		"BTCUSDT  long  +12.30  Trend\n",
		// Test money is a table of its own, so no total mixes the two.
		"Closed · Testnet\nSymbol", "SOLUSDT  short  -3.10  Futures",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	wellFormed(t, parts[0].HTML)
	if !strings.Contains(parts[0].HTML, `<blockquote expandable>margin too low</blockquote>`) || !strings.Contains(parts[0].HTML, `<code class="language-Closed">`) || !strings.Contains(parts[0].HTML, `<code class="language-Closed·Testnet">`) {
		t.Fatalf("html = %s", parts[0].HTML)
	}
}

func TestAMessageFailingPartWayResendsOnlyTheRest(t *testing.T) {
	line := strings.Repeat("x", 3000)
	m := newMessage("en").add(words(line)).add(words(line + "1")).add(words(line + "2"))
	var sent []string
	failing := true
	w := &alertWatcher{send: func(_ context.Context, part messagePart, _ bool) error {
		if failing && len(sent) == 1 {
			return errors.New("down")
		}
		sent = append(sent, part.Plain)
		return nil
	}}
	if err := w.deliver(context.Background(), m); err == nil {
		t.Fatal("expected the failure to be reported")
	}
	failing = false
	if err := w.deliver(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 3 {
		t.Fatalf("%d parts went out, want each of 3 once", len(sent))
	}
	// A later message that happens to repeat a part goes out in full.
	if err := w.deliver(context.Background(), newMessage("en").add(words(line))); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 4 {
		t.Fatal("what was delivered before should not hold up a new message")
	}
}

func TestAnEventReportedLateSaysWhenItHappened(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	w := &alertWatcher{now: func() time.Time { return now }}
	if got := w.late(now.Add(-time.Minute).UnixMilli(), now.UnixMilli()); got != "" {
		t.Fatalf("a recent event: %q", got)
	}
	if got := w.late(now.Add(-3*time.Hour).UnixMilli(), now.UnixMilli()); got != "11-02 09:00 UTC" {
		t.Fatalf("a late event: %q", got)
	}
}

func TestTraderAlertsMarkTestMoneyAndLongDetailsAreCut(t *testing.T) {
	if got := (origin{name: "Trend", testnet: true}).label("en"); got != "Trend · Testnet" {
		t.Fatalf("label = %q", got)
	}
	if got := (origin{name: "Trend"}).label("zh"); got != "Trend" {
		t.Fatalf("label = %q", got)
	}
	if got := detailOf(strings.Repeat("x", 1000)); len([]rune(got)) > maxAlertDetail+1 {
		t.Fatalf("detail not cut: %d", len(got))
	}
}

func TestDailyReportSaysWhenAnAccountCannotBeRead(t *testing.T) {
	w, _, _, clock := alertFixture(t)
	w.sources.AccountEquity = func(_, _ string) (float64, bool) { return 0, false }
	day := clock.UTC().Truncate(24 * time.Hour)
	report, ok := w.dailyReport(testdb.UserID, day, day.Add(24*time.Hour), "en")
	if !ok || !strings.Contains(report.parts()[0].Plain, "• Equity: —") {
		t.Fatalf("report = %v, %v", report, ok)
	}
}
