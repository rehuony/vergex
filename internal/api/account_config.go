package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"vergex/internal/account"
	"vergex/internal/config"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/security/crypto"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SafeAccountConfig Safe exchange configuration structure (does not contain sensitive information)
type SafeAccountConfig struct {
	ID            string `json:"id"`         // UUID
	Venue         string `json:"venue"`      // "binance" or "okx"
	Name          string `json:"name"`       // User-defined account name
	VenueName     string `json:"venue_name"` // Display name of the venue
	HasAPIKey     bool   `json:"has_api_key"`
	HasSecretKey  bool   `json:"has_secret_key"`
	HasPassphrase bool   `json:"has_passphrase"`
}

func safeAccountConfigFromStore(account *account.Account) SafeAccountConfig {
	return SafeAccountConfig{
		ID:            account.ID,
		Venue:         account.Venue,
		Name:          account.Name,
		VenueName:     account.VenueName(),
		HasAPIKey:     account.APIKey != "",
		HasSecretKey:  account.SecretKey != "",
		HasPassphrase: account.Passphrase != "",
	}
}

// AccountConfigUpdate is a single exchange account's update payload. It is a
// named type (rather than an inline anonymous struct) so the log-sanitizer in
// utils.go is guaranteed to cover every sensitive field — a drift between the
// two shapes is what let passphrases / private keys reach the logs previously.
type AccountConfigUpdate struct {
	APIKey     string `json:"api_key"`
	SecretKey  string `json:"secret_key"`
	Passphrase string `json:"passphrase"` // OKX specific
}

type UpdateAccountConfigRequest struct {
	Accounts map[string]AccountConfigUpdate `json:"accounts"`
}

// CreateAccountRequest request structure for creating a new exchange account
type CreateAccountRequest struct {
	Venue      string `json:"venue" binding:"required"` // "binance" or "okx"
	Name       string `json:"name"`                     // User-defined account name
	APIKey     string `json:"api_key"`
	SecretKey  string `json:"secret_key"`
	Passphrase string `json:"passphrase"`
}

// handleGetAccountConfigs Get exchange configurations
func (s *Server) handleGetAccountConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	logger.Infof("🔍 Querying exchange configs for user %s", userID)
	accounts, err := s.stores.Accounts.List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get exchange configs", err)
		return
	}

	// If no exchanges in database, return empty array (user needs to create accounts)
	if len(accounts) == 0 {
		logger.Infof("⚠️ No exchanges in database for user %s", userID)
		c.JSON(http.StatusOK, []SafeAccountConfig{})
		return
	}

	logger.Infof("✅ Found %d exchange configs", len(accounts))

	// Convert to safe response structure, remove sensitive information
	safeAccounts := make([]SafeAccountConfig, 0, len(accounts))
	for _, row := range accounts {
		if !account.IsVisible(row) {
			continue
		}
		safeAccounts = append(safeAccounts, safeAccountConfigFromStore(row))
	}

	c.JSON(http.StatusOK, safeAccounts)
}

