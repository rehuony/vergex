package api

import (
	"testing"

	"vergex/internal/crypto"
	"vergex/internal/store"
)

func TestSafeExchangeConfigFromStoreIncludesCredentialPresenceFlags(t *testing.T) {
	cfg := &store.Exchange{
		ID:           "ex-1",
		ExchangeType: "okx",
		AccountName:  "OKX Main",
		Name:         "OKX Main",
		Type:         "cex",
		Enabled:      true,
		APIKey:       crypto.EncryptedString("api-test-123"),
		SecretKey:    crypto.EncryptedString("secret-test-123"),
		Passphrase:   crypto.EncryptedString("passphrase-test-123"),
	}

	safe := safeExchangeConfigFromStore(cfg)
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
