package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"vergex/internal/account"
	"vergex/internal/logger"
	"vergex/internal/trader"
	"vergex/internal/trader/kind"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// trader management related structures
type CreateTraderRequest struct {
	Name         string `json:"name" binding:"required"`
	AgentModelID string `json:"agent_model_id" binding:"required"`
	AccountID    string `json:"account_id" binding:"required"`
	// Kind defaults to agent. It cannot change later.
	Kind string `json:"kind"`
	// Preset names the starting config; empty is the kind's default.
	Preset string `json:"preset"`
	// Config is a partial config applied on top of the preset.
	Config              map[string]any `json:"config"`
	InitialBalance      float64        `json:"initial_balance"`
	ScanIntervalMinutes int            `json:"scan_interval_minutes"`
	IsCrossMargin       *bool          `json:"is_cross_margin"` // Pointer type, nil means use default value true
}

// UpdateTraderRequest Update trader request. Only what is sent changes.
type UpdateTraderRequest struct {
	Name         string `json:"name"`
	AgentModelID string `json:"agent_model_id"`
	AccountID    string `json:"account_id"`
	// Config merges into the trader's config by section.
	Config              map[string]any `json:"config"`
	InitialBalance      float64        `json:"initial_balance"`
	ScanIntervalMinutes int            `json:"scan_interval_minutes"`
	IsCrossMargin       *bool          `json:"is_cross_margin"`
}

func formatTraderCreationError(reason, nextStep string) string {
	if nextStep == "" {
		return fmt.Sprintf("Failed to create the bot this time: %s", reason)
	}
	return fmt.Sprintf("Failed to create the bot this time: %s. %s", reason, nextStep)
}

func traderCreationRequestError(reason string) string {
	return formatTraderCreationError(reason, "Please check the information you just entered and submit again")
}

func accountDisplayName(account *account.Account) string {
	if account == nil {
		return "the selected exchange account"
	}
	if account.Name != "" {
		return fmt.Sprintf("%s (%s)", account.VenueName(), account.Name)
	}
	return account.VenueName()
}

func missingAccountFields(account *account.Account) []string {
	if account == nil {
		return nil
	}

	var missing []string
	switch account.Venue {
	case "binance":
		if account.APIKey == "" {
			missing = append(missing, "API Key")
		}
		if account.SecretKey == "" {
			missing = append(missing, "Secret Key")
		}
	case "okx":
		if account.APIKey == "" {
			missing = append(missing, "API Key")
		}
		if account.SecretKey == "" {
			missing = append(missing, "Secret Key")
		}
		if account.Passphrase == "" {
			missing = append(missing, "Passphrase")
		}
	}

	return missing
}

func mapStringPairs(kv ...string) map[string]string {
	if len(kv) == 0 {
		return nil
	}

	params := make(map[string]string, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		params[kv[i]] = kv[i+1]
	}
	return params
}

func validateAccountForTraderCreation(account *account.Account) (string, string, map[string]string) {
	if account == nil {
		return formatTraderCreationError("The exchange account you selected was not found", "Please go to \"Settings > Exchange Config\" and add an available account first, then come back to create the bot"),
			"trader.create.account_not_found", nil
	}

	missing := missingAccountFields(account)
	if len(missing) > 0 {
		return formatTraderCreationError(
				fmt.Sprintf("The configuration for exchange account \"%s\" is incomplete, missing %s", accountDisplayName(account), strings.Join(missing, ", ")),
				"Please go to \"Settings > Exchange Config\" to complete the required information for this account, then create the bot again",
			), "trader.create.account_missing_fields", mapStringPairs(
				"account_name", accountDisplayName(account),
				"missing_fields", strings.Join(missing, ", "),
			)
	}

	switch account.Venue {
	case "binance", "okx":
		return "", "", nil
	default:
		return formatTraderCreationError(
				fmt.Sprintf("Account \"%s\" uses type %s, which is not supported in the current version", accountDisplayName(account), account.Venue),
				"Please switch to an exchange account supported by the current version, then create the bot again",
			), "trader.create.account_unsupported", mapStringPairs(
				"account_name", accountDisplayName(account),
				"venue", account.Venue,
			)
	}
}

