package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func traderStore(t *testing.T) (*store.Store, *store.Trader) {
	t.Helper()
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	return st, storetest.AddTrader(t, st, &store.Trader{Name: "Trend"})
}

func TestACycleReadsBackWhatItDecidedAndWhatWasSaid(t *testing.T) {
	st, trader := traderStore(t)
	conversation := store.AgentConversation{ID: uuid.NewString(), UserID: trader.UserID, Feature: store.FeatureTrader, SourceID: trader.ID}
	if err := st.AgentConversation().Open(&conversation); err != nil {
		t.Fatal(err)
	}
	if err := st.AgentConversation().Append(conversation.ID,
		store.AgentMessage{Role: store.RoleSystem, Content: "the rules"},
		store.AgentMessage{Role: store.RoleUser, Content: "the market"},
		store.AgentMessage{Role: store.RoleAssistant, Content: "the reply", Thinking: "the thinking"},
	); err != nil {
		t.Fatal(err)
	}

	btc, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "BTCUSDT", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	order := store.LedgerOrder{
		AccountID: trader.ExchangeID, InstrumentID: btc, Source: store.SourceTrader, SourceID: trader.ID, Role: store.RoleEntry,
		ExchangeOrderID: "77", Side: store.SideBuy, PositionSide: store.SideLong, Type: "MARKET", Status: store.OrderNew, Quantity: decimal.NewFromInt(1),
	}
	if err := st.Ledger().RecordOrder(&order); err != nil {
		t.Fatal(err)
	}

	finished := time.Now().UTC().Truncate(time.Millisecond)
	account := &store.AccountSnapshot{TotalBalance: 1000, PositionCount: 2, MarginUsedPct: 12.5}
	record := &store.DecisionRecord{
		TraderID: trader.ID, CycleNumber: 7, Timestamp: finished, ConversationID: conversation.ID,
		CandidateCoins: []string{"BTCUSDT", "ETHUSDT"}, ExecutionLog: []string{"line one"},
		Success: false, ErrorMessage: "1 of 3 failed", AgentDurationMs: 1500, AccountState: account,
		Decisions: []store.DecisionAction{
			{Action: "open_long", Symbol: "BTCUSDT", Quantity: 0.5, Leverage: 5, Price: 100, StopLoss: 90, TakeProfit: 130, Confidence: 80, Reasoning: "up", OrderID: order.ID, Success: true},
			{Action: "close_short", Symbol: "ETHUSDT", Error: "rejected"},
			{Action: "open_short", Symbol: "SOLUSDT", Error: "cooling down", Blocked: true},
			{Action: "hold", Symbol: "XRPUSDT", Success: true},
		},
	}
	if err := st.Decision().LogDecision(record); err != nil {
		t.Fatal(err)
	}

	full, err := st.Decision().GetRecord(trader.ID, record.ID)
	if err != nil || full == nil {
		t.Fatalf("record = %+v, err %v", full, err)
	}
	if full.SystemPrompt != "the rules" || full.InputPrompt != "the market" || full.RawResponse != "the reply" || full.CoTTrace != "the thinking" {
		t.Fatalf("what was said = %+v", full)
	}
	if full.CycleNumber != 7 || !full.Timestamp.Equal(finished) || full.ErrorMessage != "1 of 3 failed" || full.AgentDurationMs != 1500 || full.Success {
		t.Fatalf("cycle = %+v", full)
	}
	if len(full.CandidateCoins) != 2 || full.CandidateCoins[1] != "ETHUSDT" || len(full.ExecutionLog) != 1 {
		t.Fatalf("lists = %+v", full)
	}
	if full.AccountState == nil || *full.AccountState != *account {
		t.Fatalf("account = %+v", full.AccountState)
	}
	if len(full.Decisions) != 4 {
		t.Fatalf("decisions = %+v", full.Decisions)
	}
	opened := full.Decisions[0]
	if opened.Quantity != 0.5 || opened.Leverage != 5 || opened.StopLoss != 90 || opened.TakeProfit != 130 || opened.Confidence != 80 || opened.Reasoning != "up" || opened.OrderID != order.ID || !opened.Success {
		t.Fatalf("opened = %+v", opened)
	}
	if failed := full.Decisions[1]; failed.Success || failed.Blocked || failed.Error != "rejected" {
		t.Fatalf("failed = %+v", failed)
	}
	if blocked := full.Decisions[2]; blocked.Success || !blocked.Blocked {
		t.Fatalf("blocked = %+v", blocked)
	}
	if hold := full.Decisions[3]; !hold.Success || hold.Blocked {
		t.Fatalf("hold = %+v", hold)
	}

	// A summary leaves out what was said, which can be tens of kilobytes.
	summaries, err := st.Decision().GetSummaries(trader.ID, 10, 0)
	if err != nil || len(summaries) != 1 || summaries[0].SystemPrompt != "" || len(summaries[0].Decisions) != 4 {
		t.Fatalf("summaries = %+v, err %v", summaries, err)
	}
}

