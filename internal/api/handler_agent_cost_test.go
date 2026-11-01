package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func TestAgentCostsRequireTraderOwnership(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	trader := storetest.AddTrader(t, st, &store.Trader{Name: "A"})
	if err := st.AgentCall().Record(&store.AgentCall{Feature: store.FeatureTrader, SourceID: trader.ID, Model: "m", Provider: "p", CostUSD: 0.5}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	s := &Server{store: st}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	router.GET("/agent-costs", s.handleGetAgentCosts)
	get := func(user, traderID string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/agent-costs?trader_id="+traderID, nil)
		r.Header.Set("Test-User", user)
		router.ServeHTTP(w, r)
		return w.Code
	}

	if code := get(storetest.UserID, trader.ID); code != http.StatusOK {
		t.Fatalf("owner got %d", code)
	}
	if code := get(storetest.OtherUserID, trader.ID); code != http.StatusNotFound {
		t.Fatalf("other user got %d, want 404", code)
	}
	if code := get(storetest.UserID, "missing"); code != http.StatusNotFound {
		t.Fatalf("unknown trader got %d, want 404", code)
	}
}
