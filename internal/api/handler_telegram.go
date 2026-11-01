package api

import (
	"net/http"

	"vergex/internal/store"
	"vergex/internal/telegram"

	"github.com/gin-gonic/gin"
)

// handleGetTelegramConfig returns current Telegram bot configuration and binding status
func (s *Server) handleGetTelegramConfig(c *gin.Context) {
	cfg, err := s.store.TelegramConfig().Get()
	if err != nil {
		// Not configured yet - return empty state
		c.JSON(http.StatusOK, gin.H{
			"configured":   false,
			"is_bound":     false,
			"token_masked": "",
			"username":     "",
		})
		return
	}

	// Mask bot token for security (show only last 6 chars)
	tokenMasked := ""
	if cfg.BotToken != "" {
		if len(cfg.BotToken) > 6 {
			tokenMasked = "***" + string(cfg.BotToken[len(cfg.BotToken)-6:])
		} else {
			tokenMasked = "***"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"configured":     cfg.BotToken != "",
		"is_bound":       cfg.ChatID != 0,
		"username":       cfg.Username,
		"bound_at":       cfg.BoundAt,
		"token_masked":   tokenMasked,
		"agent_model_id": cfg.AgentModelID,
		"alerts":         alertSwitches(cfg),
	})
}

// handleUpdateTelegramConfig saves bot token (+ optional model ID) and triggers bot hot-reload
func (s *Server) handleUpdateTelegramConfig(c *gin.Context) {
	var req struct {
		BotToken     string `json:"bot_token"`
		AgentModelID string `json:"agent_model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.BotToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token is required"})
		return
	}

	if !s.validTelegramModel(c, req.AgentModelID) {
		return
	}
	if err := s.store.TelegramConfig().Save(req.BotToken, req.AgentModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config"})
		return
	}

	s.reloadTelegram()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Bot token saved. Bot will reload automatically."})
}

// handleUnbindTelegram removes Telegram user binding
func (s *Server) handleUnbindTelegram(c *gin.Context) {
	if err := s.store.TelegramConfig().Unbind(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unbind"})
		return
	}
	// The running bot keeps the chat it was started with until it reloads.
	s.reloadTelegram()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Telegram binding removed"})
}

// handleUpdateTelegramModel updates only the model used for Telegram replies (no token re-entry needed)
func (s *Server) handleUpdateTelegramModel(c *gin.Context) {
	var req struct {
		AgentModelID string `json:"agent_model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	cfg, err := s.store.TelegramConfig().Get()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Telegram config found, save a bot token first"})
		return
	}

	if !s.validTelegramModel(c, req.AgentModelID) {
		return
	}
	if err := s.store.TelegramConfig().Save(string(cfg.BotToken), req.AgentModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save model config"})
		return
	}

	// Chats keep the model client they were created with until the bot reloads.
	s.reloadTelegram()
	c.JSON(http.StatusOK, gin.H{"success": true, "agent_model_id": req.AgentModelID})
}

// validTelegramModel reports whether id is empty or one of the user's models, and
// answers the request when it is not.
func (s *Server) validTelegramModel(c *gin.Context, id string) bool {
	if id == "" {
		return true
	}
	if _, err := s.store.AgentModel().Get(c.GetString("user_id"), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown model"})
		return false
	}
	return true
}

// reloadTelegram asks the Telegram bot to restart with the saved settings. The
// channel holds one signal, so repeated saves collapse into one reload.
func (s *Server) reloadTelegram() {
	if s.telegramReloadCh == nil {
		return
	}
	select {
	case s.telegramReloadCh <- struct{}{}:
	default:
	}
}

// alertSwitches is each alert kind and whether it is on.
func alertSwitches(cfg *store.TelegramConfig) map[string]bool {
	switches := make(map[string]bool, len(telegram.AlertKinds))
	for _, kind := range telegram.AlertKinds {
		switches[kind] = !cfg.AlertMuted(kind)
	}
	return switches
}

// handleUpdateTelegramAlerts turns alert kinds on or off. Kinds left out keep
// their setting and unknown kinds are ignored.
func (s *Server) handleUpdateTelegramAlerts(c *gin.Context) {
	var req struct {
		Alerts map[string]bool `json:"alerts" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	cfg, err := s.store.TelegramConfig().Get()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram is not configured"})
		return
	}
	muted := []string{}
	for _, kind := range telegram.AlertKinds {
		on, sent := req.Alerts[kind]
		if (sent && !on) || (!sent && cfg.AlertMuted(kind)) {
			muted = append(muted, kind)
		}
	}
	if err := s.store.TelegramConfig().SetMutedAlerts(muted); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save alerts"})
		return
	}
	cfg.MutedAlerts = muted
	c.JSON(http.StatusOK, gin.H{"alerts": alertSwitches(cfg)})
}

// handleTestTelegramAlert sends one message so the user can see alerts arrive.
func (s *Server) handleTestTelegramAlert(c *gin.Context) {
	if err := telegram.Notify(c.Request.Context(), s.store, telegram.TestAlertText(s.store.TelegramConfig().GetLanguage())); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
