package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vergex/internal/market/instrument"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
	"vergex/internal/trader/manager"

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
		{TraderID: "stopped", ExchangeID: "a"},
		{TraderID: "active", ExchangeID: "a", IsRunning: true},
		{TraderID: "idle", ExchangeID: "b"},
	}
	runs := []overviewRun{{ExchangeID: "b", Symbol: "ETH-USDT-SWAP"}}
	cases := []struct{ account, symbol, want string }{
		{"a", "BTCUSDT", "active"}, // A running trader owns its account.
		{"b", "ETHUSDT", ""},       // An active plan owns its symbol.
		{"b", "BTCUSDT", "idle"},   // Otherwise the bound trader can close it.
		{"c", "BTCUSDT", ""},       // No trader at all: Frontier.
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
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	mine := storetest.AddTrader(t, st, &store.Trader{Name: "Mine"})
	storetest.AddTrader(t, st, &store.Trader{UserID: storetest.OtherUserID, Name: "Theirs"})
	ledger := storetest.NewLedger(t, st)
	ledger.Open(mine.ID, "BTCUSDT", store.SideLong, 1, 100, time.Now().Add(-time.Second))
	ledger.Close(mine.ID, "BTCUSDT", store.SideLong, 1, 110, 12.5, time.Now())
	// Realized today is the account's, so a Frontier trade on it counts too.
	eth, err := instrument.Ensure(st.GormDB(), "binance", instrument.KindPerpetual, "ETHUSDT", "ETHUSDT")
	if err != nil {
		t.Fatal(err)
	}
	frontier := store.Owner{Source: store.SourceFrontier, SourceID: "run"}
	for _, trade := range []store.Trade{
		{ExchangeTradeID: "f1", ExchangeOrderID: "f1", Side: store.SideBuy},
		{ExchangeTradeID: "f2", ExchangeOrderID: "f2", Side: store.SideSell, RealizedPnL: decimal.NewNullDecimal(decimal.RequireFromString("2.5"))},
	} {
		trade.AccountID, trade.InstrumentID, trade.PositionSide = ledger.Account, eth, store.SideLong
		trade.Price, trade.Quantity, trade.QuoteQuantity, trade.ExecutedAt = decimal.NewFromInt(100), decimal.NewFromInt(1), decimal.NewFromInt(100), time.Now()
		if _, err := st.Ledger().RecordTrade(trade, frontier); err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{store: st, traderManager: manager.NewTraderManager(), exchangeAccountStateCache: NewExchangeAccountStateCache()}
	// Known to be unreadable, so nothing tries to reach an exchange.
	exchanges, err := st.Exchange().List(storetest.UserID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]ExchangeAccountState{}
	for _, exchange := range exchanges {
		states[exchange.ID] = ExchangeAccountState{ExchangeID: exchange.ID, Status: exchangeAccountStatusInvalidCredentials}
	}
	s.exchangeAccountStateCache.Set(storetest.UserID, states)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", storetest.UserID) })
	r.GET("/overview", s.handleOverview)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/overview", nil))
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
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	exchange := &store.Exchange{ID: "bad", UserID: "alice", ExchangeType: "binance", AccountName: "Bad"}
	s := &Server{store: st, traderManager: manager.NewTraderManager(), exchangeAccountStateCache: NewExchangeAccountStateCache()}
	s.exchangeAccountStateCache.Set("alice", map[string]ExchangeAccountState{
		"bad": {ExchangeID: "bad", Status: exchangeAccountStatusInvalidCredentials, ErrorMessage: "invalid"},
	})

	var out overviewResponse
	ready := s.addOverviewAccounts("alice", []*store.Exchange{exchange}, &out)
	addOverviewExchangeData("alice", ready, &out)

	if len(ready) != 0 || !out.PositionsPartial {
		t.Fatalf("an unread account looked empty: ready=%d partial=%v", len(ready), out.PositionsPartial)
	}
	if len(out.Attention) != 1 || out.Attention[0].Kind != "account_invalid_credentials" {
		t.Fatalf("attention: %+v", out.Attention)
	}
}

func TestOverviewNamesTradesTheLedgerIsMissing(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	ledger := storetest.NewLedger(t, st)
	s := &Server{store: st}

	var out overviewResponse
	s.addOverviewSkippedFills(storetest.UserID, &out)
	if len(out.Attention) != 0 {
		t.Fatalf("attention with nothing missing: %+v", out.Attention)
	}

	record := func(id string, at time.Time) {
		t.Helper()
		fill := &store.SkippedFill{AccountID: ledger.Account, NativeSymbol: "OLD-USDT-SWAP", ExchangeTradeID: id, Detail: "sell 7 contracts at 3: gone", ExecutedAt: at}
		if _, err := st.Ledger().RecordSkippedFill(fill); err != nil {
			t.Fatal(err)
		}
		if err := st.GormDB().Model(fill).Update("recorded_at", at).Error; err != nil {
			t.Fatal(err)
		}
	}
	record("old", time.Now().Add(-skippedFillsShownFor-time.Hour))
	s.addOverviewSkippedFills(storetest.UserID, &out)
	if len(out.Attention) != 0 {
		t.Fatalf("an old one is still shown: %+v", out.Attention)
	}

	record("t1", time.Now())
	s.addOverviewSkippedFills(storetest.UserID, &out)
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
	s.addOverviewSkippedFills(storetest.UserID, &out)
	if len(out.Attention) != 1 || !strings.HasSuffix(out.Attention[0].Detail, "and more") {
		t.Fatalf("attention: %+v", out.Attention)
	}
}