// handleUpdateAccountConfigs Update exchange configurations (supports both encrypted and plain text based on config)
func (s *Server) handleUpdateAccountConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	cfg := config.Get()

	// Read raw request body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req UpdateAccountConfigRequest

	// Check if transport encryption is enabled
	if !cfg.TransportEncryption {
		// Transport encryption disabled, accept plain JSON
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			logger.Infof("❌ Failed to parse plain JSON request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}
		logger.Infof("📝 Received plain text exchange config (UserID: %s)", userID)
	} else {
		// Transport encryption enabled, require encrypted payload
		var encryptedPayload crypto.EncryptedPayload
		if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
			logger.Infof("❌ Failed to parse encrypted payload: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format, encrypted transmission required"})
			return
		}

		// Verify encrypted data
		if encryptedPayload.WrappedKey == "" {
			logger.Infof("❌ Detected unencrypted request (UserID: %s)", userID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "This endpoint only supports encrypted transmission, please use encrypted client",
				"code":    "ENCRYPTION_REQUIRED",
				"message": "Encrypted transmission is required for security reasons",
			})
			return
		}

		// Decrypt data
		decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
		if err != nil {
			logger.Infof("❌ Failed to decrypt exchange config (UserID: %s): %v", userID, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decrypt data"})
			return
		}

		// Parse decrypted data
		if err := json.Unmarshal([]byte(decrypted), &req); err != nil {
			logger.Infof("❌ Failed to parse decrypted data: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse decrypted data"})
			return
		}
		logger.Infof("🔓 Decrypted exchange config data (UserID: %s)", userID)
	}

	// Update each exchange's configuration and track traders that need reload
	tradersToReload := make(map[string]bool)
	for accountID, accountData := range req.Accounts {
		existing, err := s.stores.Accounts.GetByID(userID, accountID)
		if err != nil {
			SafeInternalError(c, fmt.Sprintf("Load exchange %s", accountID), err)
			return
		}
		effectiveAPIKey := strings.TrimSpace(accountData.APIKey)
		if effectiveAPIKey == "" {
			effectiveAPIKey = strings.TrimSpace(string(existing.APIKey))
		}
		effectiveSecretKey := strings.TrimSpace(accountData.SecretKey)
		if effectiveSecretKey == "" {
			effectiveSecretKey = strings.TrimSpace(string(existing.SecretKey))
		}
		effectivePassphrase := strings.TrimSpace(accountData.Passphrase)
		if effectivePassphrase == "" {
			effectivePassphrase = strings.TrimSpace(string(existing.Passphrase))
		}
		if missing := account.MissingCredentialFields(
			existing.Venue,
			effectiveAPIKey,
			effectiveSecretKey,
			effectivePassphrase,
		); len(missing) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Missing required exchange fields: %s", strings.Join(missing, ", ")),
				"missing_fields": missing,
			})
			return
		}

		// Find traders using this exchange BEFORE updating
		traders, _ := s.stores.Traders.ListByAccountID(userID, accountID)
		for _, t := range traders {
			tradersToReload[t.ID] = true
		}

		err = s.stores.Accounts.Update(userID, accountID, accountData.APIKey, accountData.SecretKey, accountData.Passphrase)
		if errors.Is(err, account.ErrDuplicate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": account.ErrDuplicate.Error()})
			return
		}
		if err != nil {
			SafeInternalError(c, fmt.Sprintf("Update exchange %s", accountID), err)
			return
		}
	}

	s.accountStateCache.Invalidate(userID)

	// Remove affected traders from memory BEFORE reloading to pick up new config
	for traderID := range tradersToReload {
		logger.Infof("🔄 Removing trader %s from memory to reload with new exchange config", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	// Reload all traders for this user to make new config take effect immediately
	err = s.traderManager.LoadUserTradersFromStore(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
		// Don't return error here since exchange config was successfully updated to database
	}

	logger.Infof("✓ Exchange config updated: %+v", SanitizeAccountConfigForLog(req.Accounts))
	c.JSON(http.StatusOK, gin.H{"message": "Exchange configuration updated"})
}

// handleCreateAccount Create a new exchange account
func (s *Server) handleCreateAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	cfg := config.Get()

	// Read raw request body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req CreateAccountRequest

	// Check if transport encryption is enabled
	if !cfg.TransportEncryption {
		// Transport encryption disabled, accept plain JSON
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			logger.Infof("❌ Failed to parse plain JSON request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}
	} else {
		// Transport encryption enabled, require encrypted payload
		var encryptedPayload crypto.EncryptedPayload
		if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format, encrypted transmission required"})
			return
		}

		if encryptedPayload.WrappedKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "This endpoint only supports encrypted transmission",
				"code":    "ENCRYPTION_REQUIRED",
				"message": "Encrypted transmission is required for security reasons",
			})
			return
		}

		decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decrypt data"})
			return
		}

		if err := json.Unmarshal([]byte(decrypted), &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse decrypted data"})
			return
		}
	}

	// Validate exchange type
	if !account.IsSupportedVenue(req.Venue) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid exchange type: %s", req.Venue)})
		return
	}
	if missing := account.MissingCredentialFields(
		req.Venue,
		req.APIKey,
		req.SecretKey,
		req.Passphrase,
	); len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":          fmt.Sprintf("Missing required exchange fields: %s", strings.Join(missing, ", ")),
			"missing_fields": missing,
		})
		return
	}

	// Exchange configs only persist once complete.
	id, err := s.stores.Accounts.Create(
		userID, req.Venue, req.Name,
		req.APIKey, req.SecretKey, req.Passphrase,
	)
	if errors.Is(err, account.ErrDuplicate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": account.ErrDuplicate.Error()})
		return
	}
	if err != nil {
		logger.Infof("❌ Failed to create exchange account: %v", err)
		SafeInternalError(c, "Failed to create exchange account", err)
		return
	}

	s.accountStateCache.Invalidate(userID)

	logger.Infof("✓ Created exchange account: type=%s, name=%s, id=%s", req.Venue, req.Name, id)
	c.JSON(http.StatusOK, gin.H{
		"message": "Account created",
		"id":      id,
	})
}

