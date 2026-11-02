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
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
)

// Two traders on one account, and a jump in its equity that neither of them made.
func TestATradersReturnIsItsOwn(t *testing.T) {
	st := NewStores(testdb.Open(t))
	ledger := accounttest.NewLedger(t, st.DB)
	winner := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Winner", AccountID: ledger.Account, InitialBalance: 1000})
	loser := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Loser", AccountID: ledger.Account, InitialBalance: 1000})

	at := time.Now().Add(-time.Hour)
	ledger.Open(winner.ID, "BTCUSDT", account.SideLong, 1, 100, at)
	ledger.Close(winner.ID, "BTCUSDT", account.SideLong, 1, 150, 50, at.Add(time.Minute))
	ledger.Open(loser.ID, "ETHUSDT", account.SideLong, 1, 100, at)
	ledger.Close(loser.ID, "ETHUSDT", account.SideLong, 1, 80, -20, at.Add(time.Minute))
	traders := trader.NewStores(st.DB)
	for _, id := range []string{winner.ID, loser.ID} {
		if err := traders.Equity.Save(&trader.EquitySnapshot{TraderID: id, Timestamp: time.Now().UTC(), TotalEquity: 9000}); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	s := &Server{stores: st, traderManager: trader.NewManager(traders)}
	router.GET("/metrics", s.handleTraderMetrics)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics = %d %s", w.Code, w.Body)
	}
	var metrics map[string]struct {
		Equity    float64 `json:"equity"`
		ReturnPct float64 `json:"return_pct"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &metrics); err != nil {
		t.Fatal(err)
	}
	if got := metrics[winner.ID]; got.ReturnPct != 5 || got.Equity != 9000 {
		t.Errorf("winner = %+v", got)
	}
	if got := metrics[loser.ID]; got.ReturnPct != -2 {
		t.Errorf("loser = %+v", got)
	}
}
