package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
	"vergex/internal/trader/manager"
)

// History lives in the database, so it must stay readable when the trader
// could not be loaded into memory (for example, its exchange is unreachable).
func TestHistoryReadsDoNotNeedALoadedTrader(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	trader := storetest.AddTrader(t, st, &store.Trader{Name: "bot"})
	other := storetest.AddTrader(t, st, &store.Trader{UserID: storetest.OtherUserID, Name: "theirs"})
	if err := st.Decision().LogDecision(&store.DecisionRecord{TraderID: trader.ID, CycleNumber: 1, Success: true}); err != nil {
		t.Fatalf("log decision: %v", err)
	}
	ledger := storetest.NewLedger(t, st)
	ledger.Open(trader.ID, "BTCUSDT", store.SideLong, 0.01, 80000, time.Now().Add(-time.Minute))
	ledger.Close(trader.ID, "BTCUSDT", store.SideLong, 0.01, 81000, 10, time.Now())

	// The manager has never loaded it.
	s := &Server{store: st, traderManager: manager.NewTraderManager()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", storetest.UserID) })
	r.GET("/decisions/latest", s.handleLatestDecisions)
	r.GET("/positions/history", s.handlePositionHistory)
	r.GET("/statistics/full", s.handleStatisticsFull)
	r.GET("/my-traders/metrics", s.handleTraderMetrics)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	w := get("/decisions/latest?trader_id=" + trader.ID)
	var decisions []store.DecisionRecord
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &decisions) != nil || len(decisions) != 1 {
		t.Fatalf("decisions/latest = %d %s, want one record", w.Code, w.Body.String())
	}

	w = get("/my-traders/metrics")
	var metrics map[string]struct {
		TotalTrades int      `json:"total_trades"`
		Equity      *float64 `json:"equity"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &metrics) != nil || metrics[trader.ID].TotalTrades != 1 {
		t.Fatalf("my-traders/metrics = %d %s, want one trade", w.Code, w.Body.String())
	}
	if metrics[trader.ID].Equity != nil {
		t.Fatalf("no snapshot was recorded, but equity = %v", *metrics[trader.ID].Equity)
	}

	w = get("/positions/history?trader_id=" + trader.ID)
	var history struct {
		Positions []store.TraderPosition `json:"positions"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &history) != nil || len(history.Positions) != 1 {
		t.Fatalf("positions/history = %d %s, want one closed position", w.Code, w.Body.String())
	}

	w = get("/statistics/full?trader_id=" + trader.ID)
	var stats struct {
		TotalTrades int `json:"total_trades"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &stats) != nil || stats.TotalTrades != 1 {
		t.Fatalf("statistics/full = %d %s, want one trade", w.Code, w.Body.String())
	}

	if w := get("/decisions/latest?trader_id=" + other.ID); w.Code != http.StatusBadRequest {
		t.Fatalf("another account's trader = %d, want 400", w.Code)
	}
}
