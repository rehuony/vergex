package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"vergex/internal/manager"
	"vergex/internal/store"
)

// History lives in the database, so it must stay readable when the trader
// could not be loaded into memory (for example, its exchange is unreachable).
func TestHistoryReadsDoNotNeedALoadedTrader(t *testing.T) {
	st, err := store.New(t.TempDir() + "/vergex.db")
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const userID = "owner"
	trader := &store.Trader{ID: "trader-1", UserID: userID, Name: "bot", AIModelID: "model-1", ExchangeID: "exchange-1", InitialBalance: 1000}
	if err := st.Trader().Create(trader); err != nil {
		t.Fatalf("create trader: %v", err)
	}
	other := &store.Trader{ID: "trader-2", UserID: "someone-else", Name: "theirs", AIModelID: "model-2", ExchangeID: "exchange-2", InitialBalance: 1000}
	if err := st.Trader().Create(other); err != nil {
		t.Fatalf("create other trader: %v", err)
	}
	if err := st.Decision().LogDecision(&store.DecisionRecord{TraderID: trader.ID, CycleNumber: 1, Success: true}); err != nil {
		t.Fatalf("log decision: %v", err)
	}
	pos := &store.TraderPosition{
		TraderID: trader.ID, ExchangeID: "exchange-1", Symbol: "BTCUSDT", Side: "LONG",
		Quantity: 0.01, EntryPrice: 80000, EntryTime: time.Now().Add(-time.Minute).UnixMilli(),
	}
	if err := st.Position().Create(pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	if err := st.Position().ClosePosition(pos.ID, 81000, "exit-1", 10, 0.1, "take_profit"); err != nil {
		t.Fatalf("close position: %v", err)
	}

	// The manager has never loaded trader-1.
	s := &Server{store: st, traderManager: manager.NewTraderManager()}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", userID) })
	r.GET("/decisions/latest", s.handleLatestDecisions)
	r.GET("/positions/history", s.handlePositionHistory)
	r.GET("/statistics/full", s.handleStatisticsFull)
	r.GET("/my-traders/metrics", s.handleTraderMetrics)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	w := get("/decisions/latest?trader_id=trader-1")
	var decisions []store.DecisionRecord
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &decisions) != nil || len(decisions) != 1 {
		t.Fatalf("decisions/latest = %d %s, want one record", w.Code, w.Body.String())
	}

	w = get("/my-traders/metrics")
	var metrics map[string]struct {
		TotalTrades int      `json:"total_trades"`
		Equity      *float64 `json:"equity"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &metrics) != nil || metrics["trader-1"].TotalTrades != 1 {
		t.Fatalf("my-traders/metrics = %d %s, want one trade", w.Code, w.Body.String())
	}
	if metrics["trader-1"].Equity != nil {
		t.Fatalf("no snapshot was recorded, but equity = %v", *metrics["trader-1"].Equity)
	}

	w = get("/positions/history?trader_id=trader-1")
	var history struct {
		Positions []store.TraderPosition `json:"positions"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &history) != nil || len(history.Positions) != 1 {
		t.Fatalf("positions/history = %d %s, want one closed position", w.Code, w.Body.String())
	}

	w = get("/statistics/full?trader_id=trader-1")
	var stats struct {
		TotalTrades int `json:"total_trades"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &stats) != nil || stats.TotalTrades != 1 {
		t.Fatalf("statistics/full = %d %s, want one trade", w.Code, w.Body.String())
	}

	if w := get("/decisions/latest?trader_id=trader-2"); w.Code != http.StatusBadRequest {
		t.Fatalf("another account's trader = %d, want 400", w.Code)
	}
}
