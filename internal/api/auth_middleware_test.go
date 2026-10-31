package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"vergex/internal/auth"
	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddlewareRejectsTokensOfMissingAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth.SetJWTSecret("test-secret-long-enough-for-hs256-signing")
	st := storetest.New(t)
	if err := st.User().Create(&store.User{ID: "present", Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}
	r := gin.New()
	r.GET("/", s.authMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for userID, want := range map[string]int{"present": http.StatusOK, "gone": http.StatusUnauthorized} {
		token, err := auth.GenerateJWT(userID, "me@example.com")
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("user %q: status = %d, want %d", userID, w.Code, want)
		}
	}
}
