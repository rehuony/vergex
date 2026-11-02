package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"vergex/internal/security/auth"
	"vergex/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// handleLogout Add current token to blacklist
func (s *Server) handleLogout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
		return
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization format"})
		return
	}
	tokenString := parts[1]
	claims, err := auth.ValidateJWT(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}
	var exp time.Time
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	} else {
		exp = time.Now().Add(24 * time.Hour)
	}
	auth.BlacklistToken(tokenString, exp)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
}

// handleRegister Handle user registration request.
// handleRegister allows registration only when no users exist yet (first-time setup).
// This is a single-user system; subsequent registrations are permanently closed.
func (s *Server) handleRegister(c *gin.Context) {
	userCount, err := s.stores.Users.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check user count"})
		return
	}

	if userCount > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "System already initialized"})
		return
	}

	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Check if email already exists
	_, err = s.stores.Users.GetByEmail(req.Email)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
		return
	}

	// Generate password hash
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password processing failed"})
		return
	}

	// Create user
	userID := uuid.New().String()
	user := &user.User{
		ID:           userID,
		Email:        req.Email,
		PasswordHash: passwordHash,
	}

	err = s.stores.Users.Create(user)
	if err != nil {
		SafeInternalError(c, "Failed to create user", err)
		return
	}

	// NOTE: Orphan record adoption was removed for security reasons. Previously,
	// after a reset-user call, any new user would inherit the prior owner's
	// wallet keys and exchange API credentials — a catastrophic IDOR/takeover
	// path. Operators who need to migrate credentials across users must do so
	// explicitly via export/import, never via implicit adoption on registration.

	// Generate JWT token
	token, err := auth.GenerateJWT(user.ID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"user_id": user.ID,
		"email":   user.Email,
		"message": "Registration successful",
	})
}

// dummyPasswordHash is a valid bcrypt hash of a throwaway value. It is compared
// against when the submitted email does not exist so that login takes roughly
// the same time whether or not the account exists — closing the timing side
// channel that would otherwise let an attacker enumerate valid emails (a fast
// "no such user" vs. a slow bcrypt compare). It is not a secret.
const dummyPasswordHash = "$2a$10$0iF0bCoQLJ6Ph1bF.MXwHOW.IMTxQjeEW.w38dctRQAB2kwB6ga1q"

// handleLogin Handle user login request
func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Get user information
	user, err := s.stores.Users.GetByEmail(req.Email)
	if err != nil {
		// Perform a dummy comparison so the response time does not reveal
		// whether the email exists (anti user-enumeration), then fail uniformly.
		auth.CheckPassword(req.Password, dummyPasswordHash)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email or password incorrect"})
		return
	}

	// Verify password
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email or password incorrect"})
		return
	}

	// Issue token directly after password verification.
	token, err := auth.GenerateJWT(user.ID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"user_id": user.ID,
		"email":   user.Email,
		"message": "Login successful",
	})
}

// handleChangePassword changes the signed-in user's password once the current
// one checks out, so a session left open is not enough to take the account.
// Attempts share the per-IP login budget.
func (s *Server) handleChangePassword(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "current_password and new_password (min 8 chars) are required")
		return
	}
	if !s.authLimiter.allow(c.ClientIP(), time.Now()) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts. Try again in a minute."})
		return
	}
	user, err := s.stores.Users.GetByID(userID)
	if err != nil {
		SafeNotFound(c, "User")
		return
	}
	if !auth.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		SafeBadRequestWithDetails(c, "Current password is incorrect", "current_password_incorrect", nil)
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		SafeInternalError(c, "Password processing failed", err)
		return
	}
	if err := s.stores.Users.UpdatePassword(userID, hash); err != nil {
		SafeInternalError(c, "Failed to update password", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password updated"})
}

// handleGetProfile returns the current user's email, display name and avatar.
func (s *Server) handleGetProfile(c *gin.Context) {
	user, err := s.stores.Users.GetByID(c.GetString("user_id"))
	if err != nil {
		SafeNotFound(c, "User")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"avatar":       user.Avatar,
	})
}

// maxDisplayName caps the display name in characters, not bytes.
const maxDisplayName = 32

// handleUpdateDisplayName sets the name shown in the account menu. Spaces at
// the ends are dropped; the name must keep at least one visible character
// and no control characters.
func (s *Server) handleUpdateDisplayName(c *gin.Context) {
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > maxDisplayName {
		SafeBadRequest(c, fmt.Sprintf("Display name must be 1 to %d characters", maxDisplayName))
		return
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			SafeBadRequest(c, "Display name cannot contain control characters")
			return
		}
	}
	if err := s.stores.Users.UpdateDisplayName(c.GetString("user_id"), name); err != nil {
		SafeInternalError(c, "Failed to update display name", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"display_name": name})
}

// handleUpdateAvatar takes an image upload (multipart field "avatar"), redraws
// it with normalizeAvatar and stores the result as the user's avatar.
func (s *Server) handleUpdateAvatar(c *gin.Context) {
	// Leave room for the multipart headers around the file.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarUpload+64<<10)
	header, err := c.FormFile("avatar")
	if err != nil {
		SafeBadRequest(c, "Upload an image of 2 MB or less in the avatar field")
		return
	}
	if header.Size > maxAvatarUpload {
		SafeBadRequest(c, "Avatar must be 2 MB or smaller")
		return
	}
	file, err := header.Open()
	if err != nil {
		SafeBadRequest(c, "Could not read the upload")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxAvatarUpload+1))
	if err != nil || len(raw) > maxAvatarUpload {
		SafeBadRequest(c, "Avatar must be 2 MB or smaller")
		return
	}
	avatar, err := normalizeAvatar(raw)
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	if err := s.stores.Users.UpdateAvatar(c.GetString("user_id"), avatar); err != nil {
		SafeInternalError(c, "Failed to update avatar", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar": avatar})
}

// handleRemoveAvatar clears the user's avatar.
func (s *Server) handleRemoveAvatar(c *gin.Context) {
	if err := s.stores.Users.UpdateAvatar(c.GetString("user_id"), ""); err != nil {
		SafeInternalError(c, "Failed to remove avatar", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar": ""})
}

// NOTE: Password and account recovery used to live here as the public,
// unauthenticated handlers handleResetPassword / handleResetAccount. They were
// removed because an unauthenticated recovery endpoint is a remotely
// exploitable auth-bypass on any public-facing deployment: the confirm phrase
// was embedded in the frontend (and echoed back by the API), so it was friction
// rather than authentication. Recovery now lives in the local CLI
// (`vergex reset-password` / `vergex reset-user`, see cli.go), which requires
// shell access to the host — something a remote attacker does not have.
