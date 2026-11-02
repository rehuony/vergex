package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"vergex/internal/account"
	"vergex/internal/logger"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"

	"github.com/gin-gonic/gin"
)

const accountStateCacheTTL = 30 * time.Second

const (
	accountStatusOK                 = "ok"
	accountStatusMissingCredentials = "missing_credentials"
	accountStatusInvalidCredentials = "invalid_credentials"
	accountStatusPermissionDenied   = "permission_denied"
	accountStatusUnavailable        = "unavailable"
)

type AccountState struct {
	AccountID        string  `json:"account_id"`
	Status           string  `json:"status"`
	DisplayBalance   string  `json:"display_balance,omitempty"`
	Asset            string  `json:"asset,omitempty"`
	TotalEquity      float64 `json:"total_equity,omitempty"`
	AvailableBalance float64 `json:"available_balance,omitempty"`
	// Permissions is what the key may do, absent where the venue did not say.
	Permissions  *account.Permissions `json:"permissions,omitempty"`
	CheckedAt    time.Time            `json:"checked_at"`
	ErrorCode    string               `json:"error_code,omitempty"`
	ErrorMessage string               `json:"error_message,omitempty"`
}

type cachedAccountStates struct {
	states   map[string]AccountState
	cachedAt time.Time
}

type AccountStateCache struct {
	entries map[string]cachedAccountStates
	mu      sync.RWMutex
}

func NewAccountStateCache() *AccountStateCache {
	return &AccountStateCache{
		entries: make(map[string]cachedAccountStates),
	}
}

func (c *AccountStateCache) Get(userID string) (map[string]AccountState, bool) {
	c.mu.RLock()
	entry, ok := c.entries[userID]
	c.mu.RUnlock()
	if !ok || time.Since(entry.cachedAt) >= accountStateCacheTTL {
		return nil, false
	}
	return cloneAccountStates(entry.states), true
}

func (c *AccountStateCache) Set(userID string, states map[string]AccountState) {
	c.mu.Lock()
	c.entries[userID] = cachedAccountStates{
		states:   cloneAccountStates(states),
		cachedAt: time.Now(),
	}
	c.mu.Unlock()
}

// Put replaces one account's state in a user's cached list, keeping the
// list's age; with no cached list there is nothing to update.
func (c *AccountStateCache) Put(userID string, state AccountState) {
	c.mu.Lock()
	if entry, ok := c.entries[userID]; ok {
		entry.states[state.AccountID] = state
	}
	c.mu.Unlock()
}

func (c *AccountStateCache) Invalidate(userID string) {
	c.mu.Lock()
	delete(c.entries, userID)
	c.mu.Unlock()
}

func cloneAccountStates(states map[string]AccountState) map[string]AccountState {
	cloned := make(map[string]AccountState, len(states))
	for id, state := range states {
		cloned[id] = state
	}
	return cloned
}

