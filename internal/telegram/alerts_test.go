package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func alertFixture(t *testing.T) (*alertWatcher, *store.Store, *[]string, *time.Time) {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { st.Close() })
	if err := st.User().Create(&store.User{ID: "alice", Email: "alice@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Trader().Create(&store.Trader{ID: "t1", UserID: "alice", Name: "Trend"}); err != nil {
		t.Fatal(err)
	}
	if err := st.TelegramConfig().Save("token", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.TelegramConfig().BindUser(42, "alice"); err != nil {
		t.Fatal(err)
	}
	sent := &[]string{}
	now := time.Now()
	clock := &now
	w := &alertWatcher{
		st:   st,
		send: func(_ context.Context, text string) error { *sent = append(*sent, text); return nil },
		now:  func() time.Time { return *clock },
	}
	return w, st, sent, clock
}

func closePosition(t *testing.T, st *store.Store, exit time.Time) {
	t.Helper()
	position := &store.TraderPosition{TraderID: "t1", Symbol: "BTCUSDT", Side: "LONG", Status: "CLOSED", ExitTime: exit.UnixMilli(), RealizedPnL: 12.5, EntryTime: exit.Add(-time.Hour).UnixMilli()}
	if err := st.Position().Create(position); err != nil {
		t.Fatal(err)
	}
	// Create may force the opening status; alerts read what is stored.
	if err := st.GormDB().Model(position).Updates(map[string]any{"status": "CLOSED", "exit_time": exit.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
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
		return map[string]any{"safe_mode": safeMode, "safe_mode_reason": "3 AI failures"}
	}
	w.check(context.Background())
	w.check(context.Background())
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Safe mode: Trend: 3 AI failures") {
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
	if err := st.TelegramConfig().SetMutedAlerts([]string{AlertPositionClosed}); err != nil {
		t.Fatal(err)
	}

	*clock = clock.Add(time.Minute)
	closePosition(t, st, clock.Add(-time.Second))
	w.check(context.Background())
	if len(*sent) != 0 {
		t.Fatalf("muted kind was sent: %v", *sent)
	}

	if err := st.TelegramConfig().SetMutedAlerts(nil); err != nil {
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
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Frontier plan needs attention: ETHUSDT") {
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