// handleDeleteAccount Delete an exchange account
func (s *Server) handleDeleteAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	accountID := c.Param("id")

	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Exchange ID is required"})
		return
	}

	// Check if any traders are using this exchange
	traders, err := s.stores.Traders.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check traders"})
		return
	}

	for _, trader := range traders {
		if trader.AccountID == accountID {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":       "Cannot delete exchange account that is in use by traders",
				"trader_id":   trader.ID,
				"trader_name": trader.Name,
			})
			return
		}
	}

	// Delete exchange account
	err = s.stores.Accounts.Delete(userID, accountID)
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete an account with trading history or an unfinished Futures trade"})
		return
	}
	if err != nil {
		logger.Infof("❌ Failed to delete exchange account: %v", err)
		SafeInternalError(c, "Failed to delete exchange account", err)
		return
	}

	s.accountStateCache.Invalidate(userID)

	logger.Infof("✓ Deleted exchange account: id=%s", accountID)
	c.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}

// handleGetAccount returns one of the caller's accounts with its state.
func (s *Server) handleGetAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	row, err := s.stores.Accounts.GetByID(userID, c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Account")
		return
	}
	out := gin.H{"account": safeAccountConfigFromStore(row)}
	if states, err := s.getAccountStates(userID); err == nil {
		if state, ok := states[row.ID]; ok {
			out["state"] = state
		}
	}
	c.JSON(http.StatusOK, out)
}

// ledgerQuery reads the limit and symbol a ledger read shares.
func ledgerQuery(c *gin.Context) (symbol string, limit int) {
	limit = 100
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	if symbol = c.Query("symbol"); symbol != "" {
		symbol = instrument.Normalize(symbol)
	}
	return symbol, limit
}

// handleAccountOrders lists the orders in an account's ledger, whoever placed them.
func (s *Server) handleAccountOrders(c *gin.Context) {
	row, err := s.stores.Accounts.GetByID(c.GetString("user_id"), c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Account")
		return
	}
	symbol, limit := ledgerQuery(c)
	orders, err := s.stores.Orders.GetAccountOrders(row.ID, symbol, c.Query("status"), limit)
	if err != nil {
		SafeInternalError(c, "Get orders", err)
		return
	}
	c.JSON(http.StatusOK, orders)
}

// handleAccountFills lists the fills in an account's ledger, newest first.
func (s *Server) handleAccountFills(c *gin.Context) {
	row, err := s.stores.Accounts.GetByID(c.GetString("user_id"), c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Account")
		return
	}
	symbol, limit := ledgerQuery(c)
	fills, err := s.stores.Orders.GetAccountFills(row.ID, symbol, limit)
	if err != nil {
		SafeInternalError(c, "Get fills", err)
		return
	}
	c.JSON(http.StatusOK, fills)
}
