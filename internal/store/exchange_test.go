package store

import (
	"errors"
	"testing"

	"vergex/internal/store/database/testdb"

	"gorm.io/gorm"
)

func newTestExchangeStore(t *testing.T) (*ExchangeStore, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t)
	if err := NewUserStore(db).Create(&User{ID: conversationUser, Email: "me@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	return NewExchangeStore(db), db
}

func TestOneAPIKeyCannotBeConnectedTwice(t *testing.T) {
	s, _ := newTestExchangeStore(t)
	first, err := s.Create(conversationUser, "binance", "Main", "key-1", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(conversationUser, "binance", "Copy", "key-1", "secret", ""); !errors.Is(err, ErrDuplicateAccount) {
		t.Fatalf("second account on one key: %v", err)
	}
	second, err := s.Create(conversationUser, "binance", "Other", "key-2", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(conversationUser, second, "key-1", "", ""); !errors.Is(err, ErrDuplicateAccount) {
		t.Fatalf("moving onto a taken key: %v", err)
	}

	// A new key moves the identity with it, and frees the old one.
	if err := s.Update(conversationUser, first, "key-3", "", ""); err != nil {
		t.Fatal(err)
	}
	account, err := s.GetByID(conversationUser, first)
	if err != nil || account.Identity != AccountIdentity("binance", "key-3") || string(account.APIKey) != "key-3" {
		t.Fatalf("account = %+v, err %v", account, err)
	}
	if _, err := s.Create(conversationUser, "binance", "Again", "key-1", "secret", ""); err != nil {
		t.Fatalf("the old key should be free: %v", err)
	}
	// The same key on another venue is another account.
	if _, err := s.Create(conversationUser, "okx", "OKX", "key-1", "secret", "pass"); err != nil {
		t.Fatal(err)
	}
}

func TestALeasedAccountCannotBeDeleted(t *testing.T) {
	s, db := newTestExchangeStore(t)
	id, err := s.Create(conversationUser, "binance", "Main", "key-1", "secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if leased, err := s.Leased(id); err != nil || leased {
		t.Fatalf("new account leased = %v, err %v", leased, err)
	}
	lease := AccountLease{AccountID: id, Holder: LeaseHolderFrontier, HolderID: "run-1"}
	if err := db.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if leased, err := s.Leased(id); err != nil || !leased {
		t.Fatalf("leased = %v, err %v", leased, err)
	}
	if err := db.Create(&AccountLease{AccountID: id, Holder: LeaseHolderFrontier, HolderID: "run-2"}).Error; err == nil {
		t.Fatal("two holders on one account")
	}
	if err := s.Delete(conversationUser, id); !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("deleting a leased account: %v", err)
	}
	if err := db.Delete(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(conversationUser, id); err != nil {
		t.Fatalf("deleting a free account: %v", err)
	}
}

func TestMalformedAccountIDsAreNotFound(t *testing.T) {
	s, _ := newTestExchangeStore(t)
	if _, err := s.GetByID(conversationUser, "not-a-uuid"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetByID: %v", err)
	}
	if err := s.Delete(conversationUser, "not-a-uuid"); err == nil {
		t.Fatal("Delete accepted a malformed ID")
	}
	if accounts, err := s.List("not-a-uuid"); err != nil || len(accounts) != 0 {
		t.Fatalf("List = %v, err %v", accounts, err)
	}
}

// Names are read every 30 seconds for the Dashboard, which has no use for a key.
func TestListingNamesLeavesTheKeysUnread(t *testing.T) {
	s, db := newTestExchangeStore(t)
	if _, err := s.Create(conversationUser, "binance", "Main", "key-1", "secret", ""); err != nil {
		t.Fatal(err)
	}
	accounts, err := s.ListNames(conversationUser)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].AccountName != "Main" || accounts[0].ExchangeType != "binance" || accounts[0].ID == "" {
		t.Fatalf("accounts = %+v", accounts)
	}
	if accounts[0].APIKey != "" || accounts[0].SecretKey != "" {
		t.Fatal("listing names read the keys")
	}

	models := NewAgentModelStore(db)
	if err := models.Save(conversationUser, "deepseek", "DeepSeek", true, "model-key", "https://example.com/v1", ""); err != nil {
		t.Fatal(err)
	}
	endpoints, err := models.ListEndpoints(conversationUser)
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
