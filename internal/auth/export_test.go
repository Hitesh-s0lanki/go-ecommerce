package auth

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ProductionHashCost is the real work factor, so a test can assert it did not
// drift downwards.
const ProductionHashCost = 12

// UseFastHashing drops bcrypt's cost for the duration of a test. The cost
// exists to be slow, which is exactly wrong in a test suite.
func UseFastHashing(t *testing.T) {
	t.Helper()

	original := hashCost
	hashCost = bcrypt.MinCost

	t.Cleanup(func() { hashCost = original })
}

// NewTokenManagerForTest builds a manager with an injectable clock, so expiry
// can be verified from a future instant instead of sleeping through it.
//
// This file is only compiled into the test binary, so the clock stays
// unexported in the real API.
func NewTokenManagerForTest(
	t *testing.T, secret string, accessTTL, refreshTTL time.Duration, now func() time.Time,
) *TokenManager {
	t.Helper()

	return &TokenManager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        now,
	}
}
