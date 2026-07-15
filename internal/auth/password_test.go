package auth_test

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
)

func TestHashAndCheckPassword(t *testing.T) {
	auth.UseFastHashing(t)

	const password = "correct-horse-battery-staple"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if hash == password {
		t.Fatal("hash equals the password — it is not hashed")
	}

	if err := auth.CheckPassword(hash, password); err != nil {
		t.Errorf("CheckPassword with the right password: %v", err)
	}
}

func TestCheckPasswordRejectsWrongPassword(t *testing.T) {
	auth.UseFastHashing(t)

	hash, err := auth.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	err = auth.CheckPassword(hash, "wrong-password")
	if !errors.Is(err, auth.ErrPasswordMismatch) {
		t.Errorf("err = %v, want ErrPasswordMismatch", err)
	}
}

// The same password must produce different hashes, or the salt is not doing
// its job and identical passwords would be visible as identical hashes.
func TestHashesAreSalted(t *testing.T) {
	auth.UseFastHashing(t)

	const password = "correct-horse-battery-staple"

	first, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	second, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if first == second {
		t.Fatal("two hashes of the same password are identical — no salt")
	}

	// Both must still verify.
	if err := auth.CheckPassword(first, password); err != nil {
		t.Errorf("first hash: %v", err)
	}
	if err := auth.CheckPassword(second, password); err != nil {
		t.Errorf("second hash: %v", err)
	}
}

// bcrypt ignores input past 72 bytes. Accepting a longer password would mean
// only its first 72 bytes are ever checked — so it is rejected instead.
func TestPasswordLengthLimits(t *testing.T) {
	auth.UseFastHashing(t)

	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{"too short", "short", auth.ErrPasswordTooShort},
		{"minimum length", strings.Repeat("a", auth.MinPasswordLength), nil},
		{"maximum length", strings.Repeat("a", auth.MaxPasswordLength), nil},
		{"one byte too long", strings.Repeat("a", auth.MaxPasswordLength+1), auth.ErrPasswordTooLong},
		// 4 bytes each: 20 emoji are 80 bytes, well past the limit despite
		// being only 20 characters.
		{"multibyte over the byte limit", strings.Repeat("😀", 20), auth.ErrPasswordTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.HashPassword(tt.password)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("HashPassword: %v", err)
				}

				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// Proof of why the limit is enforced: bcrypt itself cannot tell a 72-byte
// password from that password with anything appended.
func TestBcryptTruncationIsReal(t *testing.T) {
	base := strings.Repeat("a", auth.MaxPasswordLength)

	hash, err := bcrypt.GenerateFromPassword([]byte(base), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}

	// Appending to a 72-byte password changes nothing bcrypt can see.
	err = bcrypt.CompareHashAndPassword(hash, []byte(base+"COMPLETELY-DIFFERENT-SUFFIX"))
	if err != nil {
		t.Skipf("this bcrypt build rejects long input rather than truncating: %v", err)
	}

	// Reaching here means bcrypt authenticated a different password against
	// this hash, because it only ever saw the first 72 bytes. That is exactly
	// what HashPassword's length check exists to prevent.
	if _, err := auth.HashPassword(base + "COMPLETELY-DIFFERENT-SUFFIX"); !errors.Is(err, auth.ErrPasswordTooLong) {
		t.Errorf("HashPassword accepted an over-length password: %v", err)
	}
}

// A corrupt hash must be distinguishable from a wrong password: as a bool both
// look like "denied" and the real fault stays invisible.
func TestCheckPasswordDistinguishesCorruptHash(t *testing.T) {
	err := auth.CheckPassword("not-a-bcrypt-hash", "any-password")

	if err == nil {
		t.Fatal("err = nil, want an error for a corrupt hash")
	}
	if errors.Is(err, auth.ErrPasswordMismatch) {
		t.Error("corrupt hash reported as a password mismatch, want a distinct error")
	}
}

// The production cost must not drift down: tests lower it, and a mistake there
// would otherwise go unnoticed.
func TestProductionHashCost(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}

	if cost != auth.ProductionHashCost {
		t.Errorf("bcrypt cost = %d, want %d", cost, auth.ProductionHashCost)
	}
}
