package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/trader/kind"

	"github.com/gin-gonic/gin"
)

// Launch readiness minimums. These are the single source of truth — the
// frontend reads them from the preflight response instead of hardcoding.
const (
	// MinTradingUSDC is the minimum available balance the exchange account
	// needs before the trader can place its first order.
	MinTradingUSDC = 12.0
)

const (
	launchCheckStatusOK      = "ok"
	launchCheckStatusFailed  = "failed"
	launchCheckStatusWarning = "warning"
	launchCheckStatusSkipped = "skipped"
)

// Check IDs — stable identifiers the frontend maps to guided-setup steps.
const (
	launchCheckAgentModel    = "agent_model"
	launchCheckConfig        = "config"
	launchCheckAccountConfig = "account_config"
	launchCheckAccountState  = "account_state"
	launchCheckAccountFunds  = "account_funds"
)

type LaunchCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	// Numeric context so the UI can render progress like "8.40 / 12 USDC".
	Required float64  `json:"required,omitempty"`
	Actual   *float64 `json:"actual,omitempty"`
	Asset    string   `json:"asset,omitempty"`
	// Address is the funding address for balance checks, so the UI can offer
	// a deposit shortcut next to the failing item.
	Address string `json:"address,omitempty"`
}

type LaunchPreflightResult struct {
	Ready          bool          `json:"ready"`
	Checks         []LaunchCheck `json:"checks"`
	MinTradingUSDC float64       `json:"min_trading_usdc"`
	CheckedAt      time.Time     `json:"checked_at"`
}

func (r LaunchPreflightResult) failedChecks() []LaunchCheck {
	var failed []LaunchCheck
	for _, check := range r.Checks {
		if check.Status == launchCheckStatusFailed {
			failed = append(failed, check)
		}
	}
	return failed
}

// Summary joins the failing messages into one human-readable sentence.
func (r LaunchPreflightResult) Summary() string {
	failed := r.failedChecks()
	if len(failed) == 0 {
		return ""
	}
	messages := make([]string, 0, len(failed))
	for _, check := range failed {
		if check.Message != "" {
			messages = append(messages, check.Message)
		}
	}
	return strings.Join(messages, " ")
}

type launchPreflightRequest struct {
	AgentModelID string `json:"agent_model_id"`
	AccountID    string `json:"account_id"`
	// Kind and Config are checked when sent.
	Kind   string          `json:"kind"`
	Config json.RawMessage `json:"config"`
}

// handleLaunchPreflight runs launch readiness checks for a model/exchange
// pair before any trader is created or mutated. POST /api/launch/preflight
func (s *Server) handleLaunchPreflight(c *gin.Context) {
	userID := c.GetString("user_id")

	var req launchPreflightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid preflight request")
		return
	}
	if strings.TrimSpace(req.AgentModelID) == "" || strings.TrimSpace(req.AccountID) == "" {
		SafeBadRequest(c, "agent_model_id and account_id are required")
		return
	}

	model, err := s.stores.Models.Get(userID, req.AgentModelID)
	if err != nil {
		model = nil
	}
	account, err := s.stores.Accounts.GetByID(userID, req.AccountID)
	if err != nil {
		account = nil
	}

	config := LaunchCheck{ID: launchCheckConfig, Status: launchCheckStatusSkipped}
	if len(req.Config) > 0 {
		config = checkLaunchConfig(req.Kind, req.Config)
	}

	result := s.runLaunchPreflight(userID, model, account, config)
	c.JSON(http.StatusOK, result)
}

// handleTraderPreflight runs the same checks against an existing trader's
// full configuration. GET /api/traders/:id/preflight
func (s *Server) handleTraderPreflight(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	fullCfg, err := s.stores.Traders.GetFullConfig(userID, traderID)
	if err != nil || fullCfg == nil || fullCfg.Trader == nil {
		SafeNotFound(c, "Trader")
		return
	}

	result := s.runLaunchPreflight(userID, fullCfg.AgentModel, fullCfg.Account, checkLaunchConfig(fullCfg.Trader.Kind, fullCfg.Trader.Config))
	c.JSON(http.StatusOK, result)
}

// runLaunchPreflight composes every launch readiness check. A check that
// cannot be evaluated (RPC outage, probe timeout) reports "warning" instead
// of "failed" so an infrastructure hiccup never hard-blocks a launch.
func (s *Server) runLaunchPreflight(
	userID string,
	model *agent.Model,
	account *account.Account,
	config LaunchCheck,
) LaunchPreflightResult {
	checks := []LaunchCheck{checkLaunchAgentModel(model), config}
	checks = append(checks, s.checkLaunchAccount(userID, account)...)

	ready := true
	for _, check := range checks {
		if check.Status == launchCheckStatusFailed {
			ready = false
			break
		}
	}

	return LaunchPreflightResult{
		Ready:          ready,
		Checks:         checks,
		MinTradingUSDC: MinTradingUSDC,
		CheckedAt:      time.Now().UTC(),
	}
}

