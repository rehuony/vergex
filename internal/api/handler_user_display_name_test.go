package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/gin-gonic/gin"
)

func TestDisplayNameStartsAsAdminAndCanBeChanged(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })
	if err := st.User().Create(&store.User{ID: storetest.UserID, Email: "owner@example.test", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	s := &Server{store: st}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", storetest.UserID) })
	r.GET("/user/profile", s.handleGetProfile)
	r.PUT("/user/display-name", s.handleUpdateDisplayName)

	profileName := func() string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/profile", nil))
		var profile struct {
			DisplayName string `json:"display_name"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
			t.Fatalf("decode profile: %v", err)
		}
		return profile.DisplayName
	}
	rename := func(body string) int {
		req := httptest.NewRequest(http.MethodPut, "/user/display-name", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if got := profileName(); got != store.DefaultDisplayName {
		t.Fatalf("new account display name = %q, want %q", got, store.DefaultDisplayName)
	}
	if code := rename(`{"display_name":"  小明 Trader  "}`); code != http.StatusOK {
		t.Fatalf("rename status = %d, want 200", code)
	}
	if got := profileName(); got != "小明 Trader" {
		t.Fatalf("display name = %q, want trimmed %q", got, "小明 Trader")
	}
	for _, body := range []string{
		`{"display_name":"   "}`,
		`{"display_name":"` + strings.Repeat("名", 33) + `"}`,
		`{"display_name":"a\u0007b"}`,
	} {
		if code := rename(body); code != http.StatusBadRequest {
			t.Errorf("rename %s status = %d, want 400", body, code)
		}
	}
	if got := profileName(); got != "小明 Trader" {
		t.Fatalf("rejected renames changed the name to %q", got)
	}
}
