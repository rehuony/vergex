package trader_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"
	"vergex/internal/trader"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func traderStore(t *testing.T) (*trader.Stores, *testdb.Trader) {
	t.Helper()
	st := trader.NewStores(testdb.Open(t))
	return st, testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Trend"})
}

func TestACycleReadsBackWhatItDecidedAndWhatWasSaid(t *testing.T) {
	st, row := traderStore(t)
	conversation := agent.Conversation{ID: uuid.NewString(), UserID: row.UserID, Feature: agent.FeatureTrader}
	if err := st.Conversations.Open(&conversation); err != nil {
		t.Fatal(err)
	}
	if err := st.Conversations.Append(conversation.ID,
		agent.ConversationMessage{Role: agent.RoleSystem, Content: "the rules"},
		agent.ConversationMessage{Role: agent.RoleUser, Content: "the market"},
		agent.ConversationMessage{Role: agent.RoleAssistant, Content: "the reply", Thinking: "the thinking"},
	); err != nil {
		t.Fatal(err)
	}

	btc, err := instrument.Ensure(st.DB, "binance", instrument.KindPerpetual, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	order := account.LedgerOrder{
		AccountID: row.AccountID, InstrumentID: btc, Source: account.SourceTrader, SourceID: row.ID, Role: account.RoleEntry,
		VenueOrderID: "77", Side: account.SideBuy, PositionSide: account.SideLong, Type: "MARKET", Status: account.OrderNew, Quantity: decimal.NewFromInt(1),
	}
	if err := st.Ledger.RecordOrder(&order); err != nil {
		t.Fatal(err)
	}

	finished := time.Now().UTC().Truncate(time.Millisecond)
	account := &trader.AccountSnapshot{TotalBalance: 1000, PositionCount: 2, MarginUsedPct: 12.5}
	record := &trader.DecisionRecord{
		TraderID: row.ID, Seq: 7, Timestamp: finished, ConversationID: conversation.ID,
		CandidateCoins: []string{"BTCUSDT", "ETHUSDT"}, ExecutionLog: []string{"line one"},
		Success: false, ErrorMessage: "1 of 3 failed", AgentDurationMs: 1500, AccountState: account,
		Decisions: []trader.DecisionAction{
			{Action: "open_long", Symbol: "BTCUSDT", Quantity: 0.5, Leverage: 5, Price: 100, StopLoss: 90, TakeProfit: 130, Confidence: 80, Reasoning: "up", OrderID: order.ID, Success: true},
			{Action: "close_short", Symbol: "ETHUSDT", Error: "rejected"},
			{Action: "open_short", Symbol: "SOLUSDT", Error: "cooling down", Blocked: true},
			{Action: "hold", Symbol: "XRPUSDT", Success: true},
		},
	}
	if err := st.Decisions.LogDecision(record); err != nil {
		t.Fatal(err)
	}

	full, err := st.Decisions.GetRecord(row.ID, record.ID)
	if err != nil || full == nil {
		t.Fatalf("record = %+v, err %v", full, err)
	}
	if full.SystemPrompt != "the rules" || full.InputPrompt != "the market" || full.RawResponse != "the reply" || full.CoTTrace != "the thinking" {
		t.Fatalf("what was said = %+v", full)
	}
	if full.Seq != 7 || !full.Timestamp.Equal(finished) || full.ErrorMessage != "1 of 3 failed" || full.AgentDurationMs != 1500 || full.Success {
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
	summaries, err := st.Decisions.GetSummaries(row.ID, 10, 0)
	if err != nil || len(summaries) != 1 || summaries[0].SystemPrompt != "" || len(summaries[0].Decisions) != 4 {
		t.Fatalf("summaries = %+v, err %v", summaries, err)
	}
}

func TestACycleThatStoppedEarlyHasNoAccountOrConversation(t *testing.T) {
	st, row := traderStore(t)
	record := &trader.DecisionRecord{TraderID: row.ID, Seq: 1, ErrorMessage: "Trading blocked"}
	if err := st.Decisions.LogDecision(record); err != nil {
		t.Fatal(err)
	}
	full, err := st.Decisions.GetRecord(row.ID, record.ID)
	if err != nil || full == nil || full.AccountState != nil || full.ConversationID != "" || full.SystemPrompt != "" {
		t.Fatalf("record = %+v, err %v", full, err)
	}
}

func TestAConversationNoCycleWasSavedForIsDeletedAndItsCallKept(t *testing.T) {
	st, row := traderStore(t)
	open := func() string {
		t.Helper()
		conversation := agent.Conversation{ID: uuid.NewString(), UserID: row.UserID, Feature: agent.FeatureTrader}
		if err := st.Conversations.Open(&conversation); err != nil {
			t.Fatal(err)
		}
		if err := st.Conversations.Append(conversation.ID, agent.ConversationMessage{Role: agent.RoleUser, Content: "the market"}); err != nil {
			t.Fatal(err)
		}
		call := agent.Call{Feature: agent.FeatureTrader, SourceID: row.ID, ConversationID: conversation.ID, Provider: "deepseek", Model: "m", CostUSD: 0.01}
		if err := agent.NewCallStore(st.DB).Record(&call); err != nil {
			t.Fatal(err)
		}
		return conversation.ID
	}
	saved, lost, other := open(), open(), open()
	if err := st.Decisions.LogDecision(&trader.DecisionRecord{TraderID: row.ID, Seq: 1, ConversationID: saved}); err != nil {
		t.Fatal(err)
	}

	// Named, only that one goes, and only while no cycle points at it.
	if n, err := st.Decisions.ForgetConversations(saved, lost); err != nil || n != 1 {
		t.Fatalf("forgot %d, err %v", n, err)
	}
	if _, err := st.Conversations.Get(row.UserID, other); err != nil {
		t.Fatalf("one not named went: %v", err)
	}
	// Unnamed, the rest of them go.
	if n, err := st.Decisions.ForgetConversations(); err != nil || n != 1 {
		t.Fatalf("swept %d, err %v", n, err)
	}
	if _, err := st.Conversations.Get(row.UserID, saved); err != nil {
		t.Fatalf("the saved cycle's conversation went: %v", err)
	}
	// What each call cost stays the trader's.
	_, total, err := agent.NewCallStore(st.DB).ForTrader(row.ID, "")
	if err != nil || total < 0.0299 {
		t.Fatalf("the trader's costs came to %v, err %v", total, err)
	}
	var messages int64
	if err := st.DB.Model(&agent.ConversationMessage{}).Where("conversation_id IN ?", []string{lost, other}).Count(&messages).Error; err != nil || messages != 0 {
		t.Fatalf("%d messages left, err %v", messages, err)
	}
}

func TestDecisionsKeepModelOutputPostgresWouldReject(t *testing.T) {
	st, row := traderStore(t)
	record := &trader.DecisionRecord{
		TraderID: row.ID, ErrorMessage: "up\x00 to \xffhere", ExecutionLog: []string{"up\x00 to \xffhere"},
		Decisions: []trader.DecisionAction{{Action: "hold", Reasoning: "up\x00 to \xffhere", Error: "up\x00 to \xffhere"}},
	}
	if err := st.Decisions.LogDecision(record); err != nil {
		t.Fatal(err)
	}
	full, err := st.Decisions.GetRecord(row.ID, record.ID)
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
	st, row := traderStore(t)
	finished := time.Now().UTC()
	record := &trader.DecisionRecord{
		TraderID: row.ID, Timestamp: finished,
		Decisions: []trader.DecisionAction{
			{Action: "open_long", Symbol: "BTCUSDT", Error: "rejected"},
			{Action: "open_short", Symbol: "ETHUSDT", Error: "cooling down", Blocked: true},
			{Action: "hold", Symbol: "SOLUSDT", Success: true},
			{Action: "close_long", Symbol: "XRPUSDT", Success: true},
		},
	}
	if err := st.Decisions.LogDecision(record); err != nil {
		t.Fatal(err)
	}
	failed, err := st.Decisions.FailedActionsSince([]string{row.ID}, finished.Add(-time.Minute).UnixMilli(), 10)
	if err != nil || len(failed) != 1 || failed[0].Symbol != "BTCUSDT" || failed[0].Error != "rejected" || failed[0].TraderID != row.ID {
		t.Fatalf("failed = %+v, err %v", failed, err)
	}
	// The caller remembers the time cut to milliseconds, and must not get it back.
	again, err := st.Decisions.FailedActionsSince([]string{row.ID}, failed[0].Timestamp.UnixMilli(), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("reported again: %+v, err %v", again, err)
	}
}

func TestAnArchivedTraderIsGoneButItsHistoryStays(t *testing.T) {
	st, row := traderStore(t)
	record := &trader.DecisionRecord{TraderID: row.ID, Seq: 1, Success: true}
	if err := st.Decisions.LogDecision(record); err != nil {
		t.Fatal(err)
	}
	if err := st.Equity.Save(&trader.EquitySnapshot{TraderID: row.ID, TotalEquity: 1000}); err != nil {
		t.Fatal(err)
	}
	ledger := accounttest.NewLedger(t, st.DB)
	ledger.Open(row.ID, "BTCUSDT", account.SideLong, 1, 100, time.Now().Add(-time.Minute))
	ledger.Close(row.ID, "BTCUSDT", account.SideLong, 1, 110, 10, time.Now())

	if err := st.Accounts.Delete(row.UserID, row.AccountID); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("an account in use: %v", err)
	}
	if err := st.Models.Delete(row.UserID, row.AgentModelID); !errors.Is(err, gorm.ErrCheckConstraintViolated) {
		t.Fatalf("a model in use: %v", err)
	}

	if err := st.Traders.Archive(row.UserID, row.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Traders.Archive(row.UserID, row.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("archived twice: %v", err)
	}

	if traders, err := st.Traders.List(row.UserID); err != nil || len(traders) != 0 {
		t.Fatalf("traders = %+v, err %v", traders, err)
	}
	if _, err := st.Traders.GetByID(row.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("by ID: %v", err)
	}
	if _, err := st.Traders.GetFullConfig(row.UserID, row.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("full config: %v", err)
	}

	if records, err := st.Decisions.GetSummaries(row.ID, 10, 0); err != nil || len(records) != 1 {
		t.Fatalf("cycles = %+v, err %v", records, err)
	}
	if points, err := st.Equity.GetSeries(row.ID, time.Time{}, 10); err != nil || len(points) != 1 {
		t.Fatalf("equity = %+v, err %v", points, err)
	}
	if positions, err := st.Positions.GetClosedPositions(row.ID, 10); err != nil || len(positions) != 1 {
		t.Fatalf("positions = %+v, err %v", positions, err)
	}

	// It has let go of what it used, so they can go too.
	if err := st.Models.Delete(row.UserID, row.AgentModelID); err != nil {
		t.Fatalf("model: %v", err)
	}
	if err := st.Accounts.Delete(row.UserID, row.AccountID); err != nil {
		t.Fatalf("account: %v", err)
	}
}

func TestATraderCannotSaveTwoCyclesWithOneNumber(t *testing.T) {
	st, row := traderStore(t)
	if err := st.Decisions.LogDecision(&trader.DecisionRecord{TraderID: row.ID, Seq: 1, Success: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.Decisions.LogDecision(&trader.DecisionRecord{TraderID: row.ID, Seq: 1, Success: true}); err == nil {
		t.Fatal("a second cycle 1 was saved")
	}
	if last, err := st.Decisions.LastSeq(row.ID); err != nil || last != 1 {
		t.Fatalf("last = %d, %v", last, err)
	}
}

func TestArchivingKeepsATradersConfig(t *testing.T) {
	st, row := traderStore(t)
	if err := st.Traders.Archive(row.UserID, row.ID); err != nil {
		t.Fatal(err)
	}
	var config string
	if err := st.DB.Raw(`SELECT config::text FROM trader.traders WHERE id = ?`, row.ID).Scan(&config).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, "risk_control") {
		t.Fatalf("config = %q", config)
	}
}

func TestATraderMustNameWhatItTradesWith(t *testing.T) {
	st, row := traderStore(t)
	stray := trader.Trader{ID: uuid.NewString(), UserID: row.UserID, Name: row.Name, Kind: row.Kind, Config: row.Config, AccountID: row.AccountID, AgentModelID: uuid.NewString()}
	if err := st.Traders.Create(&stray); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("a model that does not exist: %v", err)
	}
}

func TestAnIDThatIsNotOneFindsNothing(t *testing.T) {
	st, row := traderStore(t)
	if _, err := st.Traders.GetByID("t1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("by ID: %v", err)
	}
	if _, err := st.Traders.Get(row.UserID, "t1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("own: %v", err)
	}
	if traders, err := st.Traders.ListByAccountID(row.UserID, "e1"); err != nil || len(traders) != 0 {
		t.Fatalf("by account: %+v, err %v", traders, err)
	}
}

func TestLatestEquityIsEachTradersNewest(t *testing.T) {
	st, row := traderStore(t)
	other := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Other"})
	now := time.Now().UTC()
	for _, snapshot := range []trader.EquitySnapshot{
		{TraderID: row.ID, Timestamp: now.Add(-time.Hour), TotalEquity: 900},
		{TraderID: row.ID, Timestamp: now, TotalEquity: 950},
		{TraderID: other.ID, Timestamp: now, TotalEquity: 1200},
	} {
		snapshot := snapshot
		if err := st.Equity.Save(&snapshot); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := st.Equity.GetAllTradersLatest()
	if err != nil || len(latest) != 2 || latest[row.ID].TotalEquity != 950 || latest[other.ID].TotalEquity != 1200 {
		t.Fatalf("latest = %+v, err %v", latest, err)
	}
}
