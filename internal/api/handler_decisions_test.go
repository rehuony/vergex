package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vergex/internal/store"
	"vergex/internal/testdb"

	"github.com/gin-gonic/gin"
)

func traderDataServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&store.Trader{}, &store.DecisionRecordDB{}, &store.EquitySnapshot{}); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, trader := range []*store.Trader{{ID: "alice-trader", UserID: "alice"}, {ID: "bob-trader", UserID: "bob"}} {
		if err := db.Create(trader).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{store: st}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "alice") })
	r.GET("/decisions/summary", s.handleDecisionSummaries)
	r.GET("/decisions/:id", s.handleDecisionDetail)
	return s, r
}

func getJSON(t *testing.T, r http.Handler, url string, out any) int {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	if out != nil && w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code
}

func TestDecisionSummariesOmitPromptsAndPageBackwards(t *testing.T) {
	s, r := traderDataServer(t)
	for i := 1; i <= 3; i++ {
		if err := s.store.Decision().LogDecision(&store.DecisionRecord{TraderID: "alice-trader", CycleNumber: i, SystemPrompt: "secret prompt", RawResponse: "raw"}); err != nil {
			t.Fatal(err)
		}
	}

	var page []store.DecisionRecord
	if code := getJSON(t, r, "/decisions/summary?trader_id=alice-trader&limit=2", &page); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(page) != 2 || page[0].CycleNumber != 3 || page[1].CycleNumber != 2 {
		t.Fatalf("expected the newest two, got %+v", page)
	}
	if page[0].SystemPrompt != "" || page[0].RawResponse != "" {
		t.Fatalf("summary carried stored prompts: %+v", page[0])
	}

	var older []store.DecisionRecord
	getJSON(t, r, fmt.Sprintf("/decisions/summary?trader_id=alice-trader&before=%d", page[1].ID), &older)
	if len(older) != 1 || older[0].CycleNumber != 1 {
		t.Fatalf("expected the oldest, got %+v", older)
	}

	var full store.DecisionRecord
	if code := getJSON(t, r, fmt.Sprintf("/decisions/%d?trader_id=alice-trader", page[0].ID), &full); code != http.StatusOK || full.SystemPrompt != "secret prompt" {
		t.Fatalf("detail: status %d, %+v", code, full)
	}
}

func TestDecisionDetailStaysWithinTheOwner(t *testing.T) {
	s, r := traderDataServer(t)
	theirs := &store.DecisionRecord{TraderID: "bob-trader", SystemPrompt: "bob's"}
	if err := s.store.Decision().LogDecision(theirs); err != nil {
		t.Fatal(err)
	}
	if code := getJSON(t, r, fmt.Sprintf("/decisions/%d?trader_id=bob-trader", theirs.ID), nil); code != http.StatusBadRequest {
		t.Fatalf("another account's trader: status %d", code)
	}
	if code := getJSON(t, r, fmt.Sprintf("/decisions/%d?trader_id=alice-trader", theirs.ID), nil); code != http.StatusNotFound {
		t.Fatalf("another trader's decision: status %d", code)
	}
}

func TestEquitySeriesIsThinnedAndKeepsTheLatest(t *testing.T) {
	s, _ := traderDataServer(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		if err := s.store.Equity().Save(&store.EquitySnapshot{TraderID: "alice-trader", Timestamp: start.Add(time.Duration(i) * time.Hour), TotalEquity: float64(100 + i)}); err != nil {
			t.Fatal(err)
		}
	}
	points, err := s.store.Equity().GetSeries("alice-trader", time.Time{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 5 || points[0].TotalEquity != 100 || points[4].TotalEquity != 119 {
		t.Fatalf("thinning lost the ends: %d points", len(points))
	}
	recent, err := s.store.Equity().GetSeries("alice-trader", start.Add(18*time.Hour), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("since filter returned %d points", len(recent))
	}
}
