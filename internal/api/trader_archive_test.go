package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func traderRoutes(t *testing.T, st *Stores) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{stores: st, traderManager: trader.NewManager(trader.NewStores(st.DB))}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	r.DELETE("/traders/:id", s.handleDeleteTrader)
	r.PUT("/traders/:id", s.handleUpdateTrader)
	r.GET("/traders/presets", s.handleTraderPresets)
	r.POST("/traders/dry-run", s.handleTraderDryRun)
	r.POST("/traders/:id/duplicate", s.handleDuplicateTrader)
	return r
}

func sendTo(r http.Handler, method, url, body string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, url, strings.NewReader(body)))
	return w.Code
}

func TestDeletingATraderKeepsItsHistoryAndOnlyTouchesYourOwn(t *testing.T) {
	st := NewStores(testdb.Open(t))
	mine := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine"})
	theirs := testdb.AddTrader(t, st.DB, &testdb.Trader{UserID: testdb.OtherUserID, Name: "Theirs"})
	if err := st.Decisions.LogDecision(&trader.DecisionRecord{TraderID: mine.ID, CycleNumber: 1, Success: true}); err != nil {
		t.Fatal(err)
	}
	r := traderRoutes(t, st)

	if code := sendTo(r, http.MethodDelete, "/traders/"+theirs.ID, ""); code != http.StatusNotFound {
		t.Fatalf("another account's trader: status %d", code)
	}
	if _, err := st.Traders.GetByID(theirs.ID); err != nil {
		t.Fatalf("another account's trader was deleted: %v", err)
	}
	if code := sendTo(r, http.MethodDelete, "/traders/not-a-uuid", ""); code != http.StatusNotFound {
		t.Fatalf("an ID that is not one: status %d", code)
	}

	if code := sendTo(r, http.MethodDelete, "/traders/"+mine.ID, ""); code != http.StatusOK {
		t.Fatalf("own trader: status %d", code)
	}
	if traders, err := st.Traders.List(testdb.UserID); err != nil || len(traders) != 0 {
		t.Fatalf("traders = %+v, err %v", traders, err)
	}
	if records, err := st.Decisions.GetSummaries(mine.ID, 10, 0); err != nil || len(records) != 1 {
		t.Fatalf("history = %+v, err %v", records, err)
	}
	if code := sendTo(r, http.MethodDelete, "/traders/"+mine.ID, ""); code != http.StatusNotFound {
		t.Fatalf("deleted twice: status %d", code)
	}
}

func TestUpdatingATraderChecksWhatItPointsAt(t *testing.T) {
	st := NewStores(testdb.Open(t))
	trader := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine"})
	r := traderRoutes(t, st)

	body := func(model, account string) string {
		return `{"name":"Mine","agent_model_id":"` + model + `","account_id":"` + account + `"}`
	}
	for name, request := range map[string]string{
		"model":   body(uuid.NewString(), trader.AccountID),
		"account": body(trader.AgentModelID, uuid.NewString()),
		"garbage": body("not-a-uuid", trader.AccountID),
	} {
		if code := sendTo(r, http.MethodPut, "/traders/"+trader.ID, request); code != http.StatusBadRequest {
			t.Errorf("%s that does not exist: status %d", name, code)
		}
	}
}
