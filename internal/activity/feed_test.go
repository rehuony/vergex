package activity

import (
	"context"
	"fmt"
	"testing"
	"time"

	"vergex/internal/agent"
	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/shopspring/decimal"
)

// trendTrader is the trader every feed test starts with.
const trendTrader = "00000000-0000-4000-8000-0000000000a1"

func feedFixture(t *testing.T, refused func(time.Time, int) ([]RefusedOrder, error)) (*Feed, *store.Store, *time.Time) {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { st.Close() })
	if err := st.User().Create(&store.User{ID: storetest.UserID, Email: "alice@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	storetest.AddTrader(t, st, &store.Trader{ID: trendTrader, Name: "Trend"})
	now := time.Now().UTC()
	clock := &now
	feed := New(Sources{Store: st, RefusedOrders: refused})
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
	for _, call := range []store.AgentCall{
		{Feature: agent.FeatureTrader, SourceID: trendTrader, Model: "m", Provider: "deepseek", Host: "api.deepseek.com", PromptTokens: 90, CompletionTokens: 10, CostUSD: 0.02},
		{Feature: agent.FeatureFrontier, Model: "m", Provider: "openai", Host: "api.openai.com", PromptTokens: 5, Unpriced: true},
		{Feature: agent.FeatureTelegram, Model: "m", Provider: "openai", Host: "api.openai.com", PromptTokens: 5, Unpriced: true},
		{Feature: agent.FeatureTest, Model: "m", Provider: "deepseek", PromptTokens: 5, Unpriced: true},
	} {
		call := call
		if err := st.AgentCall().Record(&call); err != nil {
			t.Fatal(err)
		}
	}
	feed.poll(context.Background())

	update := feed.After(0)
	routes := map[string]int{}
	for _, event := range update.Events {
		routes[event.Route()]++
	}
	for route, want := range map[string]int{"traders>models": 1, "frontier>models": 1, "telegram>models": 1, "other>models": 1, "models>agent_cost": 1} {
		if routes[route] != want {
			t.Errorf("route %s: %d events, want %d (all: %v)", route, routes[route], want, routes)
		}
	}
	if total := update.Totals["traders>models"].H24; total.Tokens != 100 || total.Count != 1 {
		t.Fatalf("trader total = %+v", total)
	}
	// A call that cannot be priced has no cost to report; the estimate is not zero.
	if total := update.Totals["models>agent_cost"].H24; total.Count != 1 || total.Amount != 0.02 {
		t.Fatalf("cost total = %+v", total)
	}
}

func TestFeedFollowsAnOrderAndItsMoneyOnce(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	ctx := context.Background()
	ledger := storetest.NewLedger(t, st)
	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	leverage := 5
	order := &store.LedgerOrder{
		AccountID: ledger.Account, InstrumentID: btc, Source: store.SourceTrader, SourceID: trendTrader, Role: store.RoleEntry,
		ExchangeOrderID: "1", Side: store.SideBuy, PositionSide: store.SideLong, Type: "MARKET", Status: store.OrderNew,
		Quantity: decimal.NewFromInt(2), Leverage: &leverage, CreatedAt: *clock,
	}
	if err := st.Ledger().RecordOrder(order); err != nil {
		t.Fatal(err)
	}
	orderEvent := fmt.Sprintf("order:%d:", order.ID)
	feed.poll(ctx)
	placed, ok := byID(feed.After(0))[orderEvent+"NEW"]
	if !ok || placed.Route() != "traders>accounts" {
		t.Fatalf("placed order missing: %+v", feed.After(0).Events)
	}
	seq := feed.After(0).Seq

	// The same status again is not news.
	feed.poll(ctx)
	if got := feed.After(seq).Events; len(got) != 0 {
		t.Fatalf("repeated read reported %+v", got)
	}

	// The fill happened before it was read, which is usual: the sync comes later.
	*clock = clock.Add(10 * time.Second)
	opened := clock.Add(-5 * time.Second)
	recorded, err := st.Ledger().RecordTrade(store.Trade{
		AccountID: ledger.Account, InstrumentID: btc, ExchangeTradeID: "f1", ExchangeOrderID: "1", Side: store.SideBuy, PositionSide: store.SideLong,
		Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(2), QuoteQuantity: decimal.NewFromInt(200),
		Commission: decimal.RequireFromString("0.08"), CommissionAsset: "USDT", ExecutedAt: opened,
	}, store.Owner{})
	if err != nil || !recorded {
		t.Fatalf("record trade: %v, %v", recorded, err)
	}
	feed.poll(ctx)
	events := byID(feed.After(seq))
	if events[orderEvent+"FILLED"].Status != "FILLED" {
		t.Fatalf("filled order missing: %+v", events)
	}
	var fills, positions int64
	st.GormDB().Model(&store.LedgerFill{}).Select("MAX(id)").Scan(&fills)
	st.GormDB().Model(&store.LedgerPosition{}).Select("MAX(id)").Scan(&positions)
	if fee := events[fmt.Sprintf("fee:%d", fills)]; fee.Amount != 0.08 || fee.Route() != "accounts>fees" {
		t.Fatalf("fee = %+v", fee)
	}
	margin := events[fmt.Sprintf("margin:%d", positions)]
	if margin.Amount != 40 || margin.Route() != "accounts>positions" || margin.To.ID != ledger.Account+":BTCUSDT:long" {
		t.Fatalf("margin = %+v", margin)
	}

	*clock = clock.Add(10 * time.Second)
	ledger.Close(trendTrader, "BTCUSDT", store.SideLong, 2, 98.25, -3.5, *clock)
	seq = feed.After(0).Seq
	feed.poll(ctx)
	events = byID(feed.After(seq))
	realized := events[fmt.Sprintf("realized:%d", positions)]
	if realized.Amount != -3.5 || realized.Route() != "positions>accounts" {
		t.Fatalf("realized = %+v", realized)
	}
}

func TestFeedTellsAnOrderFromFrontierAndOneNothingClaims(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	ledger := storetest.NewLedger(t, st)
	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	for i, owner := range []store.Owner{{Source: store.SourceFrontier, SourceID: "run"}, {}} {
		id := fmt.Sprintf("%d", i)
		_, err := st.Ledger().RecordTrade(store.Trade{
			AccountID: ledger.Account, InstrumentID: btc, ExchangeTradeID: id, ExchangeOrderID: id, Side: store.SideBuy, PositionSide: store.SideLong,
			Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(1), QuoteQuantity: decimal.NewFromInt(100), ExecutedAt: *clock,
		}, owner)
		if err != nil {
			t.Fatal(err)
		}
	}
	feed.poll(context.Background())
	routes := map[string]int{}
	for _, event := range feed.After(0).Events {
		if event.Kind == KindOrders {
			routes[event.Route()]++
		}
	}
	if routes["frontier>accounts"] != 1 || routes["other>accounts"] != 1 {
		t.Fatalf("routes = %v", routes)
	}
}

func TestFeedPutsACycleInTheOrderItHappened(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	saved := clock.Add(-time.Minute)
	acted := saved.Add(-10 * time.Second)
	record := &store.DecisionRecord{
		TraderID:        trendTrader,
		CycleNumber:     7,
		Timestamp:       saved,
		CandidateCoins:  []string{"BTCUSDT", "ETHUSDT"},
		Success:         true,
		AgentDurationMs: 3000,
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
	recent := store.AgentCall{Feature: agent.FeatureTrader, SourceID: trendTrader, Model: "m", Provider: "p", Host: "h", PromptTokens: 5, Unpriced: true, CreatedAt: clock.Add(-time.Hour)}
	old := store.AgentCall{Feature: agent.FeatureTrader, SourceID: trendTrader, Model: "m", Provider: "p", Host: "h", PromptTokens: 5, Unpriced: true, CreatedAt: clock.Add(-25 * time.Hour)}
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

func TestFrontierOrdersRefusedBeforeTheExchangeStillShow(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	orders := []RefusedOrder{{RunID: "run", PlanID: "plan", AccountID: "e1", ClientID: "c", Role: "STOP", Symbol: "BTCUSDT", Quantity: "0.5", Error: "rejected", At: at}}
	feed, _, _ := feedFixture(t, func(time.Time, int) ([]RefusedOrder, error) { return orders, nil })
	feed.poll(context.Background())
	feed.poll(context.Background())

	events := byID(feed.After(0))
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	refused := events["plan:run:c:failed"]
	if refused.Status != StatusFailed || refused.Route() != "frontier>accounts" || refused.Ref != "plan" || refused.At != at.UnixMilli() {
		t.Fatalf("refused order = %+v", events)
	}
}

// A sync catching up after an outage writes trades long after they happened. They must
// show when they happened, not as news.
func TestAnOrderFilledDuringAnOutageShowsWhenItFilled(t *testing.T) {
	feed, st, clock := feedFixture(t, nil)
	ledger := storetest.NewLedger(t, st)
	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	trade := store.Trade{
		AccountID: ledger.Account, InstrumentID: btc, ExchangeOrderID: "1", Side: store.SideBuy, PositionSide: store.SideLong,
		Price: decimal.NewFromInt(100), Quantity: decimal.NewFromInt(1), QuoteQuantity: decimal.NewFromInt(100),
	}
	filled := clock.Add(-3 * time.Hour)
	trade.ExchangeTradeID, trade.ExecutedAt = "recent", filled
	if _, err := st.Ledger().RecordTrade(trade, store.Owner{}); err != nil {
		t.Fatal(err)
	}
	trade.ExchangeOrderID, trade.ExchangeTradeID, trade.ExecutedAt = "2", "old", clock.Add(-48*time.Hour)
	if _, err := st.Ledger().RecordTrade(trade, store.Owner{}); err != nil {
		t.Fatal(err)
	}
	feed.poll(context.Background())

	var orders []Event
	for _, event := range feed.After(0).Events {
		if event.Kind == KindOrders {
			orders = append(orders, event)
		}
	}
	if len(orders) != 1 || orders[0].At != filled.UnixMilli() {
		t.Fatalf("orders = %+v, want one at %d", orders, filled.UnixMilli())
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