func TestACycleThatStoppedEarlyHasNoAccountOrConversation(t *testing.T) {
	st, trader := traderStore(t)
	record := &store.DecisionRecord{TraderID: trader.ID, CycleNumber: 1, ErrorMessage: "Trading blocked"}
	if err := st.Decision().LogDecision(record); err != nil {
		t.Fatal(err)
	}
	full, err := st.Decision().GetRecord(trader.ID, record.ID)
	if err != nil || full == nil || full.AccountState != nil || full.ConversationID != "" || full.SystemPrompt != "" {
		t.Fatalf("record = %+v, err %v", full, err)
	}
}

func TestDecisionsKeepModelOutputPostgresWouldReject(t *testing.T) {
	st, trader := traderStore(t)
	record := &store.DecisionRecord{
		TraderID: trader.ID, ErrorMessage: "up\x00 to \xffhere", ExecutionLog: []string{"up\x00 to \xffhere"},
		Decisions: []store.DecisionAction{{Action: "hold", Reasoning: "up\x00 to \xffhere", Error: "up\x00 to \xffhere"}},
	}
	if err := st.Decision().LogDecision(record); err != nil {
		t.Fatal(err)
	}
	full, err := st.Decision().GetRecord(trader.ID, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{full.ErrorMessage, full.ExecutionLog[0], full.Decisions[0].Reasoning, full.Decisions[0].Error} {
		if strings.Contains(text, "\x00") || !strings.HasPrefix(text, "up to ") {
			t.Fatalf("kept %q", text)
		}
	}
}

func TestFailedActionsAreReportedOnce(t *testing.T) {
	st, trader := traderStore(t)
	finished := time.Now().UTC()
	record := &store.DecisionRecord{
		TraderID: trader.ID, Timestamp: finished,
		Decisions: []store.DecisionAction{
			{Action: "open_long", Symbol: "BTCUSDT", Error: "rejected"},
			{Action: "open_short", Symbol: "ETHUSDT", Error: "cooling down", Blocked: true},
			{Action: "hold", Symbol: "SOLUSDT", Success: true},
			{Action: "close_long", Symbol: "XRPUSDT", Success: true},
		},
	}
	if err := st.Decision().LogDecision(record); err != nil {
		t.Fatal(err)
	}
	failed, err := st.Decision().FailedActionsSince([]string{trader.ID}, finished.Add(-time.Minute).UnixMilli(), 10)
	if err != nil || len(failed) != 1 || failed[0].Symbol != "BTCUSDT" || failed[0].Error != "rejected" || failed[0].TraderID != trader.ID {
		t.Fatalf("failed = %+v, err %v", failed, err)
	}
	// The caller remembers the time cut to milliseconds, and must not get it back.
	again, err := st.Decision().FailedActionsSince([]string{trader.ID}, failed[0].Timestamp.UnixMilli(), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("reported again: %+v, err %v", again, err)
	}
}

func TestAnArchivedTraderIsGoneButItsHistoryStays(t *testing.T) {
	st, trader := traderStore(t)
	record := &store.DecisionRecord{TraderID: trader.ID, CycleNumber: 1, Success: true}
	if err := st.Decision().LogDecision(record); err != nil {
		t.Fatal(err)
	}
	if err := st.Equity().Save(&store.EquitySnapshot{TraderID: trader.ID, TotalEquity: 1000}); err != nil {
		t.Fatal(err)
	}
	ledger := storetest.NewLedger(t, st)
	ledger.Open(trader.ID, "BTCUSDT", store.SideLong, 1, 100, time.Now().Add(-time.Minute))
	ledger.Close(trader.ID, "BTCUSDT", store.SideLong, 1, 110, 10, time.Now())

	if err := st.Strategy().Delete(trader.UserID, trader.StrategyID); err == nil {
		t.Fatal("a strategy in use was deleted")
	}
	if err := st.Exchange().Delete(trader.UserID, trader.ExchangeID); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("an account in use: %v", err)
	}
	if err := st.AgentModel().Delete(trader.UserID, trader.AgentModelID); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("a model in use: %v", err)
	}

	if err := st.Trader().Archive(trader.UserID, trader.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Trader().Archive(trader.UserID, trader.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("archived twice: %v", err)
	}

	if traders, err := st.Trader().List(trader.UserID); err != nil || len(traders) != 0 {
		t.Fatalf("traders = %+v, err %v", traders, err)
	}
	if _, err := st.Trader().GetByID(trader.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("by ID: %v", err)
	}
	if _, err := st.Trader().GetFullConfig(trader.UserID, trader.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("full config: %v", err)
	}

	if records, err := st.Decision().GetSummaries(trader.ID, 10, 0); err != nil || len(records) != 1 {
		t.Fatalf("cycles = %+v, err %v", records, err)
	}
	if points, err := st.Equity().GetSeries(trader.ID, time.Time{}, 10); err != nil || len(points) != 1 {
		t.Fatalf("equity = %+v, err %v", points, err)
	}
	if positions, err := st.Position().GetClosedPositions(trader.ID, 10); err != nil || len(positions) != 1 {
		t.Fatalf("positions = %+v, err %v", positions, err)
	}

	// It has let go of what it used, so they can go too.
	if err := st.Strategy().Delete(trader.UserID, trader.StrategyID); err != nil {
		t.Fatalf("strategy: %v", err)
	}
	if err := st.AgentModel().Delete(trader.UserID, trader.AgentModelID); err != nil {
		t.Fatalf("model: %v", err)
	}
	if err := st.Exchange().Delete(trader.UserID, trader.ExchangeID); err != nil {
		t.Fatalf("account: %v", err)
	}
}

