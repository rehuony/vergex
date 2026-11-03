package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/trader"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	aliceTrader = "00000000-0000-4000-8000-0000000000a1"
	bobTrader   = "00000000-0000-4000-8000-0000000000a2"
)

// conversationOf keeps a cycle's exchange with the model, as the trader does.
func conversationOf(t *testing.T, st *Stores, user, traderID, prompt string) string {
	t.Helper()
	conversation := agent.Conversation{ID: uuid.NewString(), UserID: user, Feature: agent.FeatureTrader}
	if err := st.Conversations.Open(&conversation); err != nil {
		t.Fatal(err)
	}
	err := st.Conversations.Append(conversation.ID,
		agent.ConversationMessage{Role: agent.RoleSystem, Content: prompt},
		agent.ConversationMessage{Role: agent.RoleUser, Content: "the market"},
		agent.ConversationMessage{Role: agent.RoleAssistant, Content: "raw", Thinking: "because"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return conversation.ID
}

func traderDataServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st := NewStores(testdb.Open(t))
	testdb.AddTrader(t, st.DB, &testdb.Trader{ID: aliceTrader, UserID: testdb.UserID})
	testdb.AddTrader(t, st.DB, &testdb.Trader{ID: bobTrader, UserID: testdb.OtherUserID})
	s := &Server{stores: st}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	r.GET("/traders/:id/decisions/summary", s.handleDecisionSummaries)
	r.GET("/traders/:id/decisions/:decisionId", s.handleDecisionDetail)
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
		if err := s.stores.Decisions.LogDecision(&trader.DecisionRecord{TraderID: aliceTrader, Seq: i, ConversationID: conversationOf(t, s.stores, testdb.UserID, aliceTrader, "secret prompt")}); err != nil {
			t.Fatal(err)
		}
	}

	var page []trader.DecisionRecord
	if code := getJSON(t, r, "/traders/"+aliceTrader+"/decisions/summary?limit=2", &page); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(page) != 2 || page[0].Seq != 3 || page[1].Seq != 2 {
		t.Fatalf("expected the newest two, got %+v", page)
	}
	if page[0].SystemPrompt != "" || page[0].RawResponse != "" {
		t.Fatalf("summary carried stored prompts: %+v", page[0])
	}

	var older []trader.DecisionRecord
	getJSON(t, r, fmt.Sprintf("/traders/%s/decisions/summary?before=%d", aliceTrader, page[1].ID), &older)
	if len(older) != 1 || older[0].Seq != 1 {
		t.Fatalf("expected the oldest, got %+v", older)
	}

	var full trader.DecisionRecord
	if code := getJSON(t, r, fmt.Sprintf("/traders/%s/decisions/%d", aliceTrader, page[0].ID), &full); code != http.StatusOK || full.SystemPrompt != "secret prompt" || full.RawResponse != "raw" || full.CoTTrace != "because" || full.InputPrompt != "the market" {
		t.Fatalf("detail: status %d, %+v", code, full)
	}
}

func TestDecisionDetailStaysWithinTheOwner(t *testing.T) {
	s, r := traderDataServer(t)
	theirs := &trader.DecisionRecord{TraderID: bobTrader}
	if err := s.stores.Decisions.LogDecision(theirs); err != nil {
		t.Fatal(err)
	}
	if code := getJSON(t, r, fmt.Sprintf("/traders/%s/decisions/%d", bobTrader, theirs.ID), nil); code != http.StatusBadRequest {
		t.Fatalf("another account's trader: status %d", code)
	}
	if code := getJSON(t, r, fmt.Sprintf("/traders/%s/decisions/%d", aliceTrader, theirs.ID), nil); code != http.StatusNotFound {
		t.Fatalf("another trader's decision: status %d", code)
	}
}

func TestEquitySeriesIsThinnedAndKeepsTheLatest(t *testing.T) {
	s, _ := traderDataServer(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		if err := s.stores.Equity.Save(&trader.EquitySnapshot{TraderID: aliceTrader, Timestamp: start.Add(time.Duration(i) * time.Hour), TotalEquity: float64(100 + i)}); err != nil {
			t.Fatal(err)
		}
	}
	points, err := s.stores.Equity.GetSeries(aliceTrader, time.Time{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 5 || points[0].TotalEquity != 100 || points[4].TotalEquity != 119 {
		t.Fatalf("thinning lost the ends: %d points", len(points))
	}
	recent, err := s.stores.Equity.GetSeries(aliceTrader, start.Add(18*time.Hour), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("since filter returned %d points", len(recent))
	}
}
