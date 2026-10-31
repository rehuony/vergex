package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func TestAICostsRequireTraderOwnership(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Trader().Create(&store.Trader{ID: "alice-trader", UserID: "alice", Name: "A", AIModelID: "m", ExchangeID: "e", InitialBalance: 100}); err != nil {
		t.Fatal(err)
	}
	if err := st.AICharge().Record(&store.AICharge{TraderID: "alice-trader", Model: "m", Provider: "p", CostUSD: 0.5}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	s := &Server{store: st}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	router.GET("/ai-costs", s.handleGetAICosts)
	get := func(user, traderID string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/ai-costs?trader_id="+traderID, nil)
		r.Header.Set("Test-User", user)
		router.ServeHTTP(w, r)
		return w.Code
	}

	if code := get("alice", "alice-trader"); code != http.StatusOK {
		t.Fatalf("owner got %d", code)
	}
	if code := get("bob", "alice-trader"); code != http.StatusNotFound {
		t.Fatalf("other user got %d, want 404", code)
	}
	if code := get("alice", "missing"); code != http.StatusNotFound {
		t.Fatalf("unknown trader got %d, want 404", code)
	}
}
