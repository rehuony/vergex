package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/security/auth"
	"vergex/internal/user"

	"github.com/gin-gonic/gin"
)

func TestChangePasswordNeedsTheCurrentOne(t *testing.T) {
	st := NewStores(testdb.Open(t))
	hash, _ := auth.HashPassword("old-password")
	if err := st.Users.Create(&user.User{ID: testdb.UserID, Email: "owner@example.test", PasswordHash: hash}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	s := &Server{stores: st, authLimiter: newIPRateLimiter(1, 100)}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	r.PUT("/user/password", s.handleChangePassword)

	change := func(body map[string]string) (int, string) {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/user/password", strings.NewReader(string(raw))))
		var resp struct {
			ErrorKey string `json:"error_key"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp.ErrorKey
	}
	passwordIs := func(p string) bool {
		user, _ := st.Users.GetByID(testdb.UserID)
		return auth.CheckPassword(p, user.PasswordHash)
	}

	if code, _ := change(map[string]string{"new_password": "new-password"}); code != http.StatusBadRequest {
		t.Errorf("without current password = %d, want 400", code)
	}
	code, key := change(map[string]string{"current_password": "wrong-password", "new_password": "new-password"})
	if code != http.StatusBadRequest || key != "current_password_incorrect" {
		t.Errorf("wrong current password = %d %q, want 400 current_password_incorrect", code, key)
	}
	if !passwordIs("old-password") {
		t.Fatal("a rejected change replaced the password")
	}

	if code, _ := change(map[string]string{"current_password": "old-password", "new_password": "new-password"}); code != http.StatusOK {
		t.Fatalf("change = %d, want 200", code)
	}
	if !passwordIs("new-password") {
		t.Fatal("password was not changed")
	}
}
