package services_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

const testSecret = "test-secret-that-is-at-least-32-bytes-long"

// The auth service is mostly transactions, unique constraints, and soft-delete
// semantics — the parts a fake database reproduces least faithfully. So these
// run against real Postgres. Use `make test-integration`; they skip otherwise.
func newAuthService(t *testing.T) (*services.AuthService, *gorm.DB) {
	t.Helper()

	svc, db, _ := newAuthServiceWithEvents(t)

	return svc, db
}

// newAuthServiceWithEvents also hands back the publisher, so a test can assert
// on what was announced.
func newAuthServiceWithEvents(t *testing.T) (*services.AuthService, *gorm.DB, *fakePublisher) {
	t.Helper()

	db := newSchemaDB(t)

	tokens, err := auth.NewTokenManager(&config.JWTConfig{
		Secret:              testSecret,
		ExpiresIn:           time.Hour,
		RefreshTokenExpires: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}

	log := zerolog.New(io.Discard)
	publisher := &fakePublisher{}

	return services.NewAuthService(db, tokens, 24*time.Hour, &log, publisher), db, publisher
}

func registerReq(email string) *dto.RegisterRequest {
	return &dto.RegisterRequest{
		Email:     email,
		Password:  "correct-horse-battery",
		FirstName: "A",
		LastName:  "B",
	}
}

func TestRegister(t *testing.T) {
	svc, db := newAuthService(t)
	ctx := context.Background()

	resp, err := svc.Register(ctx, registerReq("new@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("Register returned an empty token")
	}
	// The role must never come from the request.
	if resp.User.Role != string(models.UserRoleCustomer) {
		t.Errorf("Role = %q, want customer", resp.User.Role)
	}

	// A user without a cart is a broken account.
	var carts int64
	if err := db.Model(&models.Cart{}).Where("user_id = ?", resp.User.ID).Count(&carts).Error; err != nil {
		t.Fatalf("count carts: %v", err)
	}
	if carts != 1 {
		t.Errorf("got %d carts, want 1", carts)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc, _ := newAuthService(t)
	ctx := context.Background()

	if _, err := svc.Register(ctx, registerReq("dup@example.com")); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	_, err := svc.Register(ctx, registerReq("dup@example.com"))
	if !errors.Is(err, services.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

// The password must never be stored in a readable form.
func TestRegisterHashesPassword(t *testing.T) {
	svc, db := newAuthService(t)
	ctx := context.Background()

	resp, err := svc.Register(ctx, registerReq("hash@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	var user models.User
	if err := db.First(&user, resp.User.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}

	if user.Password == "correct-horse-battery" {
		t.Fatal("password stored in plaintext")
	}
	if err := auth.CheckPassword(user.Password, "correct-horse-battery"); err != nil {
		t.Errorf("stored hash does not verify: %v", err)
	}
}

// A database leak must not yield working tokens.
func TestRefreshTokenStoredAsHashNotToken(t *testing.T) {
	svc, db := newAuthService(t)
	ctx := context.Background()

	resp, err := svc.Register(ctx, registerReq("stored@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	var stored models.RefreshToken
	if err := db.Where("user_id = ?", resp.User.ID).First(&stored).Error; err != nil {
		t.Fatalf("load refresh token: %v", err)
	}

	if stored.TokenHash == resp.RefreshToken {
		t.Fatal("refresh token stored verbatim — a database leak would hand out working tokens")
	}
	if len(stored.TokenHash) != 64 {
		t.Errorf("TokenHash length = %d, want 64 (hex sha256)", len(stored.TokenHash))
	}
}

func TestLogin(t *testing.T) {
	svc, _ := newAuthService(t)
	ctx := context.Background()

	if _, err := svc.Register(ctx, registerReq("login@example.com")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resp, err := svc.Login(ctx, &dto.LoginRequest{Email: "login@example.com", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("empty access token")
	}
}

func TestLoginFailures(t *testing.T) {
	svc, db := newAuthService(t)
	ctx := context.Background()

	resp, err := svc.Register(ctx, registerReq("fail@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Deactivate a second account to cover the inactive path.
	inactive, err := svc.Register(ctx, registerReq("inactive@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", inactive.User.ID).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{"wrong password", "fail@example.com", "not-the-password"},
		{"unknown email", "nobody@example.com", "correct-horse-battery"},
		{"deactivated account", "inactive@example.com", "correct-horse-battery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Every failure must be the same error: anything else lets a
			// caller tell which accounts exist.
			_, err := svc.Login(ctx, &dto.LoginRequest{Email: tt.email, Password: tt.password})
			if !errors.Is(err, services.ErrInvalidCredentials) {
				t.Errorf("err = %v, want ErrInvalidCredentials", err)
			}
		})
	}

	_ = resp
}

func TestRefreshRotates(t *testing.T) {
	svc, _ := newAuthService(t)
	ctx := context.Background()

	reg, err := svc.Register(ctx, registerReq("rotate@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	refreshed, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if refreshed.RefreshToken == reg.RefreshToken {
		t.Error("refresh returned the same token — it was not rotated")
	}
}

// Replaying a rotated token means it leaked. Every session must die, and the
// revocation must survive: it happens inside the same transaction that reports
// the failure, so a naive implementation rolls it straight back.
func TestRefreshReuseRevokesAllSessions(t *testing.T) {
	svc, db := newAuthService(t)
	ctx := context.Background()

	reg, err := svc.Register(ctx, registerReq("reuse@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	rotated, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Replay the token that was just rotated away.
	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.RefreshToken}); !errors.Is(err, services.ErrInvalidRefreshToken) {
		t.Fatalf("replay err = %v, want ErrInvalidRefreshToken", err)
	}

	// The token issued by the legitimate rotation must now be dead too.
	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: rotated.RefreshToken}); !errors.Is(err, services.ErrInvalidRefreshToken) {
		t.Fatalf("post-reuse token err = %v, want ErrInvalidRefreshToken — all sessions must be revoked", err)
	}

	var live int64
	if err := db.Model(&models.RefreshToken{}).Where("user_id = ?", reg.User.ID).Count(&live).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 0 {
		t.Errorf("%d live sessions remain, want 0", live)
	}
}

func TestRefreshRejectsAccessToken(t *testing.T) {
	svc, _ := newAuthService(t)
	ctx := context.Background()

	reg, err := svc.Register(ctx, registerReq("wrongtype@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// An access token is signed with the same secret; only the use claim says
	// otherwise.
	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.AccessToken}); !errors.Is(err, services.ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	svc, _ := newAuthService(t)
	ctx := context.Background()

	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: "not-a-token"}); !errors.Is(err, services.ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
}

// A deactivated user must not refresh their way to a fresh access token.
func TestRefreshRejectsDeactivatedUser(t *testing.T) {
	svc, db := newAuthService(t)
	ctx := context.Background()

	reg, err := svc.Register(ctx, registerReq("deactivated@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := db.Model(&models.User{}).Where("id = ?", reg.User.ID).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.RefreshToken}); !errors.Is(err, services.ErrInvalidRefreshToken) {
		t.Fatalf("err = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestLogout(t *testing.T) {
	svc, _ := newAuthService(t)
	ctx := context.Background()

	reg, err := svc.Register(ctx, registerReq("logout@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.Logout(ctx, reg.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// The revoked token must not refresh.
	if _, err := svc.Refresh(ctx, &dto.RefreshTokenRequest{RefreshToken: reg.RefreshToken}); err == nil {
		t.Error("a logged-out refresh token still works")
	}
}
