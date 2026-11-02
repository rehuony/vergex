package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/user"

	"github.com/gin-gonic/gin"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestAvatarUploadsAreRedrawnAndRejectNonImages(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Users.Create(&user.User{ID: testdb.UserID, Email: "owner@example.test", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	s := &Server{stores: st}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", testdb.UserID) })
	r.GET("/user/profile", s.handleGetProfile)
	r.PUT("/user/avatar", s.handleUpdateAvatar)
	r.DELETE("/user/avatar", s.handleRemoveAvatar)

	upload := func(data []byte) int {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, _ := form.CreateFormFile("avatar", "avatar.png")
		_, _ = part.Write(data)
		_ = form.Close()
		req := httptest.NewRequest(http.MethodPut, "/user/avatar", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	profileAvatar := func() string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/user/profile", nil))
		var profile struct {
			Avatar string `json:"avatar"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &profile) != nil {
			t.Fatalf("profile = %d %s", w.Code, w.Body.String())
		}
		return profile.Avatar
	}
	// stored decodes the saved data URL and checks it is a 256px square.
	stored := func(wantMime string) []byte {
		t.Helper()
		header, payload, _ := strings.Cut(profileAvatar(), ",")
		if header != "data:"+wantMime+";base64" {
			t.Fatalf("avatar header = %q, want %s", header, wantMime)
		}
		raw, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			t.Fatalf("avatar payload: %v", err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || cfg.Width != avatarSide || cfg.Height != avatarSide {
			t.Fatalf("avatar = %dx%d (%v), want %dpx square", cfg.Width, cfg.Height, err, avatarSide)
		}
		return raw
	}

	// An opaque photo is cropped to a square and stored as JPEG.
	photo := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for i := range photo.Pix {
		photo.Pix[i] = 0xff
	}
	if code := upload(encodePNG(t, photo)); code != http.StatusOK {
		t.Fatalf("upload photo = %d, want 200", code)
	}
	stored("image/jpeg")

	// Transparency survives as PNG, and bytes appended to the file do not.
	payload := []byte("<script>alert(1)</script>")
	logo := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	logo.Set(10, 10, color.NRGBA{R: 200, A: 255})
	if code := upload(append(encodePNG(t, logo), payload...)); code != http.StatusOK {
		t.Fatalf("upload logo = %d, want 200", code)
	}
	if raw := stored("image/png"); bytes.Contains(raw, payload) {
		t.Fatal("stored avatar still carries the appended payload")
	}
	before := profileAvatar()

	logoPNG := encodePNG(t, logo)
	for name, bad := range map[string][]byte{
		"svg":        []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"script":     []byte("#!/bin/sh\nrm -rf /\n"),
		"truncated":  logoPNG[:40],
		"too wide":   encodePNG(t, image.NewGray(image.Rect(0, 0, maxAvatarSide+1, 1))),
		"too large":  append(bytes.Clone(logoPNG), make([]byte, maxAvatarUpload)...),
		"empty file": {},
	} {
		if code := upload(bad); code != http.StatusBadRequest {
			t.Errorf("%s: upload = %d, want 400", name, code)
		}
	}
	if profileAvatar() != before {
		t.Fatal("a rejected upload changed the avatar")
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/user/avatar", nil))
	if w.Code != http.StatusOK || profileAvatar() != "" {
		t.Fatalf("remove avatar = %d, avatar %.40q; want 200 and empty", w.Code, profileAvatar())
	}
}