func checkLaunchAgentModel(model *agent.Model) LaunchCheck {
	check := LaunchCheck{ID: launchCheckAgentModel}

	switch {
	case model == nil:
		check.Status = launchCheckStatusFailed
		check.Code = "MODEL_NOT_FOUND"
		check.Message = "The selected model was not found. Configure a model first."
	case !model.Enabled:
		check.Status = launchCheckStatusFailed
		check.Code = "MODEL_DISABLED"
		check.Message = fmt.Sprintf("Model \"%s\" is disabled. Enable it first.", model.Name)
	case strings.TrimSpace(model.APIKey.String()) == "":
		check.Status = launchCheckStatusFailed
		check.Code = "MODEL_MISSING_CREDENTIALS"
		check.Message = fmt.Sprintf("Model \"%s\" has no credential saved. Add the API key first.", model.Name)
	default:
		check.Status = launchCheckStatusOK
		check.Message = model.Name
	}

	return check
}

// checkLaunchConfig checks that a trader's config can run.
func checkLaunchConfig(name string, raw []byte) LaunchCheck {
	check := LaunchCheck{ID: launchCheckConfig}
	if _, err := kind.Decode(name, raw); err != nil {
		check.Status = launchCheckStatusFailed
		check.Code = "CONFIG_INVALID"
		check.Message = err.Error()
		return check
	}
	check.Status = launchCheckStatusOK
	check.Message = name
	return check
}

// checkLaunchAccount validates exchange configuration completeness and then
// probes the live account (30s server cache) for status and balance.
func (s *Server) checkLaunchAccount(userID string, account *account.Account) []LaunchCheck {
	configCheck := LaunchCheck{ID: launchCheckAccountConfig}
	accountCheck := LaunchCheck{ID: launchCheckAccountState}
	fundsCheck := LaunchCheck{ID: launchCheckAccountFunds, Required: MinTradingUSDC}

	if msg, code := describeAccountConfigIssue(account); code != "" {
		configCheck.Status = launchCheckStatusFailed
		configCheck.Code = code
		configCheck.Message = msg
		accountCheck.Status = launchCheckStatusSkipped
		fundsCheck.Status = launchCheckStatusSkipped
		return []LaunchCheck{configCheck, accountCheck, fundsCheck}
	}

	configCheck.Status = launchCheckStatusOK
	configCheck.Message = accountDisplayName(account)
	fundsCheck.Asset = accountAssetForVenue(account.Venue)

	states, err := s.getAccountStates(userID)
	if err != nil {
		accountCheck.Status = launchCheckStatusWarning
		accountCheck.Code = "EXCHANGE_STATE_UNKNOWN"
		accountCheck.Message = "Could not verify the exchange account right now."
		fundsCheck.Status = launchCheckStatusSkipped
		return []LaunchCheck{configCheck, accountCheck, fundsCheck}
	}

	state, ok := states[account.ID]
	if !ok {
		accountCheck.Status = launchCheckStatusWarning
		accountCheck.Code = "EXCHANGE_STATE_UNKNOWN"
		accountCheck.Message = "Could not verify the exchange account right now."
		fundsCheck.Status = launchCheckStatusSkipped
		return []LaunchCheck{configCheck, accountCheck, fundsCheck}
	}

	if state.Status != accountStatusOK {
		accountCheck.Status = launchCheckStatusFailed
		accountCheck.Code = state.ErrorCode
		accountCheck.Message = state.ErrorMessage
		if accountCheck.Message == "" {
			accountCheck.Message = fmt.Sprintf("Account \"%s\" is not ready (%s).", accountDisplayName(account), state.Status)
		}
		fundsCheck.Status = launchCheckStatusSkipped
		return []LaunchCheck{configCheck, accountCheck, fundsCheck}
	}

	accountCheck.Status = launchCheckStatusOK
	accountCheck.Message = state.DisplayBalance

	// Gate on the better of available balance and total equity: a bot with
	// capital deployed in open positions has low *available* margin but is
	// clearly funded — restarting it must not be blocked.
	funded := state.AvailableBalance
	if state.TotalEquity > funded {
		funded = state.TotalEquity
	}
	fundsCheck.Actual = &funded

	if funded < MinTradingUSDC {
		message := fmt.Sprintf(
			"Account \"%s\" holds %.2f %s but needs at least %.0f %s to place the first trade.",
			accountDisplayName(account), funded, fundsCheck.Asset, MinTradingUSDC, fundsCheck.Asset,
		)
		fundsCheck.Status = launchCheckStatusFailed
		fundsCheck.Code = "EXCHANGE_INSUFFICIENT_FUNDS"
		fundsCheck.Message = message
	} else {
		fundsCheck.Status = launchCheckStatusOK
	}

	return []LaunchCheck{configCheck, accountCheck, fundsCheck}
}

// describeAccountConfigIssue mirrors the create-time exchange validation but
// returns stable uppercase codes for the checklist UI.
func describeAccountConfigIssue(account *account.Account) (string, string) {
	if account == nil {
		return "The selected exchange account was not found. Connect an exchange first.", "EXCHANGE_NOT_FOUND"
	}
	if missing := missingAccountFields(account); len(missing) > 0 {
		return fmt.Sprintf(
			"Account \"%s\" is missing %s. Complete the connection first.",
			accountDisplayName(account), strings.Join(missing, ", "),
		), "EXCHANGE_MISSING_FIELDS"
	}
	return "", ""
}
