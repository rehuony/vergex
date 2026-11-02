package api

import (
	"testing"
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/security/crypto"
	"vergex/internal/trader/kind"
)

// Placeholder credential for tests; never a real key.
const testAPIKey = "test-api-key"

func findCheck(t *testing.T, checks []LaunchCheck, id string) LaunchCheck {
	t.Helper()
	for _, check := range checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("check %q not found in %+v", id, checks)
	return LaunchCheck{}
}

func TestCheckLaunchAgentModel(t *testing.T) {
	if got := checkLaunchAgentModel(nil); got.Status != launchCheckStatusFailed || got.Code != "MODEL_NOT_FOUND" {
		t.Fatalf("nil model: expected failed/MODEL_NOT_FOUND, got %+v", got)
	}

	disabled := &agent.Model{Name: "DeepSeek", Provider: "deepseek", Enabled: false}
	if got := checkLaunchAgentModel(disabled); got.Code != "MODEL_DISABLED" {
		t.Fatalf("disabled model: expected MODEL_DISABLED, got %+v", got)
	}

	noKey := &agent.Model{Name: "DeepSeek", Provider: "deepseek", Enabled: true}
	if got := checkLaunchAgentModel(noKey); got.Code != "MODEL_MISSING_CREDENTIALS" {
		t.Fatalf("missing key: expected MODEL_MISSING_CREDENTIALS, got %+v", got)
	}

	ready := &agent.Model{Name: "DeepSeek", Provider: "deepseek", Enabled: true, APIKey: crypto.EncryptedString(testAPIKey)}
	if got := checkLaunchAgentModel(ready); got.Status != launchCheckStatusOK {
		t.Fatalf("ready model: expected ok, got %+v", got)
	}
}

func TestDescribeAccountConfigIssue(t *testing.T) {
	if _, code := describeAccountConfigIssue(nil); code != "EXCHANGE_NOT_FOUND" {
		t.Fatalf("nil exchange: expected EXCHANGE_NOT_FOUND, got %s", code)
	}

	missing := &account.Account{ID: "ex", Venue: "okx", APIKey: crypto.EncryptedString(testAPIKey), SecretKey: crypto.EncryptedString(testAPIKey)}
	if _, code := describeAccountConfigIssue(missing); code != "EXCHANGE_MISSING_FIELDS" {
		t.Fatalf("missing fields: expected EXCHANGE_MISSING_FIELDS, got %s", code)
	}

	ready := readyBinanceAccount()
	if _, code := describeAccountConfigIssue(ready); code != "" {
		t.Fatalf("ready exchange: expected no issue, got %s", code)
	}
}

func readyBinanceAccount() *account.Account {
	return &account.Account{
		ID:        "ex-binance",
		Venue:     "binance",
		APIKey:    crypto.EncryptedString(testAPIKey),
		SecretKey: crypto.EncryptedString(testAPIKey),
	}
}

func preflightTestServer(t *testing.T, userID string, states map[string]AccountState) *Server {
	t.Helper()
	server := &Server{accountStateCache: NewAccountStateCache()}
	if states != nil {
		server.accountStateCache.Set(userID, states)
	}
	return server
}

func TestCheckLaunchAccountInsufficientFundsBlocks(t *testing.T) {
	account := readyBinanceAccount()
	server := preflightTestServer(t, "user-1", map[string]AccountState{
		account.ID: {AccountID: account.ID, Status: accountStatusOK, AvailableBalance: 5.5, TotalEquity: 5.5},
	})

	checks := server.checkLaunchAccount("user-1", account)
	funds := findCheck(t, checks, launchCheckAccountFunds)
	if funds.Status != launchCheckStatusFailed || funds.Code != "EXCHANGE_INSUFFICIENT_FUNDS" {
		t.Fatalf("expected failed/EXCHANGE_INSUFFICIENT_FUNDS, got %+v", funds)
	}
	if funds.Actual == nil || *funds.Actual != 5.5 {
		t.Fatalf("expected actual 5.5, got %+v", funds.Actual)
	}
}

func TestCheckLaunchAccountDeployedMarginPasses(t *testing.T) {
	// A running bot with capital locked in positions: low available balance
	// but healthy equity. Restart must not be blocked.
	account := readyBinanceAccount()
	server := preflightTestServer(t, "user-1", map[string]AccountState{
		account.ID: {AccountID: account.ID, Status: accountStatusOK, AvailableBalance: 3, TotalEquity: 100},
	})

	checks := server.checkLaunchAccount("user-1", account)
	funds := findCheck(t, checks, launchCheckAccountFunds)
	if funds.Status != launchCheckStatusOK {
		t.Fatalf("deployed margin with healthy equity should pass, got %+v", funds)
	}
	if funds.Actual == nil || *funds.Actual != 100 {
		t.Fatalf("expected actual 100 (equity), got %+v", funds.Actual)
	}
}

func TestCheckLaunchAccountInvalidCredentials(t *testing.T) {
	account := readyBinanceAccount()
	server := preflightTestServer(t, "user-1", map[string]AccountState{
		account.ID: {
			AccountID:    account.ID,
			Status:       accountStatusInvalidCredentials,
			ErrorCode:    "INVALID_CREDENTIALS",
			ErrorMessage: "Exchange credentials are invalid",
		},
	})

	checks := server.checkLaunchAccount("user-1", account)
	state := findCheck(t, checks, launchCheckAccountState)
	if state.Status != launchCheckStatusFailed || state.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("expected failed/INVALID_CREDENTIALS, got %+v", state)
	}
	if funds := findCheck(t, checks, launchCheckAccountFunds); funds.Status != launchCheckStatusSkipped {
		t.Fatalf("funds check should be skipped when account probe fails, got %+v", funds)
	}
}

func TestRunLaunchPreflightAggregatesReadiness(t *testing.T) {
	account := readyBinanceAccount()
	server := preflightTestServer(t, "user-1", map[string]AccountState{
		account.ID: {AccountID: account.ID, Status: accountStatusOK, AvailableBalance: 100, TotalEquity: 100},
	})
	model := &agent.Model{Name: "DeepSeek", Provider: "deepseek", Enabled: true, APIKey: crypto.EncryptedString(testAPIKey)}
	config := checkLaunchConfig(kind.Agent, []byte(`{"coin_source":{"source_type":"static"}}`))

	result := server.runLaunchPreflight("user-1", model, account, config)
	if !result.Ready {
		t.Fatalf("expected ready, got %+v", result)
	}
	if result.MinTradingUSDC != MinTradingUSDC {
		t.Fatalf("minimums must be exposed in the response, got %+v", result)
	}

	// Break one prerequisite → not ready, and Summary explains it.
	model.Enabled = false
	result = server.runLaunchPreflight("user-1", model, account, config)
	if result.Ready {
		t.Fatalf("expected not ready with a disabled model, got %+v", result)
	}
	if result.Summary() == "" {
		t.Fatalf("summary should describe the failing check")
	}
}

func TestRunLaunchPreflightWarningsDoNotBlock(t *testing.T) {
	account := readyBinanceAccount()
	// No account state for this exchange → probe result unknown → warning.
	server := preflightTestServer(t, "user-1", map[string]AccountState{})
	model := &agent.Model{Name: "DeepSeek", Provider: "deepseek", Enabled: true, APIKey: crypto.EncryptedString(testAPIKey)}

	result := server.runLaunchPreflight("user-1", model, account, LaunchCheck{ID: launchCheckConfig, Status: launchCheckStatusSkipped})
	if !result.Ready {
		t.Fatalf("warnings must not block launch, got %+v", result)
	}
}
