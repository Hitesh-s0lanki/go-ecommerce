// Package services holds the business logic behind the HTTP handlers.
package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/events"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// Auth errors. They are deliberately few and vague: an attacker must not be
// able to tell "no such account" from "wrong password", or a probe of the
// registration endpoint would enumerate the user table.
var (
	ErrEmailTaken          = errors.New("email is not available")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

// decoyHash is a bcrypt hash of a value nobody knows.
//
// Login compares against it when the email does not exist, so a missing
// account costs the same ~100ms as a wrong password. Returning early instead
// would let an attacker enumerate accounts with a stopwatch.
const decoyHash = "$2a$12$5MOpVW3a9BBDmnHQPHiVHO24MrS7B0acylZSqDSTqYPQ01r6X3Umu"

// AuthService implements registration, login, refresh, and logout.
type AuthService struct {
	db         *gorm.DB
	tokens     *auth.TokenManager
	refreshTTL time.Duration
	logger     zerolog.Logger
	events     events.Publisher
}

// NewAuthService builds an AuthService.
//
// publisher may be events.Noop, which is what runs when events are switched
// off, so this never has to check whether one is configured.
func NewAuthService(
	db *gorm.DB,
	tokens *auth.TokenManager,
	refreshTTL time.Duration,
	logger *zerolog.Logger,
	publisher events.Publisher,
) *AuthService {
	return &AuthService{
		db:         db,
		tokens:     tokens,
		refreshTTL: refreshTTL,
		logger:     *logger,
		events:     publisher,
	}
}

// publishUserEvent announces something that happened to a user.
//
// Deliberately best-effort. The reference returns the publish error to the
// caller, so an SQS outage means nobody can log in or register — an outage of
// the thing that sends the welcome email takes down the front door. An event
// is a side effect of the request, not the point of it.
//
// The cost is that an event can be dropped: the queue is down, and the welcome
// email never sends. That is the right trade for these two, and if an event
// ever must not be lost — payment taken, order placed — the answer is to write
// it to the database in the same transaction and have a relay publish it, not
// to fail the request.
func (s *AuthService) publishUserEvent(ctx context.Context, eventType string, user *models.User) {
	payload := events.UserEvent{
		UserID:    user.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      string(user.Role),
	}

	if err := s.events.Publish(ctx, eventType, payload, nil); err != nil {
		s.logger.Error().
			Err(err).
			Str("event", eventType).
			Uint("user_id", user.ID).
			Msg("failed to publish event")
	}
}

// Register creates a user, their cart, and an initial token pair.
func (s *AuthService) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.AuthResponse, error) {
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		// Length rules are enforced by binding tags, so anything here is ours.
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := models.User{
		Email:     req.Email,
		Password:  hash,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Phone:     req.Phone,
		// Never from the request: that would let a caller register as admin.
		Role:     models.UserRoleCustomer,
		IsActive: true,
	}

	var resp *dto.AuthResponse

	// One transaction: a user without a cart, or a user with no tokens, is a
	// broken account. The reference creates them separately and only prints a
	// message when the cart fails.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// No "does this email exist" check first: between that query and the
		// insert another request can register the same address. The unique
		// index is the only real arbiter, so let it decide.
		if createErr := tx.Create(&user).Error; createErr != nil {
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				return ErrEmailTaken
			}

			return fmt.Errorf("create user: %w", createErr)
		}

		if createErr := tx.Create(&models.Cart{UserID: user.ID}).Error; createErr != nil {
			return fmt.Errorf("create cart: %w", createErr)
		}

		var issueErr error

		resp, issueErr = s.issueTokens(tx, &user)

		return issueErr
	})
	if err != nil {
		return nil, err
	}

	// After the commit, never inside it: an event published from inside the
	// transaction announces a user who may not exist a moment later, and
	// consumers are fast enough to act on one before the rollback lands.
	s.publishUserEvent(ctx, events.TypeUserRegistered, &user)

	return resp, nil
}

