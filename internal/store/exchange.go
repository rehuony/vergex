package store

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
var ErrDuplicateAccount = errors.New("this API key is already connected as another account")

// ExchangeStore exchange storage
type ExchangeStore struct {
	db *gorm.DB
}

// Exchange is an exchange account.
type Exchange struct {
	ID           string                 `gorm:"primaryKey" json:"id"`
	ExchangeType string                 `gorm:"column:venue" json:"exchange_type"`
	AccountName  string                 `gorm:"column:name" json:"account_name"`
	UserID       string                 `gorm:"column:user_id" json:"user_id"`
	APIKey       crypto.EncryptedString `gorm:"column:api_key" json:"apiKey"`
	SecretKey    crypto.EncryptedString `gorm:"column:secret_key" json:"secretKey"`
	Passphrase   crypto.EncryptedString `gorm:"column:passphrase" json:"passphrase"`
	// Identity is what an approval binds to: see AccountIdentity.
	Identity  string    `gorm:"column:identity;serializer:nulltext" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Exchange) TableName() string { return "trading.accounts" }

// BeforeCreate fills in what every account has however it is created.
func (e *Exchange) BeforeCreate(*gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	e.Identity = AccountIdentity(e.ExchangeType, string(e.APIKey))
	return nil
}

// VenueName is how the interface names the account's venue.
func (e Exchange) VenueName() string { return VenueName(e.ExchangeType) }

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
func AccountIdentity(venue, apiKey string) string {
	if apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(venue + ":" + apiKey))
	return hex.EncodeToString(sum[:])
}

var accountLocks sync.Map

// LockAccount serializes checking a lease and acting on the answer, in this process.
func LockAccount(accountID string) func() {
	lock, _ := accountLocks.LoadOrStore(accountID, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// LeaseHolderFrontier holds an account from a Frontier trade's approval until it ends.
const LeaseHolderFrontier = "frontier"

// AccountLease reserves an account for one holder.
type AccountLease struct {
	AccountID  string    `gorm:"column:account_id;primaryKey"`
	Holder     string    `gorm:"column:holder"`
	HolderID   string    `gorm:"column:holder_id"`
	AcquiredAt time.Time `gorm:"column:acquired_at;autoCreateTime"`
}

func (AccountLease) TableName() string { return "trading.account_leases" }

// NewExchangeStore creates a new ExchangeStore
func NewExchangeStore(db *gorm.DB) *ExchangeStore {
	return &ExchangeStore{db: db}
}

// List gets user's exchange list
func (s *ExchangeStore) List(userID string) ([]*Exchange, error) {
	var exchanges []*Exchange
	if uuid.Validate(userID) != nil {
		return exchanges, nil
	}
	err := s.db.Where("user_id = ?", userID).Order("venue, name").Find(&exchanges).Error
	if err != nil {
		return nil, err
	}
	return exchanges, nil
}

// ListNames is List without the keys, which stay unread and so are not decrypted.
func (s *ExchangeStore) ListNames(userID string) ([]*Exchange, error) {
	var exchanges []*Exchange
	if uuid.Validate(userID) != nil {
		return exchanges, nil
	}
	err := s.db.Select("id", "venue", "name").Where("user_id = ?", userID).Order("venue, name").Find(&exchanges).Error
	if err != nil {
		return nil, err
	}
	return exchanges, nil
}

// GetByID gets a specific exchange by UUID
func (s *ExchangeStore) GetByID(userID, id string) (*Exchange, error) {
	if uuid.Validate(id) != nil || uuid.Validate(userID) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var exchange Exchange
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&exchange).Error
	if err != nil {
		return nil, err
	}
	return &exchange, nil
}

// Get gets an account by UUID, whoever owns it, for what works across all of them.
func (s *ExchangeStore) Get(id string) (*Exchange, error) {
	if uuid.Validate(id) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var exchange Exchange
	if err := s.db.Where("id = ?", id).First(&exchange).Error; err != nil {
		return nil, err
	}
	return &exchange, nil
}

// ActiveIDs lists the accounts that something can be trading on: a running trader, a
// reserved Frontier trade, or a position still open in the ledger.
func (s *ExchangeStore) ActiveIDs() ([]string, error) {
	var ids []string
	err := s.db.Raw(`SELECT a.id::text FROM trading.accounts a WHERE
		EXISTS (SELECT 1 FROM trader.traders t WHERE t.account_id = a.id AND t.is_running)
		OR EXISTS (SELECT 1 FROM trading.account_leases l WHERE l.account_id = a.id)
		OR EXISTS (SELECT 1 FROM trading.positions p WHERE p.account_id = a.id AND p.status = ?)`, PositionOpen).Scan(&ids).Error
	return ids, err
}

// Leased reports whether anything has reserved the account.
func (s *ExchangeStore) Leased(id string) (bool, error) {
	var count int64
	err := s.db.Model(&AccountLease{}).Where("account_id = ?", id).Count(&count).Error
	return count != 0, err
}

// Create creates a new exchange account with UUID
func (s *ExchangeStore) Create(userID, exchangeType, accountName, apiKey, secretKey, passphrase string) (string, error) {
	if missing := MissingRequiredExchangeCredentialFields(exchangeType, apiKey, secretKey, passphrase); len(missing) > 0 {
		return "", fmt.Errorf("missing required exchange fields: %s", strings.Join(missing, ", "))
	}
	if accountName == "" {
		accountName = "Default"
	}

	exchange := &Exchange{
		ExchangeType: exchangeType,
		AccountName:  accountName,
		UserID:       userID,
		APIKey:       crypto.EncryptedString(apiKey),
		SecretKey:    crypto.EncryptedString(secretKey),
		Passphrase:   crypto.EncryptedString(passphrase),
	}
	if err := s.db.Create(exchange).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return "", ErrDuplicateAccount
		}
		return "", err
	}
	logger.Debugf("🔧 ExchangeStore.Create: userID=%s, exchangeType=%s, accountName=%s, id=%s",
		userID, exchangeType, accountName, exchange.ID)
	return exchange.ID, nil
}

// Update updates exchange configuration by UUID. A blank credential keeps the saved one.
func (s *ExchangeStore) Update(userID, id, apiKey, secretKey, passphrase string) error {
	logger.Debugf("🔧 ExchangeStore.Update: userID=%s, id=%s", userID, id)

	existing, err := s.GetByID(userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("exchange not found: id=%s, userID=%s", id, userID)
		}
		return err
	}

	updates := map[string]any{"updated_at": time.Now().UTC()}
	if apiKey != "" {
		updates["api_key"] = crypto.EncryptedString(apiKey)
		updates["identity"] = AccountIdentity(existing.ExchangeType, apiKey)
	}
	if secretKey != "" {
		updates["secret_key"] = crypto.EncryptedString(secretKey)
	}
	if passphrase != "" {
		updates["passphrase"] = crypto.EncryptedString(passphrase)
	}
	if err := s.db.Model(&Exchange{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrDuplicateAccount
		}
		return err
	}
	return nil
}

// Delete deletes an exchange account. A leased account stays, with
// gorm.ErrForeignKeyViolated.
func (s *ExchangeStore) Delete(userID, id string) error {
	if uuid.Validate(id) != nil || uuid.Validate(userID) != nil {
		return fmt.Errorf("exchange not found: id=%s, userID=%s", id, userID)
	}
	result := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Exchange{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("exchange not found: id=%s, userID=%s", id, userID)
	}
	logger.Infof("🗑️ Deleted exchange: id=%s, userID=%s", id, userID)
	return nil
}