func classifyTraderSetupReason(reason string) (string, string) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "", ""
	}

	lower := strings.ToLower(trimmed)

	switch {
	case strings.Contains(lower, "failed to read the config"):
		return "trader.reason.config_invalid", "The bot's settings are corrupted and the system cannot read them for now"
	case strings.Contains(lower, "failed to parse private key"),
		(strings.Contains(lower, "invalid hex character") && strings.Contains(lower, "private key")):
		return "trader.reason.private_key_invalid", "The private key format is incorrect and the system cannot recognize it"
	case strings.Contains(lower, "failed to get meta information"):
		return "trader.reason.account_meta_unavailable", "The system cannot read account meta information from the exchange for now"
	case strings.Contains(lower, "failed to initialize account"):
		return "trader.reason.account_account_init_failed", "Account initialization failed; please confirm the wallet address and API Key match"
	case strings.Contains(lower, "unsupported trading platform"):
		return "trader.reason.account_unsupported", "The current exchange type does not support bot initialization"
	case strings.Contains(lower, "initial balance not set and unable to fetch balance from exchange"):
		return "trader.reason.account_balance_unavailable", "The system cannot read the account balance from the exchange for now"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "no such host"), strings.Contains(lower, "connection refused"):
		return "trader.reason.account_service_unreachable", "The system cannot connect to the exchange service for now"
	default:
		return "trader.reason.unknown", trimmed
	}
}

func humanizeTraderSetupReason(reason string) string {
	_, message := classifyTraderSetupReason(reason)
	return message
}

func traderSetupReasonParams(err error, fallback string, kv ...string) map[string]string {
	params := mapStringPairs(kv...)
	rawReason := SanitizeError(err, fallback)
	reasonKey, reasonMessage := classifyTraderSetupReason(rawReason)
	if reasonMessage == "" && fallback != "" {
		reasonMessage = fallback
	}
	if reasonMessage != "" {
		if params == nil {
			params = map[string]string{}
		}
		params["reason"] = reasonMessage
	}
	if reasonKey != "" {
		if params == nil {
			params = map[string]string{}
		}
		params["reason_key"] = reasonKey
	}
	return params
}

func describeTraderCreationWarning(traderName string, err error) string {
	if err == nil {
		return fmt.Sprintf("Bot \"%s\" has been saved, but it has not yet passed the pre-start validation. Please check the model, settings, and exchange configuration first, then click start after fixing them", traderName)
	}

	reason := humanizeTraderSetupReason(SanitizeError(err, ""))
	if reason == "" {
		return fmt.Sprintf("Bot \"%s\" has been saved, but it cannot start for now. Please check the model, settings, and exchange configuration first, then click start after fixing them", traderName)
	}

	return fmt.Sprintf("Bot \"%s\" has been saved, but it cannot start for now, because: %s. Please check the model, settings, and exchange configuration first, then click start after fixing them", traderName, reason)
}

func describeTraderStartError(traderName string, err error) string {
	if err == nil {
		return fmt.Sprintf("Failed to start the bot this time: bot \"%s\" cannot start for now. Please check the model, settings, and exchange configuration, then click start again", traderName)
	}

	reason := humanizeTraderSetupReason(SanitizeError(err, ""))
	if reason == "" {
		return fmt.Sprintf("Failed to start the bot this time: bot \"%s\" cannot start for now. Please check the model, settings, and exchange configuration, then click start again", traderName)
	}

	return fmt.Sprintf("Failed to start the bot this time: bot \"%s\" cannot start for now, because: %s. Please check the model, settings, and exchange configuration, then click start again", traderName, reason)
}

func formatTraderStartError(reason, nextStep string) string {
	if nextStep == "" {
		return fmt.Sprintf("Failed to start the bot this time: %s", reason)
	}
	return fmt.Sprintf("Failed to start the bot this time: %s. %s", reason, nextStep)
}

