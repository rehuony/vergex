package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"vergex/internal/auth"
	"vergex/internal/logger"
	"vergex/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
	userCount, err := s.store.User().Count()
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
		Lang     string `json:"lang"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	lang := req.Lang
	if lang != "zh" && lang != "id" {
		lang = "en"
	}

	// Check if email already exists
	_, err = s.store.User().GetByEmail(req.Email)
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
	user := &store.User{
		ID:           userID,
		Email:        req.Email,
		PasswordHash: passwordHash,
	}

	err = s.store.User().Create(user)
	if err != nil {
		SafeInternalError(c, "Failed to create user", err)
		return
	}

	// NOTE: Orphan record adoption was removed for security reasons. Previously,
	// after a reset-account call, any new user would inherit the prior owner's
	// wallet keys and exchange API credentials — a catastrophic IDOR/takeover
	// path. Operators who need to migrate credentials across users must do so
	// explicitly via export/import, never via implicit adoption on registration.

	// Generate JWT token
	token, err := auth.GenerateJWT(user.ID, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Initialize default model and exchange configs for user
	err = s.initUserDefaultConfigs(user.ID, lang)
	if err != nil {
		logger.Infof("Failed to initialize user default configs: %v", err)
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
	user, err := s.store.User().GetByEmail(req.Email)
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
	user, err := s.store.User().GetByID(userID)
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
	if err := s.store.User().UpdatePassword(userID, hash); err != nil {
		SafeInternalError(c, "Failed to update password", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password updated"})
}

// handleGetProfile returns the current user's email, display name and avatar.
func (s *Server) handleGetProfile(c *gin.Context) {
	user, err := s.store.User().GetByID(c.GetString("user_id"))
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
	if err := s.store.User().UpdateDisplayName(c.GetString("user_id"), name); err != nil {
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
	if err := s.store.User().UpdateAvatar(c.GetString("user_id"), avatar); err != nil {
		SafeInternalError(c, "Failed to update avatar", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar": avatar})
}

// handleRemoveAvatar clears the user's avatar.
func (s *Server) handleRemoveAvatar(c *gin.Context) {
	if err := s.store.User().UpdateAvatar(c.GetString("user_id"), ""); err != nil {
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
// (`vergex reset-password` / `vergex reset-account`, see cli.go), which requires
// shell access to the host — something a remote attacker does not have.

// initUserDefaultConfigs Initialize default configs for new user
func (s *Server) initUserDefaultConfigs(userID string, lang string) error {
	if err := s.createDefaultStrategies(userID, lang); err != nil {
		logger.Warnf("Failed to create default strategies for user %s: %v", userID, err)
		// Non-fatal: user can create strategies manually
	}
	logger.Infof("✓ User %s registration completed with default strategies", userID)
	return nil
}

func (s *Server) createDefaultStrategies(userID string, lang string) error {
	type strategyI18n struct {
		name, description string
	}
	type strategyLocale struct {
		defaultStrategy strategyI18n
	}
	locales := map[string]strategyLocale{
		"zh": {
			defaultStrategy: strategyI18n{"默认加密货币策略", "内置起步策略：固定候选池（BTC/ETH/SOL）的 USDT 永续，多周期 K 线 + 成交量/OI/资金费率。实盘前请按需调整币种与风控参数。"},
		},
		"en": {
			defaultStrategy: strategyI18n{"Default Crypto Strategy", "Built-in starter strategy: a static BTC/ETH/SOL candidate pool on USDT perpetuals with multi-timeframe candles, volume, OI and funding rate. Review coins and risk settings before trading live."},
		},
		"id": {
			defaultStrategy: strategyI18n{"Strategi Kripto Default", "Strategi awal bawaan: kumpulan kandidat statis BTC/ETH/SOL pada perpetual USDT dengan candle multi-timeframe, volume, OI, dan funding rate. Tinjau koin dan pengaturan risiko sebelum trading live."},
		},
	}
	locale, ok := locales[lang]
	if !ok {
		locale = locales["en"]
	}

	type strategyDef struct {
		name        string
		description string
		isActive    bool
	}

	definitions := []strategyDef{
		{
			name:        locale.defaultStrategy.name,
			description: locale.defaultStrategy.description,
			isActive:    true,
		},
	}

	// GetDefaultStrategyConfig only supports zh/en; map id -> en
	configLang := lang
	if lang == "id" {
		configLang = "en"
	}

	// Pre-build all strategy objects before opening the transaction
	var strategies []*store.Strategy
	for _, def := range definitions {
		config := store.GetDefaultStrategyConfig(configLang)
		config.ClampLimits()

		strategy := &store.Strategy{
			ID:          uuid.New().String(),
			UserID:      userID,
			Name:        def.name,
			Description: def.description,
			IsActive:    def.isActive,
			IsDefault:   false,
		}
		if err := strategy.SetConfig(&config); err != nil {
			return fmt.Errorf("failed to set config for strategy %q: %w", def.name, err)
		}
		strategies = append(strategies, strategy)
	}

	return s.store.Transaction(func(tx *gorm.DB) error {
		// Seed only an account without strategies. The preset is a starting
		// point, not something to keep in sync: matching by name would add a
		// second copy whenever the language differs or the user renamed it.
		var count int64
		if err := tx.Model(&store.Strategy{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to count strategies: %w", err)
		}
		if count > 0 {
			return nil
		}

		for _, strategy := range strategies {
			if err := tx.Create(strategy).Error; err != nil {
				return fmt.Errorf("failed to create strategy %q: %w", strategy.Name, err)
			}
			logger.Infof("  ✓ Created default strategy: %s (active=%v)", strategy.Name, strategy.IsActive)
		}
		return nil
	})
}
