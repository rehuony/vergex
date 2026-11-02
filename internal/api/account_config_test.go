package api

import (
	"testing"

	"vergex/internal/account"
	"vergex/internal/security/crypto"
)

func TestSafeAccountConfigFromStoreIncludesCredentialPresenceFlags(t *testing.T) {
	cfg := &account.Account{
		ID:         "ex-1",
		Venue:      "okx",
		Name:       "OKX Main",
		APIKey:     crypto.EncryptedString("api-test-123"),
		SecretKey:  crypto.EncryptedString("secret-test-123"),
		Passphrase: crypto.EncryptedString("passphrase-test-123"),
	}

	safe := safeAccountConfigFromStore(cfg)
	if !safe.HasAPIKey {
		t.Fatalf("expected has_api_key to be true")
	}
	if !safe.HasSecretKey {
		t.Fatalf("expected has_secret_key to be true")
	}
	if !safe.HasPassphrase {
		t.Fatalf("expected has_passphrase to be true")
	}
}
