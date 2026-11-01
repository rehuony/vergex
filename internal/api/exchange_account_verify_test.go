package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func TestVerifyExchangeAccountChecksOneAccountPastTheCache(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	owner := storetest.Account(t, st)
	id, err := st.Exchange().Create(owner, "binance", "Main", "fixture-key", "fixture-secret", "")
	if err != nil {
		t.Fatalf("create exchange: %v", err)
	}
	// An account with a credential missing is answered without calling the exchange.
	if err := st.GormDB().Model(&store.Exchange{}).Where("id = ?", id).Update("secret_key", "").Error; err != nil {
		t.Fatalf("blank the secret: %v", err)
	}

	s := &Server{store: st, exchangeAccountStateCache: NewExchangeAccountStateCache()}
	s.exchangeAccountStateCache.Set(owner, map[string]ExchangeAccountState{
		id: {ExchangeID: id, Status: exchangeAccountStatusOK},
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", owner) })
	r.POST("/exchanges/:id/verify", s.handleVerifyExchangeAccount)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/exchanges/"+id+"/verify", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var state ExchangeAccountState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.ExchangeID != id || state.Status != exchangeAccountStatusMissingCredentials {
		t.Fatalf("state = %+v, want %s missing credentials", state, id)
	}
	cached, ok := s.exchangeAccountStateCache.Get(owner)
	if !ok || cached[id].Status != exchangeAccountStatusMissingCredentials {
		t.Fatalf("cached state = %+v, want the fresh check", cached[id])
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/exchanges/missing/verify", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown account status = %d, want 404", w.Code)
	}
}
