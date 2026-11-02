package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/security/auth"
	"vergex/internal/user"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddlewareRejectsTokensOfMissingAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth.SetJWTSecret("test-secret-long-enough-for-hs256-signing")
	st := NewStores(testdb.Open(t))
	if err := st.Users.Create(&user.User{ID: testdb.UserID, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{stores: st}
	r := gin.New()
	r.GET("/", s.authMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for userID, want := range map[string]int{testdb.UserID: http.StatusOK, "gone": http.StatusUnauthorized} {
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
