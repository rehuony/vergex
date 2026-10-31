package activity

import (
	"context"
	"testing"
	"time"

	"vergex/internal/llm"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func feedFixture(t *testing.T, plans func(time.Time, int) ([]PlanRun, error)) (*Feed, *store.Store, *time.Time) {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { st.Close() })
	if err := st.User().Create(&store.User{ID: "alice", Email: "alice@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Trader().Create(&store.Trader{ID: "t1", UserID: "alice", Name: "Trend", ExchangeID: "e1"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	clock := &now
	feed := New(Sources{Store: st, PlanRuns: plans})
	feed.now = func() time.Time { return *clock }
	feed.reader.now = feed.now
	return feed, st, clock
}

func byID(update Update) map[string]Event {
	events := make(map[string]Event, len(update.Events))
	for _, event := range update.Events {
		events[event.ID] = event
	}
	return events
}

func TestFeedSaysWhereEachCallWentAndWhoMadeIt(t *testing.T) {
	feed, st, _ := feedFixture(t, nil)
	for _, charge := range []store.AICharge{
		{TraderID: "t1", Model: "m", Provider: "deepseek", Host: "api.deepseek.com", PromptTokens: 90, CompletionTokens: 10, CostUSD: 0.02},
		{TraderID: llm.OwnerFrontier, Model: "m", Provider: "openai", Host: "api.openai.com", PromptTokens: 5, Unpriced: true},
		{TraderID: llm.OwnerTelegram, Model: "m", Provider: "openai", Host: "api.openai.com", PromptTokens: 5, Unpriced: true},
		{TraderID: llm.OwnerTest, Model: "m", Provider: "deepseek", PromptTokens: 5, Unpriced: true},
	} {
		charge := charge
		if err := st.AICharge().Record(&charge); err != nil {
			t.Fatal(err)
		}
	}
	feed.poll(context.Background())

	update := feed.After(0)
	routes := map[string]int{}
	for _, event := range update.Events {
		routes[event.Route()]++
	}
	for route, want := range map[string]int{"traders>models": 1, "frontier>models": 1, "telegram>models": 1, "other>models": 1, "models>ai_cost": 1} {
		if routes[route] != want {
			t.Errorf("route %s: %d events, want %d (all: %v)", route, routes[route], want, routes)
		}
	}
	if total := update.Totals["traders>models"].H24; total.Tokens != 100 || total.Count != 1 {
		t.Fatalf("trader total = %+v", total)
	}
	// A call that cannot be priced has no cost to report; the estimate is not zero.
	if total := update.Totals["models>ai_cost"].H24; total.Count != 1 || total.Amount != 0.02 {
		t.Fatalf("cost total = %+v", total)
	}
}

func TestFeedFollowsAnOrderAndItsMoneyOnce(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	ctx := context.Background()
	at := clock.UnixMilli()
	order := &store.TraderOrder{TraderID: "t1", ExchangeID: "e1", ExchangeOrderID: "1", Symbol: "BTCUSDT", Side: "BUY", Type: "MARKET", OrderAction: "open_long", Quantity: 2, Status: "NEW", CreatedAt: at, UpdatedAt: at}
	if err := st.Order().CreateOrder(order); err != nil {
		t.Fatal(err)
	}
	feed.poll(ctx)
	if _, ok := byID(feed.After(0))["order:1:NEW"]; !ok {
		t.Fatalf("placed order missing: %+v", feed.After(0).Events)
	}
	seq := feed.After(0).Seq

	// The same status again is not news.
	feed.poll(ctx)
	if got := feed.After(seq).Events; len(got) != 0 {
		t.Fatalf("repeated read reported %+v", got)
	}

	// Order sync stamps rows with the exchange's trade time, older than what was read.
	*clock = clock.Add(10 * time.Second)
	if err := st.GormDB().Model(order).Updates(map[string]any{"status": "FILLED", "updated_at": at - time.Minute.Milliseconds()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.GormDB().Create(&store.TraderFill{TraderID: "t1", ExchangeID: "e1", OrderID: order.ID, ExchangeOrderID: "1", ExchangeTradeID: "f1", Symbol: "BTCUSDT", Side: "BUY", Price: 100, Quantity: 2, QuoteQuantity: 200, Commission: 0.08, CommissionAsset: "USDT", CreatedAt: at}).Error; err != nil {
		t.Fatal(err)
	}
	position := &store.TraderPosition{TraderID: "t1", ExchangeID: "e1", Symbol: "BTCUSDT", Side: "LONG", EntryQuantity: 2, Quantity: 2, EntryPrice: 100, Leverage: 5, EntryTime: at, Status: "OPEN", UpdatedAt: at}
	if err := st.GormDB().Create(position).Error; err != nil {
		t.Fatal(err)
	}
	feed.poll(ctx)
	events := byID(feed.After(seq))
	if events["order:1:FILLED"].Status != "FILLED" {
		t.Fatalf("filled order missing: %+v", events)
	}
	if fee := events["fee:1"]; fee.Amount != 0.08 || fee.Route() != "accounts>fees" {
		t.Fatalf("fee = %+v", fee)
	}
	margin := events["margin:1"]
	if margin.Amount != 40 || margin.Route() != "accounts>positions" || margin.To.ID != "e1:BTCUSDT:long" {
		t.Fatalf("margin = %+v", margin)
	}

	exit := clock.UnixMilli()
	if err := st.GormDB().Model(position).Updates(map[string]any{"status": "CLOSED", "exit_time": exit, "realized_pnl": -3.5, "updated_at": exit}).Error; err != nil {
		t.Fatal(err)
	}
	seq = feed.After(0).Seq
	feed.poll(ctx)
	events = byID(feed.After(seq))
	if realized := events["realized:1"]; realized.Amount != -3.5 || realized.Route() != "positions>accounts" {
		t.Fatalf("realized = %+v", realized)
	}
	if len(events) != 1 {
		t.Fatalf("closing reported more than the result: %+v", events)
	}
}

func TestFeedPutsACycleInTheOrderItHappened(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	saved := clock.Add(-time.Minute)
	acted := saved.Add(-10 * time.Second)
	record := &store.DecisionRecord{
		TraderID:            "t1",
		CycleNumber:         7,
		Timestamp:           saved,
		CandidateCoins:      []string{"BTCUSDT", "ETHUSDT"},
		Success:             true,
		AIRequestDurationMs: 3000,
		Decisions: []store.DecisionAction{
			{Action: "hold", Symbol: "ETHUSDT"},
			{Action: "open_long", Symbol: "BTCUSDT", Timestamp: acted},
		},
	}
	if err := st.Decision().LogDecision(record); err != nil {
		t.Fatal(err)
	}
	feed.poll(context.Background())

	events := byID(feed.After(0))
	decision := events["decision:1"]
	if decision.Action != "open_long" || decision.Symbol != "BTCUSDT" || decision.Items != 1 || decision.Ref != "1" {
		t.Fatalf("decision = %+v", decision)
	}
	read := events["read:1"]
	if read.Items != 2 || read.Route() != "market>traders" {
		t.Fatalf("read = %+v", read)
	}
	if decision.At >= acted.UnixMilli() || read.At >= decision.At-3000+1 {
		t.Fatalf("out of order: read %d, decision %d, acted %d", read.At, decision.At, acted.UnixMilli())
	}
}

func TestFeedRebuildsTheLastDayAfterARestart(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	recent := store.AICharge{TraderID: "t1", Model: "m", Provider: "p", Host: "h", PromptTokens: 5, Unpriced: true, CreatedAt: clock.Add(-time.Hour)}
	old := store.AICharge{TraderID: "t1", Model: "m", Provider: "p", Host: "h", PromptTokens: 5, Unpriced: true, CreatedAt: clock.Add(-25 * time.Hour)}
	if err := st.GormDB().Create(&recent).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.GormDB().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	feed.poll(context.Background())

	update := feed.After(0)
	if len(update.Events) != 1 || update.Events[0].At != recent.CreatedAt.UnixMilli() {
		t.Fatalf("events = %+v", update.Events)
	}
	total := update.Totals["traders>models"]
	if total.H24.Count != 1 || total.H1.Count != 0 {
		t.Fatalf("an hour-old call: %+v", total)
	}
}

func TestFrontierOrdersCountOnceTheyReachTheExchange(t *testing.T) {
	runs := []PlanRun{{
		ID:         "run",
		PlanID:     "plan",
		ExchangeID: "e1",
		Symbol:     "BTCUSDT",
		UpdatedAt:  time.Now(),
		Orders: []PlanOrder{
			{ClientID: "a", Role: "ENTRY", Symbol: "BTCUSDT", Quantity: "0.5", Status: "pending"},
			{ClientID: "b", OrderID: "9", Role: "ENTRY", Symbol: "BTCUSDT", Quantity: "0.5", Status: "NEW"},
			{ClientID: "c", Role: "STOP", Symbol: "BTCUSDT", Quantity: "0.5", Error: "rejected"},
		},
	}}
	feed, _, _ := feedFixture(t, func(time.Time, int) ([]PlanRun, error) { return runs, nil })
	feed.poll(context.Background())

	events := byID(feed.After(0))
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	placed := events["plan:run:b:NEW"]
	if placed.Route() != "frontier>accounts" || placed.Ref != "plan" || placed.Quantity != 0.5 {
		t.Fatalf("placed = %+v", placed)
	}
	if events["plan:run:c:failed"].Status != StatusFailed {
		t.Fatalf("refused order = %+v", events)
	}
}

func TestTotalsStayExactPastTheEndOfTheList(t *testing.T) {
	feed, _, clock := feedFixture(t, nil)
	var events []Event
	for i := 0; i < keepEvents+20; i++ {
		events = append(events, Event{ID: time.Duration(i).String(), At: clock.UnixMilli(), From: Endpoint{Block: BlockAccounts}, To: Endpoint{Block: BlockFees}, Amount: 1, Asset: "USDT"})
	}
	events = append(events, Event{ID: "bnb", At: clock.UnixMilli(), From: Endpoint{Block: BlockAccounts}, To: Endpoint{Block: BlockFees}, Amount: 9, Asset: "BNB"})
	feed.add(events)

	update := feed.After(0)
	if len(update.Events) != keepEvents {
		t.Fatalf("kept %d events", len(update.Events))
	}
	total := update.Totals["accounts>fees"].H1
	if total.Count != keepEvents+21 || total.Amount != keepEvents+20 {
		t.Fatalf("total = %+v", total)
	}

	wake, stop := feed.Subscribe()
	defer stop()
	feed.add([]Event{{ID: "next", At: clock.UnixMilli()}})
	select {
	case <-wake:
	default:
		t.Fatal("a subscriber was not woken")
	}
	if got := feed.After(update.Seq).Events; len(got) != 1 || got[0].ID != "next" {
		t.Fatalf("after = %+v", got)
	}
}
