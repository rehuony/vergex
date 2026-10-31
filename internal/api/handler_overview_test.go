package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vergex/internal/manager"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/gin-gonic/gin"
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
	for _, trader := range []*store.Trader{
		{ID: "mine", UserID: "alice", Name: "Mine", AIModelID: "m", ExchangeID: "e", InitialBalance: 100},
		{ID: "theirs", UserID: "bob", Name: "Theirs", AIModelID: "m", ExchangeID: "e", InitialBalance: 100},
	} {
		if err := st.Trader().Create(trader); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UnixMilli()
	position := &store.TraderPosition{TraderID: "mine", ExchangeID: "e", Symbol: "BTCUSDT", Side: "LONG", Quantity: 1, EntryPrice: 100, EntryTime: now - 1000}
	if err := st.Position().Create(position); err != nil {
		t.Fatal(err)
	}
	if err := st.Position().ClosePosition(position.ID, 110, "exit", 12.5, 0.1, "take_profit"); err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, traderManager: manager.NewTraderManager(), exchangeAccountStateCache: NewExchangeAccountStateCache()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "alice") })
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
	if len(body.Traders) != 1 || body.Traders[0].TraderID != "mine" {
		t.Fatalf("traders: %+v", body.Traders)
	}
	if body.RealizedToday == nil || *body.RealizedToday != 12.5 {
		t.Fatalf("realized today: %v", body.RealizedToday)
	}
	if body.PositionsPartial {
		t.Fatal("with no account, there is nothing left unread")
	}
}

func TestOverviewAccountThatCannotBeReadIsNotReportedAsEmpty(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	exchange := &store.Exchange{ID: "bad", UserID: "alice", ExchangeType: "binance", Name: "Bad", Enabled: true}
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