func (s *Server) handleGetAccountStates(c *gin.Context) {
	userID := c.GetString("user_id")

	states, err := s.getAccountStates(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get exchange account states", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"states": states})
}

// handleVerifyAccount checks one account now, past the cache, and
// stores the result for the next list read.
func (s *Server) handleVerifyAccount(c *gin.Context) {
	userID := c.GetString("user_id")
	accountCfg, err := s.stores.Accounts.GetByID(userID, c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Account")
		return
	}
	state := probeAccountState(accountCfg, userID)
	s.accountStateCache.Put(userID, state)
	c.JSON(http.StatusOK, state)
}

func (s *Server) getAccountStates(userID string) (map[string]AccountState, error) {
	if cached, ok := s.accountStateCache.Get(userID); ok {
		return cached, nil
	}

	accounts, err := s.stores.Accounts.List(userID)
	if err != nil {
		return nil, err
	}

	states := make(map[string]AccountState, len(accounts))
	if len(accounts) == 0 {
		return states, nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, accountCfg := range accounts {
		accountCfg := accountCfg
		wg.Add(1)
		go func() {
			defer wg.Done()
			state := probeAccountState(accountCfg, userID)
			mu.Lock()
			states[accountCfg.ID] = state
			mu.Unlock()
		}()
	}

	wg.Wait()
	s.accountStateCache.Set(userID, states)

	return cloneAccountStates(states), nil
}

func probeAccountState(accountCfg *account.Account, userID string) AccountState {
	state := AccountState{
		AccountID: accountCfg.ID,
		CheckedAt: time.Now().UTC(),
		Asset:     accountAssetForVenue(accountCfg.Venue),
	}

	if status, code, message, missing := missingAccountCredentials(accountCfg); missing {
		state.Status = status
		state.ErrorCode = code
		state.ErrorMessage = message
		return state
	}

	tempTrader, err := buildAccountProbe(accountCfg, userID)
	if err != nil {
		status, code, message := classifyAccountProbeError(err)
		state.Status = status
		state.ErrorCode = code
		state.ErrorMessage = message
		return state
	}

	// Read first: a key the balance read refuses is when what it may do matters most.
	state.Permissions = readKeyPermissions(tempTrader, accountCfg)

	balanceInfo, err := tempTrader.GetBalance()
	if err != nil {
		status, code, message := classifyAccountProbeError(err)
		state.Status = status
		state.ErrorCode = code
		state.ErrorMessage = message
		logger.Infof("⚠️ Failed to probe exchange account %s (%s): %v", accountCfg.ID, accountCfg.Venue, err)
		return state
	}

	totalEquity, totalFound := extractFirstNumeric(balanceInfo,
		"total_equity", "totalEquity", "totalWalletBalance", "wallet_balance", "totalEq", "balance")
	availableBalance, availableFound := extractFirstNumeric(balanceInfo,
		"available_balance", "availableBalance", "available")

	if !totalFound && availableFound {
		totalEquity = availableBalance
		totalFound = true
	}

	if !availableFound && totalFound {
		availableBalance = totalEquity
		availableFound = true
	}

	if !totalFound && !availableFound {
		state.Status = accountStatusUnavailable
		state.ErrorCode = "BALANCE_NOT_FOUND"
		state.ErrorMessage = "Connected but no balance fields were returned"
		return state
	}

	state.Status = accountStatusOK
	if totalFound {
		state.TotalEquity = totalEquity
		state.DisplayBalance = formatDisplayBalance(totalEquity, state.Asset)
	}
	if availableFound {
		state.AvailableBalance = availableBalance
		if state.DisplayBalance == "" {
			state.DisplayBalance = formatDisplayBalance(availableBalance, state.Asset)
		}
	}

	return state
}

// readKeyPermissions asks the venue what the account's key may do. It adds to what the
// state says and never decides it, so a failure only leaves the permissions out.
func readKeyPermissions(trading account.Trading, accountCfg *account.Account) *account.Permissions {
	reader, ok := trading.(account.PermissionReader)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	permissions, err := reader.Permissions(ctx)
	if err != nil {
		logger.Infof("⚠️ Failed to read the key permissions of account %s (%s): %v", accountCfg.ID, accountCfg.Venue, err)
		return nil
	}
	return permissions
}

func buildAccountProbe(accountCfg *account.Account, userID string) (account.Trading, error) {
	switch accountCfg.Venue {
	case "binance":
		return binance.NewReadOnlyAccount(context.Background(), string(accountCfg.APIKey), string(accountCfg.SecretKey), accountCfg.Testnet)
	case "okx":
		return okx.NewReadOnlyAccount(string(accountCfg.APIKey), string(accountCfg.SecretKey), string(accountCfg.Passphrase), accountCfg.Testnet), nil
	default:
		return nil, fmt.Errorf("unsupported exchange type: %s", accountCfg.Venue)
	}
}

func extractTotalEquity(balanceInfo map[string]interface{}) (float64, bool) {
	return extractFirstNumeric(balanceInfo,
		"total_equity", "totalEquity", "totalWalletBalance", "wallet_balance", "totalEq", "balance")
}

func extractFirstNumeric(values map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}

		switch v := raw.(type) {
		case float64:
			return v, true
		case float32:
			return float64(v), true
		case int:
			return float64(v), true
		case int64:
			return float64(v), true
		case int32:
			return float64(v), true
		case string:
			parsed, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return parsed, true
			}
		}
	}

	return 0, false
}

func formatDisplayBalance(value float64, asset string) string {
	formatted := strconv.FormatFloat(value, 'f', 4, 64)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if formatted == "" {
		formatted = "0"
	}
	if asset == "" {
		return formatted
	}
	return fmt.Sprintf("%s %s", formatted, asset)
}

func accountAssetForVenue(venue string) string {
	return "USDT"
}

func missingAccountCredentials(accountCfg *account.Account) (status string, code string, message string, missing bool) {
	missingFields := account.MissingCredentialFields(
		accountCfg.Venue,
		string(accountCfg.APIKey),
		string(accountCfg.SecretKey),
		string(accountCfg.Passphrase),
	)
	if len(missingFields) > 0 {
		if len(missingFields) == 1 && missingFields[0] == "venue" {
			return accountStatusUnavailable, "UNSUPPORTED_EXCHANGE", "Unsupported exchange type", true
		}
		return accountStatusMissingCredentials, "MISSING_REQUIRED_FIELDS", "Missing required fields: " + strings.Join(missingFields, ", "), true
	}

	return "", "", "", false
}

func classifyAccountProbeError(err error) (status string, code string, message string) {
	if err == nil {
		return accountStatusOK, "", ""
	}

	rawMessage := err.Error()
	msg := strings.ToLower(rawMessage)

	switch {
	case strings.Contains(msg, "unsupported exchange type"):
		return accountStatusUnavailable, "UNSUPPORTED_EXCHANGE", "Unsupported exchange type"
	case strings.Contains(msg, "requires ") || strings.Contains(msg, "missing") || strings.Contains(msg, "empty"):
		return accountStatusMissingCredentials, "MISSING_REQUIRED_FIELDS", "Exchange credentials are incomplete"
	case strings.Contains(msg, "permission") || strings.Contains(msg, "forbidden") || strings.Contains(msg, "no authority") || strings.Contains(msg, "not allowed"):
		return accountStatusPermissionDenied, "PERMISSION_DENIED", "Account has no permission to read balances"
	case strings.Contains(msg, "invalid") || strings.Contains(msg, "signature") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "api key") || strings.Contains(msg, "api-key") || strings.Contains(msg, "auth"):
		return accountStatusInvalidCredentials, "INVALID_CREDENTIALS", "Exchange credentials are invalid"
	default:
		return accountStatusUnavailable, "EXCHANGE_UNAVAILABLE", limitErrorMessage(rawMessage)
	}
}

func limitErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "Unable to fetch exchange balance right now"
	}
	if len(message) <= 160 {
		return message
	}
	return message[:157] + "..."
}
