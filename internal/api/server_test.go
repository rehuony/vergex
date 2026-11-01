package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestPublicDecryptRouteNotRegistered is a security regression test: the
// unauthenticated POST /api/crypto/decrypt route was a decryption oracle and
// must never be re-registered. A built server's router must not route to it.
func TestPublicDecryptRouteNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New()}
	s.setupRoutes()

	for _, r := range s.router.Routes() {
		if r.Method == http.MethodPost && r.Path == "/api/crypto/decrypt" {
			t.Fatalf("SECURITY REGRESSION: public decryption oracle POST /api/crypto/decrypt is registered")
		}
	}

	// Also assert at the HTTP layer that the route is not handled.
	req := httptest.NewRequest(http.MethodPost, "/api/crypto/decrypt", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for POST /api/crypto/decrypt, got %d", w.Code)
	}
}
