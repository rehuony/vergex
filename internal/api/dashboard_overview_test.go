package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/database/testdb"
	"vergex/internal/market/instrument"
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func TestOverviewPositionsUseOneShapeWhateverTheAdapterReturns(t *testing.T) {
	position, ok := normalizeOverviewPosition(map[string]interface{}{
		"symbol": "BTCUSDT", "side": "short", "positionAmt": -0.5, "entryPrice": 100.0,
		"markPrice": 110.0, "unRealizedProfit": -5.0, "leverage": 5.0,
	})
	if !ok {
		t.Fatal("a complete position was dropped")
	}
	if position.Quantity != 0.5 || position.EntryPrice != 100 || position.MarkPrice != 110 || position.UnrealizedPnL != -5 {
		t.Fatalf("fields were not read: %+v", position)
	}
	if position.UnrealizedPnLPct != -50 {
		t.Fatalf("return on margin = %v, want -50", position.UnrealizedPnLPct)
	}
	if _, ok := normalizeOverviewPosition(map[string]interface{}{"symbol": "BTCUSDT", "side": "flat"}); ok {
		t.Fatal("a position without a direction was kept")
	}
}

func TestOverviewPositionOwner(t *testing.T) {
	traders := []overviewTrader{
		{TraderID: "stopped", AccountID: "a"},
		{TraderID: "active", AccountID: "a", Running: true},
		{TraderID: "idle", AccountID: "b"},
	}
	runs := []overviewRun{{AccountID: "b", Symbol: "ETH-USDT-SWAP"}}
	cases := []struct{ account, symbol, want string }{
		{"a", "BTCUSDT", "active"}, // A running trader owns its account.
		{"b", "ETHUSDT", ""},       // An active plan owns its symbol.
		{"b", "BTCUSDT", "idle"},   // Otherwise the bound trader can close it.
		{"c", "BTCUSDT", ""},       // No trader at all: Futures.
	}
	for _, c := range cases {
		if got := overviewPositionOwner(traders, runs, c.account, c.symbol); got != c.want {
			t.Errorf("%s %s: owner %q, want %q", c.account, c.symbol, got, c.want)
		}
	}
}

func TestOverviewCacheBuildsOncePerInterval(t *testing.T) {
	var cache overviewCache
	builds := 0
	build := func() overviewResponse {
		builds++
		return overviewResponse{}
	}
	cache.get("alice", build)
	cache.get("alice", build)
	if builds != 1 {
		t.Fatalf("built %d times within the interval", builds)
	}
	cache.get("bob", build)
	if builds != 2 {
		t.Fatal("users must not share an overview")
	}
	cache.entries["alice"] = cachedOverview{at: time.Now().Add(-overviewCacheTTL)}
	cache.get("alice", build)
	if builds != 3 {
		t.Fatal("an expired overview was not rebuilt")
	}
}

func TestOverviewStaysWithinTheOwnerAndNeedsNoExchange(t *testing.T) {
	st := NewStores(testdb.Open(t))
	mine := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine"})
	testdb.AddTrader(t, st.DB, &testdb.Trader{UserID: testdb.OtherUserID, Name: "Theirs"})
	ledger := accounttest.NewLedger(t, st.DB)
	ledger.Open(mine.ID, "BTCUSDT", account.SideLong, 1, 100, time.Now().Add(-time.Second))
	ledger.Close(mine.ID, "BTCUSDT", account.SideLong, 1, 110, 12.5, time.Now())
	// Realized today is the account's, so a Futures trade on it counts too.
	eth, err := instrument.Ensure(st.DB, "binance", instrument.KindPerpetual, "ETHUSDT")
	if err != nil {
		t.Fatal(err)
	}
	futures := account.Owner{Source: account.SourceFutures, SourceID: "run"}
	for _, trade := range []account.Trade{
		{VenueTradeID: "f1", VenueOrderID: "f1", Side: account.SideBuy},
		{VenueTradeID: "f2", VenueOrderID: "f2", Side: account.SideSell, RealizedPnL: decimal.NewNullDecimal(decimal.RequireFromString("2.5"))},
	} {
		trade.AccountID, trade.InstrumentID, trade.PositionSide = ledger.Account, eth, account.SideLong
		trade.Price, trade.Quantity, trade.QuoteQuantity, trade.ExecutedAt = decimal.NewFromInt(100), decimal.NewFromInt(1), decimal.NewFromInt(100), time.Now()
		if _, err := st.Ledger.RecordTrade(trade, futures); err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{stores: st, traderManager: trader.NewManager(trader.NewStores(st.DB)), accountStateCache: NewAccountStateCache()}
	// Known to be unreadable, so nothing tries to reach an exchange.
	accounts, err := st.Accounts.List(testdb.UserID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]AccountState{}
	for _, account := range accounts {
		states[account.ID] = AccountState{AccountID: account.ID, Status: accountStatusInvalidCredentials}
	}
	s.accountStateCache.Set(testdb.UserID, states)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	r.GET("/dashboard/overview", s.handleOverview)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body overviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Traders) != 1 || body.Traders[0].TraderID != mine.ID {
		t.Fatalf("traders: %+v", body.Traders)
	}
	if body.RealizedToday == nil || *body.RealizedToday != 15 {
		t.Fatalf("realized today: %v", body.RealizedToday)
	}
}

