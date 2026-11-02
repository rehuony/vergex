package futures

import (
	"time"

	"vergex/internal/account"

	"gorm.io/gorm"
)

// The states of a manual close.
const (
	AccountCloseUnconfirmed = "unconfirmed"
	AccountCloseConfirmed   = "confirmed"
)

// AccountClose is a manual close of a position, saved before it is sent. A timeout,
// reload or duplicate click must never authorize another close without reconciling the
// original exchange order.
type AccountClose struct {
	ID         string        `gorm:"primaryKey"`
	UserID     string        `gorm:"column:user_id"`
	AccountID  string        `gorm:"column:account_id"`
	AccountKey string        `gorm:"column:account_identity"`
	Symbol     string        `gorm:"column:symbol"`
	Side       string        `gorm:"column:side"`
	Quantity   string        `gorm:"column:quantity"`
	Entry      string        `gorm:"column:entry"`
	State      string        `gorm:"column:state"`
	Order      account.Order `gorm:"column:order_state;serializer:json"`
	UpdatedAt  time.Time     `gorm:"column:updated_at"`
}

func (AccountClose) TableName() string { return "futures.account_closes" }

// AccountCloseStore keeps manual closes.
type AccountCloseStore struct {
	db *gorm.DB
}

// NewAccountCloseStore keeps manual closes on a connection.
func NewAccountCloseStore(db *gorm.DB) *AccountCloseStore {
	return &AccountCloseStore{db: db}
}

// Get returns a close by its request ID.
func (s *AccountCloseStore) Get(id string) (*AccountClose, error) {
	var row AccountClose
	if err := s.db.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Unconfirmed returns the closes of a position still waiting on the exchange.
func (s *AccountCloseStore) Unconfirmed(accountKey, symbol, side string) ([]AccountClose, error) {
	var rows []AccountClose
	err := s.db.Where("account_identity = ? AND symbol = ? AND side = ? AND state = ?", accountKey, symbol, side, AccountCloseUnconfirmed).Find(&rows).Error
	return rows, err
}

// Create saves a close before it is sent.
func (s *AccountCloseStore) Create(row *AccountClose) error {
	return s.db.Create(row).Error
}

// Save saves what is now known of a close.
func (s *AccountCloseStore) Save(row *AccountClose) error {
	return s.db.Save(row).Error
}
