package store

import (
	"fmt"
	"strings"
	"time"
	"vergex/internal/crypto"
	"vergex/internal/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExchangeStore exchange storage
type ExchangeStore struct {
	db *gorm.DB
}

// Exchange exchange configuration
type Exchange struct {
	ID           string                 `gorm:"primaryKey" json:"id"`
	ExchangeType string                 `gorm:"column:exchange_type;not null;default:''" json:"exchange_type"`
	AccountName  string                 `gorm:"column:account_name;not null;default:''" json:"account_name"`
	UserID       string                 `gorm:"column:user_id;not null;default:default;index" json:"user_id"`
	Name         string                 `gorm:"not null" json:"name"`
	Type         string                 `gorm:"not null" json:"type"` // "cex" or "dex"
	Enabled      bool                   `gorm:"default:false" json:"enabled"`
	APIKey       crypto.EncryptedString `gorm:"column:api_key;default:''" json:"apiKey"`
	SecretKey    crypto.EncryptedString `gorm:"column:secret_key;default:''" json:"secretKey"`
	Passphrase   crypto.EncryptedString `gorm:"column:passphrase;default:''" json:"passphrase"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

func (Exchange) TableName() string { return "exchanges" }

// NewExchangeStore creates a new ExchangeStore
func NewExchangeStore(db *gorm.DB) *ExchangeStore {
	return &ExchangeStore{db: db}
}

func (s *ExchangeStore) initTables() error {
	return s.db.AutoMigrate(&Exchange{})
}

func (s *ExchangeStore) initDefaultData() error {
	// No longer pre-populate exchanges - create on demand when user configures
	return nil
}

// List gets user's exchange list
func (s *ExchangeStore) List(userID string) ([]*Exchange, error) {
	var exchanges []*Exchange
	err := s.db.Where("user_id = ?", userID).Order("exchange_type, account_name").Find(&exchanges).Error
	if err != nil {
		return nil, err
	}
	return exchanges, nil
}

// GetByID gets a specific exchange by UUID
func (s *ExchangeStore) GetByID(userID, id string) (*Exchange, error) {
	var exchange Exchange
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&exchange).Error
	if err != nil {
		return nil, err
	}
	return &exchange, nil
}

// getExchangeNameAndType returns the display name and type for an exchange type
func getExchangeNameAndType(exchangeType string) (name string, typ string) {
	switch exchangeType {
	case "binance":
		return "Binance Futures", "cex"
	case "okx":
		return "OKX Futures", "cex"
	default:
		return exchangeType + " Exchange", "cex"
	}
}

// Create creates a new exchange account with UUID
func (s *ExchangeStore) Create(userID, exchangeType, accountName string, enabled bool,
	apiKey, secretKey, passphrase string) (string, error) {

	if missing := MissingRequiredExchangeCredentialFields(exchangeType, apiKey, secretKey, passphrase); len(missing) > 0 {
		return "", fmt.Errorf("missing required exchange fields: %s", strings.Join(missing, ", "))
	}

	id := uuid.New().String()
	name, typ := getExchangeNameAndType(exchangeType)

	if accountName == "" {
		accountName = "Default"
	}

	logger.Debugf("🔧 ExchangeStore.Create: userID=%s, exchangeType=%s, accountName=%s, id=%s",
		userID, exchangeType, accountName, id)

	exchange := &Exchange{
		ID:           id,
		ExchangeType: exchangeType,
		AccountName:  accountName,
		UserID:       userID,
		Name:         name,
		Type:         typ,
		Enabled:      true,
		APIKey:       crypto.EncryptedString(apiKey),
		SecretKey:    crypto.EncryptedString(secretKey),
		Passphrase:   crypto.EncryptedString(passphrase),
	}

	if err := s.db.Create(exchange).Error; err != nil {
		return "", err
	}
	return id, nil
}

// Update updates exchange configuration by UUID
func (s *ExchangeStore) Update(userID, id string, enabled bool, apiKey, secretKey, passphrase string) error {

	logger.Debugf("🔧 ExchangeStore.Update: userID=%s, id=%s", userID, id)

	updates := map[string]interface{}{
		"enabled":    true,
		"updated_at": time.Now().UTC(),
	}

	// Only update encrypted fields if not empty
	if apiKey != "" {
		updates["api_key"] = crypto.EncryptedString(apiKey)
	}
	if secretKey != "" {
		updates["secret_key"] = crypto.EncryptedString(secretKey)
	}
	if passphrase != "" {
		updates["passphrase"] = crypto.EncryptedString(passphrase)
	}

	result := s.db.Model(&Exchange{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("exchange not found: id=%s, userID=%s", id, userID)
	}
	return nil
}

// Delete deletes an exchange account
func (s *ExchangeStore) Delete(userID, id string) error {
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
