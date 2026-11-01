package api

import (
	"testing"

	"vergex/internal/security/crypto"
	"vergex/internal/store"
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

	disabled := &store.AgentModel{Name: "DeepSeek", Provider: "deepseek", Enabled: false}
	if got := checkLaunchAgentModel(disabled); got.Code != "MODEL_DISABLED" {
		t.Fatalf("disabled model: expected MODEL_DISABLED, got %+v", got)
	}

	noKey := &store.AgentModel{Name: "DeepSeek", Provider: "deepseek", Enabled: true}
	if got := checkLaunchAgentModel(noKey); got.Code != "MODEL_MISSING_CREDENTIALS" {
		t.Fatalf("missing key: expected MODEL_MISSING_CREDENTIALS, got %+v", got)
	}

	ready := &store.AgentModel{Name: "DeepSeek", Provider: "deepseek", Enabled: true, APIKey: crypto.EncryptedString(testAPIKey)}
	if got := checkLaunchAgentModel(ready); got.Status != launchCheckStatusOK {
		t.Fatalf("ready model: expected ok, got %+v", got)
	}
}

func TestDescribeExchangeConfigIssue(t *testing.T) {
	if _, code := describeExchangeConfigIssue(nil); code != "EXCHANGE_NOT_FOUND" {
		t.Fatalf("nil exchange: expected EXCHANGE_NOT_FOUND, got %s", code)
	}

	missing := &store.Exchange{ID: "ex", ExchangeType: "okx", APIKey: crypto.EncryptedString(testAPIKey), SecretKey: crypto.EncryptedString(testAPIKey)}
	if _, code := describeExchangeConfigIssue(missing); code != "EXCHANGE_MISSING_FIELDS" {
		t.Fatalf("missing fields: expected EXCHANGE_MISSING_FIELDS, got %s", code)
	}

	ready := readyBinanceExchange()
	if _, code := describeExchangeConfigIssue(ready); code != "" {
		t.Fatalf("ready exchange: expected no issue, got %s", code)
	}
}

func readyBinanceExchange() *store.Exchange {
	return &store.Exchange{
		ID:           "ex-binance",
		ExchangeType: "binance",
		APIKey:       crypto.EncryptedString(testAPIKey),
		SecretKey:    crypto.EncryptedString(testAPIKey),
	}
}

func preflightTestServer(t *testing.T, userID string, states map[string]ExchangeAccountState) *Server {
	t.Helper()
	server := &Server{exchangeAccountStateCache: NewExchangeAccountStateCache()}
	if states != nil {
		server.exchangeAccountStateCache.Set(userID, states)
	}
	return server
}

func TestCheckLaunchExchangeInsufficientFundsBlocks(t *testing.T) {
	exchange := readyBinanceExchange()
	server := preflightTestServer(t, "user-1", map[string]ExchangeAccountState{
		exchange.ID: {ExchangeID: exchange.ID, Status: exchangeAccountStatusOK, AvailableBalance: 5.5, TotalEquity: 5.5},
	})

	checks := server.checkLaunchExchange("user-1", exchange)
	funds := findCheck(t, checks, launchCheckExchangeFunds)
	if funds.Status != launchCheckStatusFailed || funds.Code != "EXCHANGE_INSUFFICIENT_FUNDS" {
		t.Fatalf("expected failed/EXCHANGE_INSUFFICIENT_FUNDS, got %+v", funds)
	}
	if funds.Actual == nil || *funds.Actual != 5.5 {
		t.Fatalf("expected actual 5.5, got %+v", funds.Actual)
	}
}

func TestCheckLaunchExchangeDeployedMarginPasses(t *testing.T) {
	// A running bot with capital locked in positions: low available balance
	// but healthy equity. Restart must not be blocked.
	exchange := readyBinanceExchange()
	server := preflightTestServer(t, "user-1", map[string]ExchangeAccountState{
		exchange.ID: {ExchangeID: exchange.ID, Status: exchangeAccountStatusOK, AvailableBalance: 3, TotalEquity: 100},
	})

	checks := server.checkLaunchExchange("user-1", exchange)
	funds := findCheck(t, checks, launchCheckExchangeFunds)
	if funds.Status != launchCheckStatusOK {
		t.Fatalf("deployed margin with healthy equity should pass, got %+v", funds)
	}
	if funds.Actual == nil || *funds.Actual != 100 {
		t.Fatalf("expected actual 100 (equity), got %+v", funds.Actual)
	}
}

func TestCheckLaunchExchangeInvalidCredentials(t *testing.T) {
	exchange := readyBinanceExchange()
	server := preflightTestServer(t, "user-1", map[string]ExchangeAccountState{
		exchange.ID: {
			ExchangeID:   exchange.ID,
			Status:       exchangeAccountStatusInvalidCredentials,
			ErrorCode:    "INVALID_CREDENTIALS",
			ErrorMessage: "Exchange credentials are invalid",
		},
	})

	checks := server.checkLaunchExchange("user-1", exchange)
	account := findCheck(t, checks, launchCheckExchangeAccount)
	if account.Status != launchCheckStatusFailed || account.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("expected failed/INVALID_CREDENTIALS, got %+v", account)
	}
	if funds := findCheck(t, checks, launchCheckExchangeFunds); funds.Status != launchCheckStatusSkipped {
		t.Fatalf("funds check should be skipped when account probe fails, got %+v", funds)
	}
}

func TestRunLaunchPreflightAggregatesReadiness(t *testing.T) {
	exchange := readyBinanceExchange()
	server := preflightTestServer(t, "user-1", map[string]ExchangeAccountState{
		exchange.ID: {ExchangeID: exchange.ID, Status: exchangeAccountStatusOK, AvailableBalance: 100, TotalEquity: 100},
	})
	model := &store.AgentModel{Name: "DeepSeek", Provider: "deepseek", Enabled: true, APIKey: crypto.EncryptedString(testAPIKey)}
	strategy := &store.Strategy{ID: "strat-1", Name: "Autopilot"}

	result := server.runLaunchPreflight("user-1", model, exchange, strategy, true)
	if !result.Ready {
		t.Fatalf("expected ready, got %+v", result)
	}
	if result.MinTradingUSDC != MinTradingUSDC {
		t.Fatalf("minimums must be exposed in the response, got %+v", result)
	}

	// Break one prerequisite → not ready, and Summary explains it.
	model.Enabled = false
	result = server.runLaunchPreflight("user-1", model, exchange, strategy, true)
	if result.Ready {
		t.Fatalf("expected not ready with a disabled model, got %+v", result)
	}
	if result.Summary() == "" {
		t.Fatalf("summary should describe the failing check")
	}
}

func TestRunLaunchPreflightWarningsDoNotBlock(t *testing.T) {
	exchange := readyBinanceExchange()
	// No account state for this exchange → probe result unknown → warning.
	server := preflightTestServer(t, "user-1", map[string]ExchangeAccountState{})
	model := &store.AgentModel{Name: "DeepSeek", Provider: "deepseek", Enabled: true, APIKey: crypto.EncryptedString(testAPIKey)}

	result := server.runLaunchPreflight("user-1", model, exchange, nil, false)
	if !result.Ready {
		t.Fatalf("warnings must not block launch, got %+v", result)
	}
}
