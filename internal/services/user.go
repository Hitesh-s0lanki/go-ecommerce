package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// ErrUserNotFound is returned when a user does not exist, or exists but is
// deactivated — a token outlives the account it was issued for.
var ErrUserNotFound = errors.New("user not found")

// UserService reads and updates user profiles.
type UserService struct {
	db *gorm.DB
}

// NewUserService builds a UserService.
func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

// GetProfile loads a user.
//
// Deactivated accounts are reported as not found: an access token stays valid
// for its full lifetime, so the account behind it must be re-checked rather
// than assumed live.
func (s *UserService) GetProfile(ctx context.Context, userID uint) (*dto.UserResponse, error) {
	var user models.User

	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("find user: %w", err)
	}

	if !user.IsActive {
		return nil, ErrUserNotFound
	}

	resp := dto.NewUserResponse(&user)

	return &resp, nil
}

// UpdateProfile replaces a user's editable details.
//
// Only the three profile columns are written. The reference loads the row and
// calls Save, which rewrites every column from the copy it read — so a role or
// email change committed in between is silently reverted. Naming the columns
// also means the fields a caller must never set (role, is_active, password)
// cannot be touched even by mistake.
func (s *UserService) UpdateProfile(ctx context.Context, userID uint, req *dto.UpdateProfileRequest) (*dto.UserResponse, error) {
	var user models.User

	// Postgres can return the updated row, so this is one round trip rather
	// than the reference's three (load, save, re-load).
	result := s.db.WithContext(ctx).
		Model(&user).
		Clauses(clause.Returning{}).
		Where("id = ? AND is_active = ?", userID, true).
		Updates(map[string]any{
			"first_name": req.FirstName,
			"last_name":  req.LastName,
			"phone":      req.Phone,
		})

	if result.Error != nil {
		return nil, fmt.Errorf("update user: %w", result.Error)
	}

	// No row matched: the user is gone or deactivated.
	if result.RowsAffected == 0 {
		return nil, ErrUserNotFound
	}

	resp := dto.NewUserResponse(&user)

	return &resp, nil
}
