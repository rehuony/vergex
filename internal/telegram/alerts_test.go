package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/accounttest"
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

func TestSplitMessageKeepsLinesWithinTheLimit(t *testing.T) {
	line := strings.Repeat("x", 2000)
	parts := splitMessage(line + "\n" + line + "\n" + line)
	if len(parts) != 3 {
		t.Fatalf("parts = %d", len(parts))
	}
}