// handleCreateTrader Create new trader
func (s *Server) handleCreateTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	var req CreateTraderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequestWithDetails(c, traderCreationRequestError("The submitted information is incomplete or has an invalid format"), "trader.create.invalid_request", nil)
		return
	}

	model, err := s.stores.Models.Get(userID, req.AgentModelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			SafeBadRequestWithDetails(c, formatTraderCreationError("The model you selected was not found", "Please go to \"Settings > Model Config\" to add and enable an available model first, then come back to create the bot"), "trader.create.model_not_found", nil)
			return
		}
		SafeError(c, http.StatusInternalServerError,
			formatTraderCreationError("Unable to read your model configuration for now", "Please retry later; if the problem persists, check whether the local service is running normally"),
			err,
		)
		return
	}
	if !model.Enabled {
		SafeBadRequestWithDetails(c, formatTraderCreationError(
			fmt.Sprintf("Model \"%s\" is not enabled yet", model.Name),
			"Please go to \"Settings > Model Config\" to enable it, then create the bot again",
		), "trader.create.model_disabled", mapStringPairs("model_name", model.Name))
		return
	}
	if model.APIKey == "" {
		SafeBadRequestWithDetails(c, formatTraderCreationError(
			fmt.Sprintf("Model \"%s\" is missing an API Key", model.Name),
			"Please go to \"Settings > Model Config\" to complete the model credentials, then create the bot again",
		), "trader.create.model_missing_credentials", mapStringPairs("model_name", model.Name))
		return
	}

	if req.Kind == "" {
		req.Kind = kind.Agent
	}
	preset, err := kind.FromPreset(req.Kind, req.Preset)
	if err != nil {
		SafeBadRequestWithDetails(c, traderCreationRequestError(err.Error()), "trader.create.config_invalid", nil)
		return
	}
	config, warnings, err := kind.Merge(req.Kind, preset, req.Config)
	if err != nil {
		SafeBadRequestWithDetails(c, traderCreationRequestError(err.Error()), "trader.create.config_invalid", nil)
		return
	}
	if estimate, exceeds := exceedsEveryModel(config); exceeds {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count", estimate.Total),
			"token_estimate": estimate,
		})
		return
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}

	traderID := uuid.NewString()

	// Set default values
	isCrossMargin := true // Default to cross margin mode
	if req.IsCrossMargin != nil {
		isCrossMargin = *req.IsCrossMargin
	}

	// Set scan interval default value
	scanIntervalMinutes := req.ScanIntervalMinutes
	if scanIntervalMinutes <= 0 {
		scanIntervalMinutes = 15
	} else if scanIntervalMinutes < 3 {
		scanIntervalMinutes = 3 // Explicit values below 3 minutes are clamped to the minimum.
	}

	// Query exchange actual balance, override user input
	actualBalance := req.InitialBalance // Default to use user input
	accounts, err := s.stores.Accounts.List(userID)
	if err != nil {
		SafeError(c, http.StatusInternalServerError,
			formatTraderCreationError("Unable to read your exchange configuration for now", "Please retry later; if the problem persists, check whether the local service is running normally"),
			err,
		)
		return
	}

	// Find matching exchange configuration
	var accountCfg *account.Account
	for _, ex := range accounts {
		if ex.ID == req.AccountID {
			accountCfg = ex
			break
		}
	}

	if accountMsg, accountErrorKey, accountErrorParams := validateAccountForTraderCreation(accountCfg); accountMsg != "" {
		SafeBadRequestWithDetails(c, accountMsg, accountErrorKey, accountErrorParams)
		return
	}

	{
		tempTrader, createErr := buildAccountProbe(accountCfg, userID)
		if createErr != nil {
			SafeBadRequestWithDetails(c, formatTraderCreationError(
				fmt.Sprintf("Account \"%s\" did not pass initialization validation, because: %s", accountDisplayName(accountCfg), humanizeTraderSetupReason(SanitizeError(createErr, "Configuration validation failed"))),
				"Please go to \"Settings > Exchange Config\" to check whether this account's keys, address, and account information are entered correctly",
			), "trader.create.account_probe_failed", traderSetupReasonParams(createErr, "Configuration validation failed",
				"account_name", accountDisplayName(accountCfg),
			))
			return
		} else if tempTrader != nil {
			// Query actual balance
			balanceInfo, balanceErr := tempTrader.GetBalance()
			if balanceErr != nil {
				logger.Infof("⚠️ Failed to query exchange balance, using user input for initial balance: %v", balanceErr)
			} else {
				if extractedBalance, found := extractTotalEquity(balanceInfo); found {
					actualBalance = extractedBalance
					logger.Infof("✓ Queried exchange total equity: %.2f %s (user input: %.2f)",
						actualBalance, accountAssetForVenue(accountCfg.Venue), req.InitialBalance)
				} else {
					logger.Infof("⚠️ Unable to extract total equity from balance info, balanceInfo=%v, using user input for initial balance", balanceInfo)
				}
			}
		}
	}

	// Create trader configuration (database entity)
	logger.Infof("🔧 DEBUG: Starting to create trader config, ID=%s, Name=%s, AgentModel=%s, Exchange=%s, Kind=%s", traderID, req.Name, req.AgentModelID, req.AccountID, req.Kind)
	traderRecord := &trader.Trader{
		ID:                  traderID,
		UserID:              userID,
		Name:                req.Name,
		AgentModelID:        req.AgentModelID,
		AccountID:           req.AccountID,
		Kind:                req.Kind,
		Config:              configJSON,
		InitialBalance:      actualBalance, // Use actual queried balance
		MarginMode:          trader.MarginModeOf(isCrossMargin),
		ScanIntervalMinutes: scanIntervalMinutes,
		IsRunning:           false,
	}

	// Save to database
	logger.Infof("🔧 DEBUG: Preparing to call CreateTrader")
	err = s.stores.Traders.Create(traderRecord)
	if err != nil {
		logger.Infof("❌ Failed to create trader: %v", err)
		publicMsg := SanitizeError(err, formatTraderCreationError("The bot configuration was not saved successfully", "Please check the name, model, settings, and exchange configuration, then try again"))
		statusCode := http.StatusBadRequest
		if publicMsg == formatTraderCreationError("The bot configuration was not saved successfully", "Please check the name, model, settings, and exchange configuration, then try again") {
			statusCode = http.StatusInternalServerError
		}
		SafeError(c, statusCode, publicMsg, err)
		return
	}
	logger.Infof("🔧 DEBUG: CreateTrader succeeded")

	// Immediately load new trader into Manager
	logger.Infof("🔧 DEBUG: Preparing to call LoadUserTraders")
	startupWarning := ""
	err = s.traderManager.LoadUserTradersFromStore(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to load user traders into memory: %v", err)
		startupWarning = describeTraderCreationWarning(req.Name, err)
	}
	logger.Infof("🔧 DEBUG: LoadUserTraders completed")

	if startupWarning == "" {
		if loadErr := s.traderManager.GetLoadError(traderID); loadErr != nil {
			logger.Infof("⚠️ Trader %s failed to load after creation: %v", traderID, loadErr)
			startupWarning = describeTraderCreationWarning(req.Name, loadErr)
		}
	}

	if startupWarning == "" {
		if _, getErr := s.traderManager.GetTrader(traderID); getErr != nil {
			logger.Infof("⚠️ Trader %s not found in memory after creation: %v", traderID, getErr)
			startupWarning = describeTraderCreationWarning(req.Name, getErr)
		}
	}

	logger.Infof("✓ Trader created successfully: %s (model: %s, exchange: %s)", req.Name, req.AgentModelID, req.AccountID)

	response := gin.H{
		"trader_id":       traderID,
		"trader_name":     req.Name,
		"kind":            req.Kind,
		"agent_model":     req.AgentModelID,
		"is_running":      false,
		"startup_warning": startupWarning,
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	c.JSON(http.StatusCreated, response)
}

