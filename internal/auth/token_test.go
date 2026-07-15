package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
)

const testSecret = "test-secret-that-is-at-least-32-bytes-long"

func testManager(t *testing.T) *auth.TokenManager {
	t.Helper()

	m, err := auth.NewTokenManager(&config.JWTConfig{
		Secret:              testSecret,
		ExpiresIn:           time.Hour,
		RefreshTokenExpires: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}

	return m
}

// A short secret weakens HS256 and must stop startup, not surface later.
func TestNewTokenManagerRejectsWeakSecret(t *testing.T) {
	_, err := auth.NewTokenManager(&config.JWTConfig{
		Secret:              "too-short",
		ExpiresIn:           time.Hour,
		RefreshTokenExpires: time.Hour,
	})

	if !errors.Is(err, auth.ErrWeakSecret) {
		t.Fatalf("err = %v, want ErrWeakSecret", err)
	}
}

func TestNewTokenManagerRejectsNonPositiveTTL(t *testing.T) {
	_, err := auth.NewTokenManager(&config.JWTConfig{
		Secret:              testSecret,
		ExpiresIn:           0,
		RefreshTokenExpires: time.Hour,
	})

	if err == nil {
		t.Fatal("err = nil, want an error for a zero access TTL")
	}
}

func TestGenerateAndParseRoundTrip(t *testing.T) {
	m := testManager(t)

	pair, err := m.GenerateTokenPair(42, "a@example.com", "admin")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	claims, err := m.ParseAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}

	if claims.UserID != 42 {
		t.Errorf("UserID = %d, want 42", claims.UserID)
	}
	if claims.Email != "a@example.com" {
		t.Errorf("Email = %q, want a@example.com", claims.Email)
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q, want admin", claims.Role)
	}
	if claims.Use != auth.TokenTypeAccess {
		t.Errorf("Use = %q, want access", claims.Use)
	}
	if want := int64(time.Hour.Seconds()); pair.ExpiresIn != want {
		t.Errorf("ExpiresIn = %d, want %d", pair.ExpiresIn, want)
	}
	if pair.RefreshID == "" {
		t.Error("RefreshID is empty, want a jti for revocation")
	}
}

// The core reason token types exist. Both tokens are signed with the same
// secret, so without the check a refresh token — deliberately long-lived —
// would be accepted anywhere an access token is.
func TestRefreshTokenRejectedAsAccessToken(t *testing.T) {
	m := testManager(t)

	pair, err := m.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if _, err := m.ParseAccessToken(pair.RefreshToken); !errors.Is(err, auth.ErrWrongTokenUse) {
		t.Fatalf("ParseAccessToken(refresh) err = %v, want ErrWrongTokenUse", err)
	}
}

func TestAccessTokenRejectedAsRefreshToken(t *testing.T) {
	m := testManager(t)

	pair, err := m.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if _, err := m.ParseRefreshToken(pair.AccessToken); !errors.Is(err, auth.ErrWrongTokenUse) {
		t.Fatalf("ParseRefreshToken(access) err = %v, want ErrWrongTokenUse", err)
	}
}

// The two tokens must not be byte-identical, or the type claim is not actually
// being applied.
func TestAccessAndRefreshTokensDiffer(t *testing.T) {
	m := testManager(t)

	pair, err := m.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if pair.AccessToken == pair.RefreshToken {
		t.Fatal("access and refresh tokens are identical")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	m := testManager(t)

	pair, err := m.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// Verify from a point past the token's expiry rather than sleeping.
	future := auth.NewTokenManagerForTest(t, testSecret, time.Hour, 24*time.Hour,
		func() time.Time { return time.Now().Add(2 * time.Hour) })

	if _, err := future.ParseAccessToken(pair.AccessToken); !errors.Is(err, auth.ErrExpiredToken) {
		t.Fatalf("err = %v, want ErrExpiredToken", err)
	}
}

func TestTokenSignedWithOtherSecretRejected(t *testing.T) {
	m := testManager(t)

	other, err := auth.NewTokenManager(&config.JWTConfig{
		Secret:              "a-completely-different-secret-32-bytes",
		ExpiresIn:           time.Hour,
		RefreshTokenExpires: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}

	pair, err := other.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	if _, err := m.ParseAccessToken(pair.AccessToken); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	m := testManager(t)

	pair, err := m.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	// Flip a character in the payload; the signature no longer matches.
	parts := strings.Split(pair.AccessToken, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	payload := []byte(parts[1])
	if payload[0] == 'a' {
		payload[0] = 'b'
	} else {
		payload[0] = 'a'
	}

	tampered := parts[0] + "." + string(payload) + "." + parts[2]

	if _, err := m.ParseAccessToken(tampered); err == nil {
		t.Fatal("tampered token accepted, want an error")
	}
}

// The classic JWT attack: a token whose header says alg=none, which a parser
// that trusts the header will accept unsigned.
func TestAlgNoneTokenRejected(t *testing.T) {
	m := testManager(t)

	claims := &auth.Claims{
		UserID: 1, Email: "attacker@example.com", Role: "admin", Use: auth.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}

	if _, err := m.ParseAccessToken(unsigned); err == nil {
		t.Fatal("alg=none token accepted — algorithm is not pinned")
	}
}

// A token with no exp would otherwise be valid forever.
func TestTokenWithoutExpiryRejected(t *testing.T) {
	m := testManager(t)

	claims := &auth.Claims{
		UserID: 1, Email: "a@example.com", Role: "customer", Use: auth.TokenTypeAccess,
	}

	eternal, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := m.ParseAccessToken(eternal); err == nil {
		t.Fatal("token without exp accepted, want an error")
	}
}

// A token with no use claim predates the type check; it must not be honoured.
func TestTokenWithoutUseClaimRejected(t *testing.T) {
	m := testManager(t)

	claims := jwt.MapClaims{
		"user_id": 1,
		"email":   "a@example.com",
		"role":    "admin",
		"exp":     time.Now().Add(time.Hour).Unix(),
	}

	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := m.ParseAccessToken(legacy); !errors.Is(err, auth.ErrWrongTokenUse) {
		t.Fatalf("err = %v, want ErrWrongTokenUse", err)
	}
}

func TestGarbageTokenRejected(t *testing.T) {
	m := testManager(t)

	for _, token := range []string{"", "garbage", "a.b.c", "Bearer x"} {
		if _, err := m.ParseAccessToken(token); err == nil {
			t.Errorf("token %q accepted, want an error", token)
		}
	}
}

// Each issued token must be individually identifiable for revocation.
func TestTokenIDsAreUnique(t *testing.T) {
	m := testManager(t)

	seen := make(map[string]bool)

	for range 5 {
		pair, err := m.GenerateTokenPair(1, "a@example.com", "customer")
		if err != nil {
			t.Fatalf("GenerateTokenPair: %v", err)
		}

		if seen[pair.RefreshID] {
			t.Fatalf("duplicate refresh jti %q", pair.RefreshID)
		}

		seen[pair.RefreshID] = true
	}
}
