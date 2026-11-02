package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFuturesPrivateRoutesRequireLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New()}
	s.setupRoutes()
	for _, r := range []struct{ method, path string }{{"GET", "/api/futures/state"}, {"GET", "/api/futures/account"}, {"POST", "/api/futures/approve"}, {"POST", "/api/futures/execute"}, {"POST", "/api/futures/chat"}, {"POST", "/api/futures/compile"}, {"POST", "/api/futures/stop"}, {"POST", "/api/futures/reconcile"}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(r.method, r.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want401", r.method, r.path, w.Code)
		}
	}
}

func TestFuturesShutdownDoesNotInitializeService(t *testing.T) {
	s := &Server{}
	if e := s.Shutdown(); e != nil {
		t.Fatal(e)
	}
	if p, e := s.futuresService(); e == nil || p != nil {
		t.Fatal("service initialized after shutdown")
	}
}

func TestFuturesApprovalRequiresExplicitClick(t *testing.T) {
	s := &Server{}
	router := gin.New()
	router.POST("/approve", s.handleFuturesApprove)
	for _, body := range []string{`{}`, `{"confirmed":false}`, `{"confirmation":"APPROVE LIVE"}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/approve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "explicit click") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestFuturesMarketDoesNotRequireExecutionAccount(t *testing.T) {
	s := &Server{}
	for _, venue := range []string{"binance", "okx"} {
		workspace, err := s.futuresWorkspace("alice", "", venue)
		if err != nil || workspace.ID != "" || workspace.Venue != venue {
			t.Fatalf("%+v %v", workspace, err)
		}
	}
	if _, err := s.futuresWorkspace("alice", "", "unknown"); err == nil {
		t.Fatal("unsupported venue accepted")
	}
	// Execution still requires a real account; the empty planning binding is
	// not promoted to an account or an execution broker.
	workspace, _ := s.futuresWorkspace("alice", "", "binance")
	if workspace.APIKey != "" {
		t.Fatal("planning workspace has trading authority")
	}
}
