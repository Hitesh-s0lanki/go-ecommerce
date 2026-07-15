// Package auth holds the security primitives: password hashing and JWT
// issuing/verification.
//
// These live apart from internal/utils on purpose. utils holds the HTTP
// response envelope; mixing presentation helpers with the code that decides
// who a caller is makes the security surface hard to find and hard to review.
package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
)

// TokenType distinguishes an access token from a refresh token.
//
// Without it the two are interchangeable: both carry the same claims and are
// signed with the same secret, so a refresh token — deliberately long-lived —
// would be accepted anywhere an access token is.
type TokenType string

// The token types.
const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// signingMethod is the only algorithm this service issues or accepts.
var signingMethod = jwt.SigningMethodHS256

// minSecretLen is 256 bits, matching HS256's output. A shorter key weakens the
// HMAC.
const minSecretLen = 32

// Errors returned when a token is not usable. They are deliberately coarse:
// callers should tell the client "invalid token", not which check failed.
var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrExpiredToken  = errors.New("token expired")
	ErrWrongTokenUse = errors.New("token used for the wrong purpose")
	ErrWeakSecret    = fmt.Errorf("jwt secret must be at least %d bytes", minSecretLen)
)

// Claims is the JWT payload.
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	// Use is "access" or "refresh". Checked on every parse.
	Use TokenType `json:"use"`

	jwt.RegisteredClaims
}

// TokenPair is a freshly issued access/refresh pair.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn is the access token's lifetime in seconds.
	ExpiresIn int64
	// RefreshID is the refresh token's jti, so callers can record or revoke
	// the specific token rather than every session a user has.
	RefreshID string
}

// TokenManager issues and verifies tokens.
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	// now is injectable so expiry can be tested without sleeping.
	now func() time.Time
}

// NewTokenManager validates the JWT configuration and returns a manager.
//
// The secret is checked here rather than at first use: a weak secret should
// stop the process at startup, not surface as a forged token later.
func NewTokenManager(cfg *config.JWTConfig) (*TokenManager, error) {
	if len(cfg.Secret) < minSecretLen {
		return nil, ErrWeakSecret
	}

	if cfg.ExpiresIn <= 0 || cfg.RefreshTokenExpires <= 0 {
		return nil, errors.New("jwt token lifetimes must be positive")
	}

	return &TokenManager{
		secret:     []byte(cfg.Secret),
		accessTTL:  cfg.ExpiresIn,
		refreshTTL: cfg.RefreshTokenExpires,
		now:        time.Now,
	}, nil
}

// GenerateTokenPair issues an access and a refresh token for a user.
func (m *TokenManager) GenerateTokenPair(userID uint, email, role string) (TokenPair, error) {
	// One timestamp for the pair, so iat matches and expiries are relative to
	// the same instant.
	issuedAt := m.now()

	accessToken, _, err := m.sign(userID, email, role, TokenTypeAccess, issuedAt, m.accessTTL)
	if err != nil {
		return TokenPair{}, fmt.Errorf("sign access token: %w", err)
	}

	refreshToken, refreshID, err := m.sign(userID, email, role, TokenTypeRefresh, issuedAt, m.refreshTTL)
	if err != nil {
		return TokenPair{}, fmt.Errorf("sign refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(m.accessTTL.Seconds()),
		RefreshID:    refreshID,
	}, nil
}

func (m *TokenManager) sign(
	userID uint, email, role string, use TokenType, issuedAt time.Time, ttl time.Duration,
) (token, id string, err error) {
	id = uuid.NewString()

	claims := &Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		Use:    use,
		RegisteredClaims: jwt.RegisteredClaims{
			// jti lets a single token be revoked without invalidating every
			// session the user has.
			ID:        id,
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(ttl)),
		},
	}

	signed, err := jwt.NewWithClaims(signingMethod, claims).SignedString(m.secret)
	if err != nil {
		return "", "", err
	}

	return signed, id, nil
}

// ParseAccessToken verifies a token and requires it to be an access token.
func (m *TokenManager) ParseAccessToken(token string) (*Claims, error) {
	return m.parse(token, TokenTypeAccess)
}

// ParseRefreshToken verifies a token and requires it to be a refresh token.
func (m *TokenManager) ParseRefreshToken(token string) (*Claims, error) {
	return m.parse(token, TokenTypeRefresh)
}

func (m *TokenManager) parse(token string, want TokenType) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		// Pin the algorithm. Without this the parser accepts whatever the
		// token's header claims, which is the root of the classic JWT
		// algorithm-confusion attacks.
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		// A token with no exp would otherwise be valid forever.
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}

		// The underlying reason is deliberately dropped: it is not the
		// caller's business why their token failed.
		return nil, ErrInvalidToken
	}

	// The signature only proves the token is ours, not that it is the right
	// kind. A refresh token is signed with the same secret.
	if claims.Use != want {
		return nil, ErrWrongTokenUse
	}

	return claims, nil
}
