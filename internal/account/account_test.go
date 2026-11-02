package account

import (
	"errors"
	"testing"

	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/user"

	"gorm.io/gorm"
)

func newTestAccountStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t)
	if err := user.NewStore(db).Create(&user.User{ID: testdb.UserID, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	return NewStore(db), db
}

func TestOneAPIKeyCannotBeConnectedTwice(t *testing.T) {
	s, _ := newTestAccountStore(t)
	first, err := s.Create(testdb.UserID, "binance", "Main", "key-1", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(testdb.UserID, "binance", "Copy", "key-1", "secret", ""); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second account on one key: %v", err)
	}
	second, err := s.Create(testdb.UserID, "binance", "Other", "key-2", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(testdb.UserID, second, "key-1", "", ""); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("moving onto a taken key: %v", err)
	}

	// A new key moves the identity with it, and frees the old one.
	if err := s.Update(testdb.UserID, first, "key-3", "", ""); err != nil {
		t.Fatal(err)
	}
	account, err := s.GetByID(testdb.UserID, first)
	if err != nil || account.Identity != Identity("binance", "key-3") || string(account.APIKey) != "key-3" {
		t.Fatalf("account = %+v, err %v", account, err)
	}
	if _, err := s.Create(testdb.UserID, "binance", "Again", "key-1", "secret", ""); err != nil {
		t.Fatalf("the old key should be free: %v", err)
	}
	// The same key on another venue is another account.
	if _, err := s.Create(testdb.UserID, "okx", "OKX", "key-1", "secret", "pass"); err != nil {
		t.Fatal(err)
	}
}

func TestALeasedAccountCannotBeDeleted(t *testing.T) {
	s, db := newTestAccountStore(t)
	id, err := s.Create(testdb.UserID, "binance", "Main", "key-1", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if leased, err := s.Leased(id); err != nil || leased {
		t.Fatalf("new account leased = %v, err %v", leased, err)
	}
	lease := Lease{AccountID: id, Holder: LeaseHolderFutures, HolderID: "run-1"}
	if err := db.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if leased, err := s.Leased(id); err != nil || !leased {
		t.Fatalf("leased = %v, err %v", leased, err)
	}
	if err := db.Create(&Lease{AccountID: id, Holder: LeaseHolderFutures, HolderID: "run-2"}).Error; err == nil {
		t.Fatal("two holders on one account")
	}
	if err := s.Delete(testdb.UserID, id); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("deleting a leased account: %v", err)
	}
	if err := db.Delete(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(testdb.UserID, id); err != nil {
		t.Fatalf("deleting a free account: %v", err)
	}
}

func TestMalformedAccountIDsAreNotFound(t *testing.T) {
	s, _ := newTestAccountStore(t)
	if _, err := s.GetByID(testdb.UserID, "not-a-uuid"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetByID: %v", err)
	}
	if err := s.Delete(testdb.UserID, "not-a-uuid"); err == nil {
		t.Fatal("Delete accepted a malformed ID")
	}
	if accounts, err := s.List("not-a-uuid"); err != nil || len(accounts) != 0 {
		t.Fatalf("List = %v, err %v", accounts, err)
	}
}

// Names are read every 30 seconds for the Dashboard, which has no use for a key.
func TestListingNamesLeavesTheKeysUnread(t *testing.T) {
	s, db := newTestAccountStore(t)
	if _, err := s.Create(testdb.UserID, "binance", "Main", "key-1", "secret", ""); err != nil {
		t.Fatal(err)
	}
	accounts, err := s.ListNames(testdb.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Name != "Main" || accounts[0].Venue != "binance" || accounts[0].ID == "" {
		t.Fatalf("accounts = %+v", accounts)
	}
	if accounts[0].APIKey != "" || accounts[0].SecretKey != "" {
		t.Fatal("listing names read the keys")
	}

	models := agent.NewModelStore(db)
	if err := models.Save(testdb.UserID, "deepseek", "DeepSeek", true, "model-key", "https://example.com/v1", ""); err != nil {
		t.Fatal(err)
	}
	endpoints, err := models.ListEndpoints(testdb.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 || endpoints[0].Provider != "deepseek" || endpoints[0].CustomAPIURL != "https://example.com/v1" || endpoints[0].ID == "" {
		t.Fatalf("endpoints = %+v", endpoints)
	}
	if endpoints[0].APIKey != "" {
		t.Fatal("listing endpoints read the key")
	}
}