// handleUpdateTrader Update trader configuration
func (s *Server) handleUpdateTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	var req UpdateTraderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Check if trader exists and belongs to current user
	traders, err := s.stores.Traders.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get trader list"})
		return
	}

	var existingTrader *trader.Trader
	for _, t := range traders {
		if t.ID == traderID {
			existingTrader = t
			break
		}
	}

	if existingTrader == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist"})
		return
	}

	name := req.Name
	if name == "" {
		name = existingTrader.Name
	}
	agentModelID := req.AgentModelID
	if agentModelID == "" {
		agentModelID = existingTrader.AgentModelID
	}
	accountID := req.AccountID
	if accountID == "" {
		accountID = existingTrader.AccountID
	}

	// What it is pointed at must exist: the database would refuse it, less helpfully.
	if _, err := s.stores.Models.Get(userID, agentModelID); err != nil {
		SafeBadRequest(c, "The model you selected was not found")
		return
	}
	if _, err := s.stores.Accounts.GetByID(userID, accountID); err != nil {
		SafeBadRequest(c, "The exchange account you selected was not found")
		return
	}

	// Sections of the config that were not sent keep their values.
	var configJSON json.RawMessage
	var warnings []string
	if len(req.Config) > 0 {
		base, err := kind.Decode(existingTrader.Kind, existingTrader.Config)
		if err != nil {
			SafeBadRequest(c, err.Error())
			return
		}
		merged, adjusted, err := kind.Merge(existingTrader.Kind, base, req.Config)
		if err != nil {
			SafeBadRequest(c, err.Error())
			return
		}
		if estimate, exceeds := exceedsEveryModel(merged); exceeds {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count", estimate.Total),
				"token_estimate": estimate,
			})
			return
		}
		if configJSON, err = json.Marshal(merged); err != nil {
			SafeInternalError(c, "Serialize configuration", err)
			return
		}
		warnings = adjusted
	}

	// Set default values
	isCrossMargin := existingTrader.IsCrossMargin() // Keep original value
	if req.IsCrossMargin != nil {
		isCrossMargin = *req.IsCrossMargin
	}

	// Set scan interval, allow updates
	scanIntervalMinutes := req.ScanIntervalMinutes
	logger.Infof("📊 Update trader scan_interval: req=%d, existing=%d", req.ScanIntervalMinutes, existingTrader.ScanIntervalMinutes)
	if scanIntervalMinutes <= 0 {
		scanIntervalMinutes = existingTrader.ScanIntervalMinutes // Keep original value
	} else if scanIntervalMinutes < 3 {
		scanIntervalMinutes = 3
	}
	logger.Infof("📊 Final scan_interval_minutes: %d", scanIntervalMinutes)

	accountChanged := accountID != existingTrader.AccountID
	resetInitialBalance := accountChanged && req.InitialBalance <= 0

	initialBalance := existingTrader.InitialBalance
	if req.InitialBalance > 0 {
		initialBalance = req.InitialBalance
	}
	if resetInitialBalance {
		initialBalance = 0
	}

	// Update trader configuration
	traderRecord := &trader.Trader{
		ID:                  traderID,
		UserID:              userID,
		Name:                name,
		AgentModelID:        agentModelID,
		AccountID:           accountID,
		Config:              configJSON,
		InitialBalance:      initialBalance,
		MarginMode:          trader.MarginModeOf(isCrossMargin),
		ScanIntervalMinutes: scanIntervalMinutes,
		IsRunning:           existingTrader.IsRunning, // Keep original value
	}

	// Check if trader was running before update (we'll restart it after)
	wasRunning := false
	if existingMemTrader, memErr := s.traderManager.GetTrader(traderID); memErr == nil {
		status := existingMemTrader.GetStatus()
		if running, ok := status["is_running"].(bool); ok && running {
			wasRunning = true
			logger.Infof("🔄 Trader %s was running, will restart with new config after update", traderID)
		}
	}

	// Update database
	logger.Infof("🔄 Updating trader: ID=%s, Name=%s, AgentModelID=%s, ScanInterval=%d min",
		traderRecord.ID, traderRecord.Name, traderRecord.AgentModelID, scanIntervalMinutes)
	err = s.stores.Traders.Update(traderRecord)
	if err != nil {
		SafeInternalError(c, "Failed to update trader", err)
		return
	}

	if resetInitialBalance {
		logger.Infof("🔄 Exchange changed for trader %s, resetting stale initial_balance to 0", traderID)
		if err := s.stores.Traders.UpdateInitialBalance(userID, traderID, 0); err != nil {
			SafeInternalError(c, "Failed to reset trader initial balance", err)
			return
		}
	}

	// A running agent trader picks up a change to its config on its next cycle, with
	// nothing else about it disturbed.
	onlyConfig := len(configJSON) > 0 &&
		name == existingTrader.Name &&
		agentModelID == existingTrader.AgentModelID &&
		accountID == existingTrader.AccountID &&
		scanIntervalMinutes == existingTrader.ScanIntervalMinutes &&
		isCrossMargin == existingTrader.IsCrossMargin() &&
		initialBalance == existingTrader.InitialBalance
	if onlyConfig && existingTrader.Kind == kind.Agent {
		response := gin.H{
			"trader_id":   traderID,
			"trader_name": name,
			"agent_model": agentModelID,
			"is_running":  wasRunning,
			"message":     "Trader updated successfully",
		}
		if len(warnings) > 0 {
			response["warnings"] = warnings
		}
		logger.Infof("✓ Trader config updated: %s", name)
		c.JSON(http.StatusOK, response)
		return
	}

	// Remove old trader from memory first (this also stops if running)
	s.traderManager.RemoveTrader(traderID)

	// Reload traders into memory with fresh config
	err = s.traderManager.LoadUserTradersFromStore(userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
	}

	// If trader was running before, restart it with new config
	if wasRunning {
		if reloadedTrader, getErr := s.traderManager.GetTrader(traderID); getErr == nil {
			go func() {
				logger.Infof("▶️ Restarting trader %s with new config...", traderID)
				if runErr := reloadedTrader.Run(); runErr != nil {
					logger.Infof("❌ Trader %s runtime error: %v", traderID, runErr)
				}
			}()
		}
	}

	logger.Infof("✓ Trader updated successfully: %s (model: %s, exchange: %s)", name, agentModelID, accountID)

	response := gin.H{
		"trader_id":   traderID,
		"trader_name": name,
		"agent_model": agentModelID,
		// A running trader is restarted with the new config (async above), so
		// report it as running — callers must not fire a redundant start.
		"is_running": wasRunning,
		"message":    "Trader updated successfully",
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	c.JSON(http.StatusOK, response)
}

