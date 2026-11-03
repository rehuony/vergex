package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"vergex/internal/account"
	"vergex/internal/account/accounttest"
	"vergex/internal/database/testdb"
	"vergex/internal/trader"
)

// History lives in the database, so it must stay readable when the trader
// could not be loaded into memory (for example, its exchange is unreachable).
func TestHistoryReadsDoNotNeedALoadedTrader(t *testing.T) {
	st := NewStores(testdb.Open(t))

	tr := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "bot"})
	other := testdb.AddTrader(t, st.DB, &testdb.Trader{UserID: testdb.OtherUserID, Name: "theirs"})
	if err := st.Decisions.LogDecision(&trader.DecisionRecord{TraderID: tr.ID, Seq: 1, Success: true}); err != nil {
		t.Fatalf("log decision: %v", err)
	}
	ledger := accounttest.NewLedger(t, st.DB)
	ledger.Open(tr.ID, "BTCUSDT", account.SideLong, 0.01, 80000, time.Now().Add(-time.Minute))
	ledger.Close(tr.ID, "BTCUSDT", account.SideLong, 0.01, 81000, 10, time.Now())

	// The manager has never loaded it.
	s := &Server{stores: st, traderManager: trader.NewManager(trader.NewStores(st.DB))}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	r.GET("/traders/:id/decisions/latest", s.handleLatestDecisions)
	r.GET("/traders/:id/positions", s.handlePositionHistory)
	r.GET("/traders/:id/statistics/full", s.handleStatisticsFull)
	r.GET("/traders/metrics", s.handleTraderMetrics)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	w := get("/traders/" + tr.ID + "/decisions/latest")
	var decisions []trader.DecisionRecord
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &decisions) != nil || len(decisions) != 1 {
		t.Fatalf("decisions/latest = %d %s, want one record", w.Code, w.Body.String())
	}

	w = get("/traders/metrics")
	var metrics map[string]struct {
		TotalTrades int      `json:"total_trades"`
		Equity      *float64 `json:"equity"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &metrics) != nil || metrics[tr.ID].TotalTrades != 1 {
		t.Fatalf("my-traders/metrics = %d %s, want one trade", w.Code, w.Body.String())
	}
	if metrics[tr.ID].Equity != nil {
		t.Fatalf("no snapshot was recorded, but equity = %v", *metrics[tr.ID].Equity)
	}

	w = get("/traders/" + tr.ID + "/positions")
	var history struct {
		Positions []account.TraderPosition `json:"positions"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &history) != nil || len(history.Positions) != 1 {
		t.Fatalf("positions/history = %d %s, want one closed position", w.Code, w.Body.String())
	}

	w = get("/traders/" + tr.ID + "/statistics/full")
	var stats struct {
		TotalTrades int `json:"total_trades"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &stats) != nil || stats.TotalTrades != 1 {
		t.Fatalf("statistics/full = %d %s, want one trade", w.Code, w.Body.String())
	}

	if w := get("/traders/" + other.ID + "/decisions/latest"); w.Code != http.StatusBadRequest {
		t.Fatalf("another account's trader = %d, want 400", w.Code)
	}
}