func TestOverviewAccountThatCannotBeReadIsNotReportedAsEmpty(t *testing.T) {
	st := NewStores(testdb.Open(t))
	bad := &account.Account{ID: "bad", UserID: "alice", Venue: "binance", Name: "Bad"}
	s := &Server{stores: st, traderManager: trader.NewManager(trader.NewStores(st.DB)), accountStateCache: NewAccountStateCache()}
	s.accountStateCache.Set("alice", map[string]AccountState{
		"bad": {AccountID: "bad", Status: accountStatusInvalidCredentials, ErrorMessage: "invalid"},
	})

	var out overviewResponse
	ready := s.addOverviewAccounts("alice", []*account.Account{bad}, &out)
	addOverviewAccountData("alice", ready, &out)

	if len(ready) != 0 || !out.PositionsPartial {
		t.Fatalf("an unread account looked empty: ready=%d partial=%v", len(ready), out.PositionsPartial)
	}
	if len(out.Attention) != 1 || out.Attention[0].Kind != "account_invalid_credentials" {
		t.Fatalf("attention: %+v", out.Attention)
	}
}

func TestOverviewNamesTradesTheLedgerIsMissing(t *testing.T) {
	st := NewStores(testdb.Open(t))
	ledger := accounttest.NewLedger(t, st.DB)
	s := &Server{stores: st}

	var out overviewResponse
	s.addOverviewSkippedFills(testdb.UserID, &out)
	if len(out.Attention) != 0 {
		t.Fatalf("attention with nothing missing: %+v", out.Attention)
	}

	record := func(id string, at time.Time) {
		t.Helper()
		fill := &account.SkippedFill{AccountID: ledger.Account, NativeSymbol: "OLD-USDT-SWAP", VenueTradeID: id, Detail: "sell 7 contracts at 3: gone", ExecutedAt: at}
		if _, err := st.Ledger.RecordSkippedFill(fill); err != nil {
			t.Fatal(err)
		}
		if err := st.DB.Model(fill).Update("recorded_at", at).Error; err != nil {
			t.Fatal(err)
		}
	}
	record("old", time.Now().Add(-skippedFillsShownFor-time.Hour))
	s.addOverviewSkippedFills(testdb.UserID, &out)
	if len(out.Attention) != 0 {
		t.Fatalf("an old one is still shown: %+v", out.Attention)
	}

	record("t1", time.Now())
	s.addOverviewSkippedFills(testdb.UserID, &out)
	if len(out.Attention) != 1 || out.Attention[0].Kind != "account_fill_skipped" || out.Attention[0].Name != "OLD-USDT-SWAP" {
		t.Fatalf("attention: %+v", out.Attention)
	}
	if out.Attention[0].Detail != "trade t1: sell 7 contracts at 3: gone" {
		t.Fatalf("detail: %s", out.Attention[0].Detail)
	}

	// However many there are, they share one row and say there are more.
	for i := range skippedFillsSpelled + 2 {
		record(fmt.Sprintf("more-%d", i), time.Now())
	}
	out = overviewResponse{}
	s.addOverviewSkippedFills(testdb.UserID, &out)
	if len(out.Attention) != 1 || !strings.HasSuffix(out.Attention[0].Detail, "and more") {
		t.Fatalf("attention: %+v", out.Attention)
	}
}

func TestOverviewLeavesTestMoneyOutOfItsTotals(t *testing.T) {
	st := NewStores(testdb.Open(t))
	s := &Server{stores: st, traderManager: trader.NewManager(trader.NewStores(st.DB)), accountStateCache: NewAccountStateCache()}
	demo := &account.Account{ID: "demo", UserID: "alice", Venue: "binance", Name: "Demo", Testnet: true}
	main := &account.Account{ID: "main", UserID: "alice", Venue: "binance", Name: "Main"}
	s.accountStateCache.Set("alice", map[string]AccountState{
		"demo": {AccountID: "demo", Status: accountStatusOK, TotalEquity: 5000},
		"main": {AccountID: "main", Status: accountStatusOK, TotalEquity: 100},
	})
	var out overviewResponse
	ready := s.addOverviewAccounts("alice", []*account.Account{demo, main}, &out)
	if len(ready) != 2 {
		t.Fatalf("ready: %d", len(ready))
	}
	if out.Accounts[0].TotalEquity != nil || out.Accounts[0].Name != "Demo (testnet)" {
		t.Fatalf("testnet row: %+v", out.Accounts[0])
	}
	if out.Accounts[1].TotalEquity == nil || *out.Accounts[1].TotalEquity != 100 || out.Accounts[1].Name != "Main" {
		t.Fatalf("real row: %+v", out.Accounts[1])
	}
	out = overviewResponse{}
	s.addOverviewRealized([]*account.Account{demo}, &out)
	if out.RealizedToday != nil {
		t.Fatalf("realized on testnet alone: %v", *out.RealizedToday)
	}
}
