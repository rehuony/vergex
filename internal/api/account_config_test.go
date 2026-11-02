package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/account"
	"vergex/internal/database/testdb"
	"vergex/internal/security/crypto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAccountsAreListedByEnvironmentThenName(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	for _, row := range []struct {
		venue, name string
		testnet     bool
	}{
		{"okx", "alpha", false},
		{"binance", "beta", true},
		{"binance", "Zeta", false},
		{"binance", "alpha", true},
		{"binance", "Delta", false},
	} {
		if _, err := st.Accounts.Create(user, row.venue, row.name, row.venue+row.name, "secret", "passphrase", row.testnet); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.Accounts.List(user)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	if got := strings.Join(names, " "); got != "Delta Zeta alpha beta alpha" {
		t.Errorf("order = %s", got)
	}
}

func TestRenamingAnAccountChangesOnlyItsName(t *testing.T) {
	st := NewStores(testdb.Open(t))
	user := testdb.AddUser(t, st.DB)
	id, err := st.Accounts.Create(user, "binance", "Main", "fixture-key", "fixture-secret", "", false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.Accounts.GetByID(user, id)
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", c.GetHeader("Test-User")) })
	router.PUT("/accounts/:id/name", (&Server{stores: st}).handleRenameAccount)
	rename := func(as, target, body string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/accounts/"+target+"/name", strings.NewReader(body))
		r.Header.Set("Test-User", as)
		router.ServeHTTP(w, r)
		return w.Code
	}

	if code := rename(user, id, `{"name":"  Second  "}`); code != http.StatusOK {
		t.Fatalf("rename = %d", code)
	}
	after, err := st.Accounts.GetByID(user, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Second" {
		t.Errorf("name = %q", after.Name)
	}
	if after.APIKey != before.APIKey || after.SecretKey != before.SecretKey || after.Identity != before.Identity || after.Venue != before.Venue || after.Testnet != before.Testnet {
		t.Error("a rename changed more than the name")
	}
	if code := rename(user, id, `{"name":"   "}`); code != http.StatusBadRequest {
		t.Errorf("an empty name got %d", code)
	}
	if code := rename(uuid.NewString(), id, `{"name":"Mine now"}`); code != http.StatusNotFound {
		t.Errorf("another user got %d", code)
	}
	if code := rename(user, "not-a-uuid", `{"name":"Second"}`); code != http.StatusNotFound {
		t.Errorf("a malformed ID got %d", code)
	}
}

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
