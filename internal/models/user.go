// Package models holds the GORM models backing the store's schema.
package models

import (
	"time"

	"gorm.io/gorm"
)

// UserRole is the permission level of a user.
type UserRole string

// Supported user roles.
const (
	UserRoleCustomer UserRole = "customer"
	UserRoleAdmin    UserRole = "admin"
)

// Valid reports whether r is a known role.
func (r UserRole) Valid() bool {
	return r == UserRoleCustomer || r == UserRoleAdmin
}

// User is a customer or administrator of the store.
type User struct {
	ID uint `json:"id" gorm:"primaryKey"`
	// The unique index is partial: a soft-deleted user must not reserve their
	// email forever, which a plain uniqueIndex would do.
	Email     string   `json:"email"                gorm:"not null;uniqueIndex:uniq_users_email,where:deleted_at IS NULL"`
	Password  string   `json:"-"                    gorm:"not null"`
	FirstName string   `json:"first_name"           gorm:"not null"`
	LastName  string   `json:"last_name"            gorm:"not null"`
	Phone     string   `json:"phone,omitempty"`
	IsActive  bool     `json:"is_active"            gorm:"not null;default:true"`
	Role      UserRole `json:"role"                 gorm:"type:varchar(20);not null;default:customer"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	RefreshTokens []RefreshToken `json:"-"`
	Orders        []Order        `json:"-"`
	Cart          *Cart          `json:"-"`
}

// IsAdmin reports whether the user has administrative rights.
func (u *User) IsAdmin() bool {
	return u.Role == UserRoleAdmin
}

// FullName is the user's display name.
func (u *User) FullName() string {
	return u.FirstName + " " + u.LastName
}

// RefreshToken is a long-lived token used to mint new access tokens.
type RefreshToken struct {
	ID     uint `json:"id"      gorm:"primaryKey"`
	UserID uint `json:"user_id" gorm:"not null;index"`
	// TokenHash is a SHA-256 hash of the refresh token, never the token
	// itself: a database leak must not hand out working credentials. Plain
	// SHA-256 rather than bcrypt is right here — the token is already
	// high-entropy, so there is nothing to brute force.
	//
	// The unique index is partial so a revoked token does not block reissuing.
	TokenHash string `json:"-" gorm:"not null;uniqueIndex:uniq_refresh_tokens_token_hash,where:deleted_at IS NULL"`
	// Indexed and NOT NULL: a token with no expiry never expires, and expiry
	// sweeps scan this column.
	ExpiresAt time.Time      `json:"expires_at" gorm:"not null;index"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	User *User `json:"-"`
}

// Expired reports whether the token is no longer usable at time now.
func (t *RefreshToken) Expired(now time.Time) bool {
	return now.After(t.ExpiresAt)
}
