package store

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"vergex/internal/security/crypto"
	"vergex/internal/store/database/testdb"
)

func TestBotTokenIsEncryptedAtRest(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(crypto.EnvRSAPrivateKey, string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})))
	t.Setenv(crypto.EnvDataEncryptionKey, "a key for this test")
	service, err := crypto.NewCryptoService()
	if err != nil {
		t.Fatal(err)
	}
	crypto.SetGlobalCryptoService(service)
	t.Cleanup(func() { crypto.SetGlobalCryptoService(nil) })

	db := testdb.Open(t)
	st := NewTelegramConfigStore(db)
	const token = "123456:the-bot-token"
	if err := st.Save(token, ""); err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := db.Raw("SELECT bot_token FROM telegram.bot").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored == "" || strings.Contains(stored, "the-bot-token") {
		t.Fatalf("stored token = %q", stored)
	}
	cfg, err := st.Get()
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.BotToken) != token {
		t.Fatalf("read token = %q", cfg.BotToken)
	}
}

func TestMutedAlertsAndBindingRoundTrip(t *testing.T) {
	st := NewTelegramConfigStore(testdb.Open(t))
	if err := st.BindUser(42, "me"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMutedAlerts([]string{"position_closed"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := st.Get()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChatID != 42 || cfg.BoundAt == nil || !cfg.AlertMuted("position_closed") || cfg.AlertMuted("other") {
		t.Fatalf("cfg = %+v", cfg)
	}
	if err := st.Unbind(); err != nil {
		t.Fatal(err)
	}
	if cfg, err = st.Get(); err != nil {
		t.Fatal(err)
	}
	if cfg.ChatID != 0 || cfg.BoundAt != nil || !cfg.AlertMuted("position_closed") {
		t.Fatalf("after unbind = %+v", cfg)
	}
}