// Login exchanges credentials for a token pair.
func (s *AuthService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error) {
	var user models.User

	err := s.db.WithContext(ctx).Where("email = ?", req.Email).First(&user).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// Spend the same time as a real comparison before failing.
		_ = auth.CheckPassword(decoyHash, req.Password)

		return nil, ErrInvalidCredentials
	case err != nil:
		return nil, fmt.Errorf("find user: %w", err)
	}

	if checkErr := auth.CheckPassword(user.Password, req.Password); checkErr != nil {
		if !errors.Is(checkErr, auth.ErrPasswordMismatch) {
			// A corrupt hash is our bug, not a failed login.
			s.logger.Error().Err(checkErr).Uint("user_id", user.ID).Msg("password check failed")
		}

		return nil, ErrInvalidCredentials
	}

	// Checked after the password so a deactivated account is indistinguishable
	// from a wrong password.
	if !user.IsActive {
		return nil, ErrInvalidCredentials
	}

	var resp *dto.AuthResponse

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var issueErr error

		resp, issueErr = s.issueTokens(tx, &user)

		return issueErr
	})
	if err != nil {
		return nil, err
	}

	s.publishUserEvent(ctx, events.TypeUserLoggedIn, &user)

	return resp, nil
}

// Refresh rotates a refresh token for a new pair.
//
// It publishes nothing. The reference emits its login event from the helper
// all three of these share, so registering fires "logged in" as well, and so
// does every token rotation — which for a client refreshing on a timer is an
// event per interval, and a welcome email per interval for anything listening.
//
// Rotation means the presented token is revoked as the new one is issued, so a
// stolen token is usable at most once. Replay of an already-rotated token is
// treated as theft: every session for that user is revoked.
func (s *AuthService) Refresh(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.AuthResponse, error) {
	// Verify the signature and that this is a refresh token, before touching
	// the database.
	claims, err := s.tokens.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	hash := hashToken(req.RefreshToken)

	var (
		resp *dto.AuthResponse
		// Set instead of returning an error on reuse: returning from the
		// transaction rolls it back, which would undo the very revocation the
		// reuse triggered. The revocation must commit, so the error is
		// reported after.
		reuseDetected bool
	)

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stored models.RefreshToken

		// Unscoped: rotated tokens are soft-deleted and must still be found,
		// otherwise a replay looks identical to an unknown token.
		//
		// Locked for update: two concurrent refreshes with the same token
		// would otherwise both read it as live and both rotate.
		findErr := tx.Unscoped().
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ?", hash).
			First(&stored).Error
		if findErr != nil {
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				return ErrInvalidRefreshToken
			}

			return fmt.Errorf("find refresh token: %w", findErr)
		}

		// Already rotated. A legitimate client never reuses a token, so this
		// is either a stolen token or a replay: drop every session the user
		// has and make them log in again.
		if stored.DeletedAt.Valid {
			s.logger.Warn().
				Uint("user_id", stored.UserID).
				Str("jti", claims.ID).
				Msg("refresh token reuse detected; revoking all sessions")

			if revokeErr := tx.Where("user_id = ?", stored.UserID).
				Delete(&models.RefreshToken{}).Error; revokeErr != nil {
				return fmt.Errorf("revoke sessions: %w", revokeErr)
			}

			reuseDetected = true

			// nil, so the revocation commits.
			return nil
		}

		if stored.Expired(time.Now()) {
			return ErrInvalidRefreshToken
		}

		var user models.User
		if userErr := tx.First(&user, stored.UserID).Error; userErr != nil {
			return ErrInvalidRefreshToken
		}

		// A deactivated user must not be able to refresh their way to a fresh
		// access token.
		if !user.IsActive {
			return ErrInvalidRefreshToken
		}

		if delErr := tx.Delete(&stored).Error; delErr != nil {
			return fmt.Errorf("revoke used token: %w", delErr)
		}

		var issueErr error

		resp, issueErr = s.issueTokens(tx, &user)

		return issueErr
	})
	if err != nil {
		return nil, err
	}

	// Reported after the transaction committed the revocation.
	if reuseDetected {
		return nil, ErrInvalidRefreshToken
	}

	return resp, nil
}

// Logout revokes a refresh token. An unknown token is not an error: the caller
// wanted the session gone, and it is.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)

	if err := s.db.WithContext(ctx).
		Where("token_hash = ?", hash).
		Delete(&models.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}

	return nil
}

// issueTokens mints a pair and records the refresh token's hash. It takes the
// transaction so the record and the caller's work commit together.
func (s *AuthService) issueTokens(tx *gorm.DB, user *models.User) (*dto.AuthResponse, error) {
	pair, err := s.tokens.GenerateTokenPair(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	record := models.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(pair.RefreshToken),
		ExpiresAt: time.Now().Add(s.refreshTTL),
	}

	if err := tx.Create(&record).Error; err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &dto.AuthResponse{
		User:         dto.NewUserResponse(user),
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
	}, nil
}

// hashToken is SHA-256, not bcrypt: the token is already high-entropy, so
// there is nothing to brute force, and refresh must stay fast.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