// handleDeleteTrader Delete trader
func (s *Server) handleDeleteTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	if _, err := s.stores.Traders.Get(userID, traderID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist"})
			return
		}
		SafeInternalError(c, "Failed to delete trader", err)
		return
	}

	// If trader is running, stop it first
	if trader, err := s.traderManager.GetTrader(traderID); err == nil {
		status := trader.GetStatus()
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			trader.Stop()
			logger.Infof("⏹  Stopped running trader: %s", traderID)
		}
	}

	// Remove trader from memory
	s.traderManager.RemoveTrader(traderID)

	// Its cycles, equity and ledger entries stay.
	if err := s.stores.Traders.Archive(userID, traderID); err != nil {
		SafeInternalError(c, "Failed to delete trader", err)
		return
	}

	logger.Infof("✓ Trader deleted: %s", traderID)
	c.JSON(http.StatusOK, gin.H{"message": "Trader deleted"})
}

// handleStartTrader Start trader
func (s *Server) handleStartTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	// Verify trader belongs to current user
	fullCfg, err := s.stores.Traders.GetFullConfig(userID, traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist or no access permission"})
		return
	}
	if fullCfg != nil && fullCfg.Account != nil {
		unlock, e := s.lockFuturesAccount(userID, fullCfg.Account.ID)
		if e != nil {
			SafeBadRequest(c, e.Error())
			return
		}
		defer unlock()
		leased, leaseError := s.stores.Accounts.Leased(fullCfg.Account.ID)
		if leaseError != nil {
			SafeBadRequest(c, "无法核对交易计划账户锁定状态")
			return
		}
		if leased {
			SafeBadRequest(c, "该账户有已批准或尚未结束的交易计划，先取消或完成计划再启动自动交易")
			return
		}
	}
	traderName := traderID
	if fullCfg != nil && fullCfg.Trader != nil && fullCfg.Trader.Name != "" {
		traderName = fullCfg.Trader.Name
	}

	// Check if trader exists in memory and if it's running
	existingTrader, _ := s.traderManager.GetTrader(traderID)
	if existingTrader != nil {
		status := existingTrader.GetStatus()
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":     "Trader is already running",
				"error_key": "trader.start.already_running",
			})
			return
		}
		// Trader exists but is stopped - remove from memory to reload fresh config
		logger.Infof("🔄 Removing stopped trader %s from memory to reload config...", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	// Load trader from database (always reload to get latest config)
	logger.Infof("🔄 Loading trader %s from database...", traderID)
	if loadErr := s.traderManager.LoadUserTradersFromStore(userID); loadErr != nil {
		logger.Infof("❌ Failed to load user traders: %v", loadErr)
		SafeErrorWithDetails(c, http.StatusInternalServerError, describeTraderStartError(traderName, loadErr), "trader.start.load_failed", traderSetupReasonParams(loadErr, "", "trader_name", traderName), loadErr)
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		if fullCfg != nil && fullCfg.Trader != nil {
			// Check model
			if fullCfg.AgentModel == nil {
				SafeBadRequestWithDetails(c, formatTraderStartError("The model associated with this bot does not exist", "Please go to \"Settings > Model Config\" to check, then click start again"), "trader.start.model_not_found", mapStringPairs("trader_name", traderName))
				return
			}
			if !fullCfg.AgentModel.Enabled {
				SafeBadRequestWithDetails(c, formatTraderStartError(
					fmt.Sprintf("The model \"%s\" associated with bot \"%s\" is not enabled yet", fullCfg.AgentModel.Name, traderName),
					"Please go to \"Settings > Model Config\" to enable it, then click start again",
				), "trader.start.model_disabled", mapStringPairs("trader_name", traderName, "model_name", fullCfg.AgentModel.Name))
				return
			}
			// Check exchange
			if fullCfg.Account == nil {
				SafeBadRequestWithDetails(c, formatTraderStartError("The exchange account associated with this bot does not exist", "Please go to \"Settings > Exchange Config\" to check, then click start again"), "trader.start.account_not_found", mapStringPairs("trader_name", traderName))
				return
			}
		}
		// Check if there's a specific load error
		if loadErr := s.traderManager.GetLoadError(traderID); loadErr != nil {
			SafeBadRequestWithDetails(c, describeTraderStartError(traderName, loadErr), "trader.start.load_failed", traderSetupReasonParams(loadErr, "", "trader_name", traderName))
			return
		}
		SafeBadRequestWithDetails(c, describeTraderStartError(traderName, err), "trader.start.setup_invalid", traderSetupReasonParams(err, "", "trader_name", traderName))
		return
	}

	// Server-side launch gate: the trader cannot function without a funded agent
	// wallet and a ready exchange account, so verify both before the run loop
	// starts. `?force=true` skips the gate for deliberate manual overrides.
	if c.Query("force") != "true" {
		// A trader that loaded into memory necessarily has a valid config (the manager
		// refuses to load without one), so the preflight config check is skipped here.
		preflight := s.runLaunchPreflight(userID, fullCfg.AgentModel, fullCfg.Account, LaunchCheck{ID: launchCheckConfig, Status: launchCheckStatusSkipped})
		if !preflight.Ready {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":     formatTraderStartError(preflight.Summary(), "Complete the failing checks, then start the bot again"),
				"error_key": "trader.start.preflight_failed",
				"preflight": preflight,
			})
			return
		}
	}

	// Start trader
	go func() {
		logger.Infof("▶️  Starting trader %s (%s)", traderID, trader.GetName())
		if err := trader.Run(); err != nil {
			logger.Infof("❌ Trader %s runtime error: %v", trader.GetName(), err)
		}
	}()

	// Update running status in database
	err = s.stores.Traders.UpdateStatus(userID, traderID, true)
	if err != nil {
		logger.Infof("⚠️  Failed to update trader status: %v", err)
	}

	logger.Infof("✓ Trader %s started", trader.GetName())
	c.JSON(http.StatusOK, gin.H{"message": "Trader started"})
}

