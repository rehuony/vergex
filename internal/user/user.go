package user

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserStore user storage
type Store struct {
	db *gorm.DB
}

// User user model
type User struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	Avatar       string    `gorm:"column:avatar" json:"avatar"` // small image as a data URL; empty shows the initial
	DisplayName  string    `gorm:"column:display_name" json:"display_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "core.users" }

// DefaultDisplayName is the name a new account starts with until its owner
// sets one in settings.
const DefaultDisplayName = "Admin"

// NewUserStore creates a new UserStore
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Create creates user
func (s *Store) Create(user *User) error {
	if user.DisplayName == "" {
		user.DisplayName = DefaultDisplayName
	}
	return s.db.Create(user).Error
}

// GetByEmail gets user by email
func (s *Store) GetByEmail(email string) (*User, error) {
	var user User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID gets user by ID
func (s *Store) GetByID(userID string) (*User, error) {
	if uuid.Validate(userID) != nil {
		return nil, gorm.ErrRecordNotFound
	}
	var user User
	err := s.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Count returns the total number of users
func (s *Store) Count() (int, error) {
	var count int64
	err := s.db.Model(&User{}).Count(&count).Error
	return int(count), err
}

// GetAllIDs gets all user IDs
func (s *Store) GetAllIDs() ([]string, error) {
	var userIDs []string
	err := s.db.Model(&User{}).Order("id").Pluck("id", &userIDs).Error
	return userIDs, err
}

// GetAll returns all users ordered by creation time.
func (s *Store) GetAll() ([]User, error) {
	var users []User
	err := s.db.Model(&User{}).Order("created_at").Find(&users).Error
	return users, err
}

// UpdatePassword updates password
func (s *Store) UpdatePassword(userID, passwordHash string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"password_hash": passwordHash,
		"updated_at":    time.Now().UTC(),
	}).Error
}

// UpdateAvatar stores the avatar data URL; an empty string removes it.
func (s *Store) UpdateAvatar(userID, avatar string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"avatar":     avatar,
		"updated_at": time.Now().UTC(),
	}).Error
}

// UpdateDisplayName stores the name shown in the account menu.
func (s *Store) UpdateDisplayName(userID, name string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"display_name": name,
		"updated_at":   time.Now().UTC(),
	}).Error
}
