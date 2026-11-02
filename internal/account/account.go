package account

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"vergex/internal/logger"
	"vergex/internal/security/crypto"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrDuplicateAccount means an API key is already connected as an account. Two
// accounts on one key would trade the same money without knowing of each other.
var ErrDuplicate = errors.New("this API key is already connected as another account")

// AccountStore account storage
type Store struct {
	db *gorm.DB
}

// Account is an account on a venue: a set of API keys.
type Account struct {
	ID         string                 `gorm:"primaryKey" json:"id"`
	Venue      string                 `gorm:"column:venue" json:"venue"`
	Name       string                 `gorm:"column:name" json:"name"`
	UserID     string                 `gorm:"column:user_id" json:"user_id"`
	APIKey     crypto.EncryptedString `gorm:"column:api_key" json:"apiKey"`
	SecretKey  crypto.EncryptedString `gorm:"column:secret_key" json:"secretKey"`
	Passphrase crypto.EncryptedString `gorm:"column:passphrase" json:"passphrase"`
	// Testnet says the keys reach the venue's test environment. It never changes.
	Testnet bool `gorm:"column:testnet" json:"testnet"`
	// Identity is what an approval binds to: see AccountIdentity.
	Identity  string    `gorm:"column:identity;serializer:nulltext" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Account) TableName() string { return "account.accounts" }

// BeforeCreate fills in what every account has however it is created.
func (e *Account) BeforeCreate(*gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	e.Identity = Identity(e.Venue, string(e.APIKey))
	return nil
}

// VenueName is how the interface names the account's venue.
func (e Account) VenueName() string { return VenueName(e.Venue) }

// VenueName is how the interface names a venue.
func VenueName(venue string) string {
	switch venue {
	case "binance":
		return "Binance Futures"
	case "okx":
		return "OKX Futures"
	default:
		return venue + " Exchange"
	}
}

// AccountIdentity derives an account's identity from its API key. API keys are
// account identifiers; different keys for one exchange account require operator
// coordination because the account UID is not exposed here. An account with no key
// has no identity.
func Identity(venue, apiKey string) string {
	if apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(venue + ":" + apiKey))
	return hex.EncodeToString(sum[:])
}

var accountLocks sync.Map

// LockAccount serializes checking a lease and acting on the answer, in this process.
func Lock(accountID string) func() {
	lock, _ := accountLocks.LoadOrStore(accountID, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// LeaseHolderFutures holds an account from a Futures trade's approval until it ends.
const LeaseHolderFutures = "futures"

// AccountLease reserves an account for one holder.
type Lease struct {
	AccountID  string    `gorm:"column:account_id;primaryKey"`
	Holder     string    `gorm:"column:holder"`
	HolderID   string    `gorm:"column:holder_id"`
	AcquiredAt time.Time `gorm:"column:acquired_at;autoCreateTime"`
}

func (Lease) TableName() string { return "account.leases" }

// NewAccountStore creates a new AccountStore
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// List gets user's accounts, by venue, real before testnet, then by name.
func (s *Store) List(userID string) ([]*Account, error) {
	var accounts []*Account
	if uuid.Validate(userID) != nil {
		return accounts, nil
	}
	err := s.db.Where("user_id = ?", userID).Order("venue, testnet, lower(name), id").Find(&accounts).Error
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

// ListNames is List without the keys, which stay unread and so are not decrypted.
func (s *Store) ListNames(userID string) ([]*Account, error) {
	var accounts []*Account
	if uuid.Validate(userID) != nil {
		return accounts, nil
	}
	err := s.db.Select("id", "venue", "name", "testnet").Where("user_id = ?", userID).Order("venue, testnet, lower(name), id").Find(&accounts).Error
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

// GetByID gets a specific account by UUID
func (s *Store) GetByID(userID, id string) (*Account, error) {
	if uuid.Validate(id) != nil || uuid.Validate(userID) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var account Account
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// Get gets an account by UUID, whoever owns it, for what works across all of them.
func (s *Store) Get(id string) (*Account, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var account Account
	if err := s.db.Where("id = ?", id).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

// ActiveIDs lists the accounts that the ledger knows something can be trading on: a
// reserved one, or one with a position still open.
func (s *Store) ActiveIDs() ([]string, error) {
	var ids []string
	err := s.db.Raw(`SELECT a.id::text FROM account.accounts a WHERE
		EXISTS (SELECT 1 FROM account.leases l WHERE l.account_id = a.id)
		OR EXISTS (SELECT 1 FROM account.positions p WHERE p.account_id = a.id AND p.status = ?)`, PositionOpen).Scan(&ids).Error
	return ids, err
}

// Leased reports whether anything has reserved the account.
func (s *Store) Leased(id string) (bool, error) {
	var count int64
	err := s.db.Model(&Lease{}).Where("account_id = ?", id).Count(&count).Error
	return count != 0, err
}

// Create creates a new account with UUID
func (s *Store) Create(userID, venue, accountName, apiKey, secretKey, passphrase string, testnet bool) (string, error) {
	if missing := MissingCredentialFields(venue, apiKey, secretKey, passphrase); len(missing) > 0 {
		return "", fmt.Errorf("missing required account fields: %s", strings.Join(missing, ", "))
	}
	if accountName == "" {
		accountName = "Default"
	}

	account := &Account{
		Venue:      venue,
		Name:       accountName,
		UserID:     userID,
		APIKey:     crypto.EncryptedString(apiKey),
		SecretKey:  crypto.EncryptedString(secretKey),
		Passphrase: crypto.EncryptedString(passphrase),
		Testnet:    testnet,
	}
	if err := s.db.Create(account).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return "", ErrDuplicate
		}
		return "", err
	}
	logger.Debugf("🔧 AccountStore.Create: userID=%s, venue=%s, accountName=%s, id=%s",
		userID, venue, accountName, account.ID)
	return account.ID, nil
}

// Update updates an account by UUID. A blank credential keeps the saved one.
func (s *Store) Update(userID, id, apiKey, secretKey, passphrase string) error {
	logger.Debugf("🔧 AccountStore.Update: userID=%s, id=%s", userID, id)

	existing, err := s.GetByID(userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("account not found: id=%s, userID=%s", id, userID)
		}
		return err
	}

	updates := map[string]any{"updated_at": time.Now().UTC()}
	if apiKey != "" {
		updates["api_key"] = crypto.EncryptedString(apiKey)
		updates["identity"] = Identity(existing.Venue, apiKey)
	}
	if secretKey != "" {
		updates["secret_key"] = crypto.EncryptedString(secretKey)
	}
	if passphrase != "" {
		updates["passphrase"] = crypto.EncryptedString(passphrase)
	}
	if err := s.db.Model(&Account{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// Rename changes the name an account goes by. Nothing trades on it, so whatever is using
// the account carries on.
func (s *Store) Rename(userID, id, name string) error {
	if uuid.Validate(id) != nil || uuid.Validate(userID) != nil {
		return gorm.ErrRecordNotFound
	}
	result := s.db.Model(&Account{}).Where("id = ? AND user_id = ?", id, userID).Update("name", name)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete deletes an account. A leased account stays, with
// gorm.ErrForeignKeyViolated.
func (s *Store) Delete(userID, id string) error {
	if uuid.Validate(id) != nil || uuid.Validate(userID) != nil {
		return fmt.Errorf("account not found: id=%s, userID=%s", id, userID)
	}
	result := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Account{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("account not found: id=%s, userID=%s", id, userID)
	}
	logger.Infof("🗑️ Deleted exchange: id=%s, userID=%s", id, userID)
	return nil
}