// handleStopTrader Stop trader
func (s *Server) handleResumeTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")
	if _, err := s.stores.Traders.GetFullConfig(userID, traderID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist or no access permission"})
		return
	}
	var err error
	if at, loadErr := s.traderManager.GetTrader(traderID); loadErr == nil {
		err = at.Resume()
	} else {
		now := time.Now().UTC()
		err = s.stores.Traders.ClearHalt(traderID, &now)
	}
	if err != nil {
		SafeInternalError(c, "Resume trader", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Trader resumed"})
}

func (s *Server) handleStopTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	// Verify trader belongs to current user
	_, err := s.stores.Traders.GetFullConfig(userID, traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist or no access permission"})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist"})
		return
	}

	// Check if trader is running
	status := trader.GetStatus()
	if isRunning, ok := status["is_running"].(bool); ok && !isRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Trader is already stopped"})
		return
	}

	// Stop trader
	trader.Stop()

	// Update running status in database
	err = s.stores.Traders.UpdateStatus(userID, traderID, false)
	if err != nil {
		logger.Infof("⚠️  Failed to update trader status: %v", err)
	}

	logger.Infof("⏹  Trader %s stopped", trader.GetName())
	if stopErr := trader.StopError(); stopErr != nil {
		c.JSON(http.StatusOK, gin.H{"message": "Trader stopped", "warning": "orders_left"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Trader stopped"})
}
