package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/database/testdb"

	"github.com/gin-gonic/gin"
)

func TestRoutesAreGroupedByModule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New(), cryptoHandler: &CryptoHandler{}}
	s.setupRoutes()
	registered := map[string]bool{}
	for _, route := range s.router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/accounts", "POST /api/accounts", "PUT /api/accounts", "GET /api/accounts/state",
		"GET /api/accounts/:id", "DELETE /api/accounts/:id", "POST /api/accounts/:id/verify",
		"GET /api/accounts/:id/positions", "GET /api/accounts/:id/open-orders", "GET /api/accounts/:id/orders", "GET /api/accounts/:id/fills",
		"GET /api/agent/models", "PUT /api/agent/models", "POST /api/agent/models/test", "POST /api/agent/models/available", "GET /api/agent/models/:id/available", "GET /api/agent/providers", "GET /api/agent/costs", "GET /api/agent/costs/summary", "GET /api/agent/storage",
		"GET /api/traders", "POST /api/traders", "GET /api/traders/metrics", "POST /api/traders/preflight", "GET /api/traders/presets",
		"POST /api/traders/dry-run", "POST /api/traders/estimate-tokens", "POST /api/traders/:id/duplicate",
		"GET /api/traders/:id/status", "GET /api/traders/:id/account", "GET /api/traders/:id/open-positions", "GET /api/traders/:id/positions",
		"GET /api/traders/:id/orders", "GET /api/traders/:id/orders/:orderId/fills", "GET /api/traders/:id/open-orders", "GET /api/traders/:id/trades",
		"GET /api/traders/:id/decisions", "GET /api/traders/:id/decisions/latest", "GET /api/traders/:id/decisions/summary", "GET /api/traders/:id/decisions/:decisionId",
		"GET /api/traders/:id/equity", "GET /api/traders/:id/statistics", "GET /api/traders/:id/statistics/full",
		"POST /api/futures/chat", "GET /api/futures/candles", "POST /api/futures/approve", "POST /api/futures/execute",
		"GET /api/market/status", "GET /api/market/settings",
		"GET /api/dashboard/overview", "GET /api/dashboard/events",
		"GET /api/system/config", "GET /api/system/ip",
		"GET /api/forecast/strategies", "GET /api/telegram", "GET /api/user/profile",
	} {
		if !registered[want] {
			t.Errorf("%s is not registered", want)
		}
	}
	for _, gone := range []string{
		"GET /api/strategies", "GET /api/my-traders", "GET /api/exchanges", "GET /api/models", "GET /api/supported-exchanges", "GET /api/supported-models",
		"GET /api/positions", "GET /api/orders", "GET /api/open-orders", "GET /api/trades", "GET /api/decisions", "GET /api/equity", "GET /api/statistics",
		"GET /api/account", "GET /api/status", "GET /api/config", "GET /api/server-ip", "GET /api/overview", "GET /api/activity/stream",
		"GET /api/planning/candles", "GET /api/market-data/status", "POST /api/launch/preflight", "GET /api/agent-costs",
	} {
		if registered[gone] {
			t.Errorf("%s should be gone", gone)
		}
	}
}

func TestAnAccountsLedgerIsItsOwnersAlone(t *testing.T) {
	st := NewStores(testdb.Open(t))
	ledger := accounttest.NewLedger(t, st.DB)
	ledger.Leverage = 3
	now := time.Now().UTC()
	ledger.Open("bot", "BTCUSDT", account.SideLong, 1, 100, now.Add(-time.Minute))
	ledger.Close("bot", "BTCUSDT", account.SideLong, 1, 110, 10, now)

	gin.SetMode(gin.TestMode)
	s := &Server{stores: st}
	route := func(user string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_id", user) })
		r.GET("/accounts/:id/orders", s.handleAccountOrders)
		r.GET("/accounts/:id/fills", s.handleAccountFills)
		return r
	}
	read := func(user, path string, out any) int {
		w := httptest.NewRecorder()
		route(user).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if out != nil && w.Code == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code
	}

	var orders, fills []map[string]any
	if code := read(testdb.UserID, "/accounts/"+ledger.Account+"/orders", &orders); code != http.StatusOK || len(orders) == 0 {
		t.Fatalf("orders: status %d, %v", code, orders)
	}
	if code := read(testdb.UserID, "/accounts/"+ledger.Account+"/fills?symbol=btcusdt", &fills); code != http.StatusOK || len(fills) != 2 {
		t.Fatalf("fills: status %d, %v", code, fills)
	}
	if code := read(testdb.UserID, "/accounts/"+ledger.Account+"/fills?symbol=ETHUSDT", &fills); code != http.StatusOK || len(fills) != 0 {
		t.Fatalf("fills of another symbol: status %d, %v", code, fills)
	}
	if code := read(testdb.OtherUserID, "/accounts/"+ledger.Account+"/orders", nil); code != http.StatusNotFound {
		t.Fatalf("another user's orders: status %d", code)
	}
	if code := read(testdb.OtherUserID, "/accounts/"+ledger.Account+"/fills", nil); code != http.StatusNotFound {
		t.Fatalf("another user's fills: status %d", code)
	}
}
