package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordLength is bcrypt's hard limit. Input beyond 72 bytes is ignored
// by the algorithm, so "correct-horse-battery-staple...<73rd byte onwards>"
// would authenticate against a hash of only the first 72 bytes. Rejecting the
// input is the only honest option.
const MaxPasswordLength = 72

// MinPasswordLength is the shortest password accepted.
const MinPasswordLength = 8

// hashCost is bcrypt's work factor. 12 rather than bcrypt.DefaultCost (10):
// the default was chosen long ago and each increment doubles the work an
// attacker must do per guess.
//
// A var, not a const, only so tests can lower it — the whole point of the cost
// is to be slow, which would otherwise add ~30s to every test run and every
// pre-push hook. Nothing outside this package can change it.
var hashCost = 12

// Password errors.
var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d bytes", MaxPasswordLength)
	ErrPasswordMismatch = errors.New("password does not match")
)

// HashPassword returns a bcrypt hash of password.
//
// Length is checked explicitly: bcrypt itself would either error opaquely or,
// in older versions, truncate silently.
func HashPassword(password string) (string, error) {
	// Bytes, not runes: bcrypt's limit is on the byte string, and one emoji
	// costs four of them.
	if len(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}

	if len(password) > MaxPasswordLength {
		return "", ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), hashCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return string(hash), nil
}

// CheckPassword reports whether password matches hash.
//
// It returns an error rather than a bool so a corrupt or truncated hash in the
// database is distinguishable from a wrong password — as a bool both look like
// "denied", and the real fault stays invisible.
func CheckPassword(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err == nil {
		return nil
	}

	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}

	return fmt.Errorf("compare password: %w", err)
}
