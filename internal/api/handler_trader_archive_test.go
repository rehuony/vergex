package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/store"
	"vergex/internal/store/storetest"
	"vergex/internal/trader/manager"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func traderRoutes(t *testing.T, st *store.Store) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{store: st, traderManager: manager.NewTraderManager()}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", storetest.UserID) })
	r.DELETE("/traders/:id", s.handleDeleteTrader)
	r.PUT("/traders/:id", s.handleUpdateTrader)
	return r
}

func sendTo(r http.Handler, method, url, body string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, url, strings.NewReader(body)))
	return w.Code
}

func TestDeletingATraderKeepsItsHistoryAndOnlyTouchesYourOwn(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	mine := storetest.AddTrader(t, st, &store.Trader{Name: "Mine"})
	theirs := storetest.AddTrader(t, st, &store.Trader{UserID: storetest.OtherUserID, Name: "Theirs"})
	if err := st.Decision().LogDecision(&store.DecisionRecord{TraderID: mine.ID, CycleNumber: 1, Success: true}); err != nil {
		t.Fatal(err)
	}
	r := traderRoutes(t, st)

	if code := sendTo(r, http.MethodDelete, "/traders/"+theirs.ID, ""); code != http.StatusNotFound {
		t.Fatalf("another account's trader: status %d", code)
	}
	if _, err := st.Trader().GetByID(theirs.ID); err != nil {
		t.Fatalf("another account's trader was deleted: %v", err)
	}
	if code := sendTo(r, http.MethodDelete, "/traders/not-a-uuid", ""); code != http.StatusNotFound {
		t.Fatalf("an ID that is not one: status %d", code)
	}

	if code := sendTo(r, http.MethodDelete, "/traders/"+mine.ID, ""); code != http.StatusOK {
		t.Fatalf("own trader: status %d", code)
	}
	if traders, err := st.Trader().List(storetest.UserID); err != nil || len(traders) != 0 {
		t.Fatalf("traders = %+v, err %v", traders, err)
	}
	if records, err := st.Decision().GetSummaries(mine.ID, 10, 0); err != nil || len(records) != 1 {
		t.Fatalf("history = %+v, err %v", records, err)
	}
	if code := sendTo(r, http.MethodDelete, "/traders/"+mine.ID, ""); code != http.StatusNotFound {
		t.Fatalf("deleted twice: status %d", code)
	}
}

func TestUpdatingATraderChecksWhatItPointsAt(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	trader := storetest.AddTrader(t, st, &store.Trader{Name: "Mine"})
	r := traderRoutes(t, st)

	body := func(model, account, strategy string) string {
		return `{"name":"Mine","agent_model_id":"` + model + `","exchange_id":"` + account + `","strategy_id":"` + strategy + `"}`
	}
	for name, request := range map[string]string{
		"model":    body(uuid.NewString(), trader.ExchangeID, trader.StrategyID),
		"account":  body(trader.AgentModelID, uuid.NewString(), trader.StrategyID),
		"strategy": body(trader.AgentModelID, trader.ExchangeID, uuid.NewString()),
		"garbage":  body("not-a-uuid", trader.ExchangeID, trader.StrategyID),
	} {
		if code := sendTo(r, http.MethodPut, "/traders/"+trader.ID, request); code != http.StatusBadRequest {
			t.Errorf("%s that does not exist: status %d", name, code)
		}
	}
}
