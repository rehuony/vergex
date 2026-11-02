package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vergex/internal/account"
	"vergex/internal/database/testdb"
)

func TestVerifyAccountChecksOneAccountPastTheCache(t *testing.T) {
	st := NewStores(testdb.Open(t))
	owner := testdb.AddUser(t, st.DB)
	id, err := st.Accounts.Create(owner, "binance", "Main", "fixture-key", "fixture-secret", "")
	if err != nil {
		t.Fatalf("create exchange: %v", err)
	}
	// An account with a credential missing is answered without calling the exchange.
	if err := st.DB.Model(&account.Account{}).Where("id = ?", id).Update("secret_key", "").Error; err != nil {
		t.Fatalf("blank the secret: %v", err)
	}

	s := &Server{stores: st, accountStateCache: NewAccountStateCache()}
	s.accountStateCache.Set(owner, map[string]AccountState{
		id: {AccountID: id, Status: accountStatusOK},
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", owner) })
	r.POST("/accounts/:id/verify", s.handleVerifyAccount)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts/"+id+"/verify", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var state AccountState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.AccountID != id || state.Status != accountStatusMissingCredentials {
		t.Fatalf("state = %+v, want %s missing credentials", state, id)
	}
	cached, ok := s.accountStateCache.Get(owner)
	if !ok || cached[id].Status != accountStatusMissingCredentials {
		t.Fatalf("cached state = %+v, want the fresh check", cached[id])
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts/missing/verify", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown account status = %d, want 404", w.Code)
	}
}