func TestATraderMustNameWhatItTradesWith(t *testing.T) {
	st, trader := traderStore(t)
	stray := *trader
	stray.ID = uuid.NewString()
	stray.StrategyID = uuid.NewString()
	if err := st.Trader().Create(&stray); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("a strategy that does not exist: %v", err)
	}
}

func TestAnIDThatIsNotOneFindsNothing(t *testing.T) {
	st, trader := traderStore(t)
	if _, err := st.Trader().GetByID("t1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("by ID: %v", err)
	}
	if _, err := st.Trader().Get(trader.UserID, "t1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("own: %v", err)
	}
	if traders, err := st.Trader().ListByExchangeID(trader.UserID, "e1"); err != nil || len(traders) != 0 {
		t.Fatalf("by account: %+v, err %v", traders, err)
	}
}

func TestLatestEquityIsEachTradersNewest(t *testing.T) {
	st, trader := traderStore(t)
	other := storetest.AddTrader(t, st, &store.Trader{Name: "Other"})
	now := time.Now().UTC()
	for _, snapshot := range []store.EquitySnapshot{
		{TraderID: trader.ID, Timestamp: now.Add(-time.Hour), TotalEquity: 900},
		{TraderID: trader.ID, Timestamp: now, TotalEquity: 950},
		{TraderID: other.ID, Timestamp: now, TotalEquity: 1200},
	} {
		snapshot := snapshot
		if err := st.Equity().Save(&snapshot); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := st.Equity().GetAllTradersLatest()
	if err != nil || len(latest) != 2 || latest[trader.ID].TotalEquity != 950 || latest[other.ID].TotalEquity != 1200 {
		t.Fatalf("latest = %+v, err %v", latest, err)
	}
}
