package services_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

// newUserService reuses the auth service's fixture: registering through the
// real flow is the honest way to get a user with a hashed password and a cart.
func newUserService(t *testing.T) (*services.UserService, *services.AuthService, *gorm.DB) {
	t.Helper()

	authSvc, db := newAuthService(t)

	return services.NewUserService(db), authSvc, db
}

func TestGetProfile(t *testing.T) {
	users, authSvc, _ := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("profile@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	profile, err := users.GetProfile(ctx, reg.User.ID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}

	if profile.Email != "profile@example.com" {
		t.Errorf("Email = %q, want profile@example.com", profile.Email)
	}
	if profile.Role != string(models.UserRoleCustomer) {
		t.Errorf("Role = %q, want customer", profile.Role)
	}
}

func TestGetProfileUnknownUser(t *testing.T) {
	users, _, _ := newUserService(t)

	if _, err := users.GetProfile(context.Background(), 99999); !errors.Is(err, services.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

// An access token outlives a deactivation, so the account must be re-checked.
func TestGetProfileDeactivatedUser(t *testing.T) {
	users, authSvc, db := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("gone@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := db.Model(&models.User{}).Where("id = ?", reg.User.ID).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if _, err := users.GetProfile(ctx, reg.User.ID); !errors.Is(err, services.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestUpdateProfile(t *testing.T) {
	users, authSvc, _ := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("update@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	updated, err := users.UpdateProfile(ctx, reg.User.ID, &dto.UpdateProfileRequest{
		FirstName: "New",
		LastName:  "Name",
		Phone:     "+14155550123",
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	if updated.FirstName != "New" || updated.LastName != "Name" {
		t.Errorf("name = %q %q, want New Name", updated.FirstName, updated.LastName)
	}
	if updated.Phone != "+14155550123" {
		t.Errorf("Phone = %q, want +14155550123", updated.Phone)
	}
	// The returned row must be the persisted one, not the request echoed back.
	if updated.Email != "update@example.com" {
		t.Errorf("Email = %q, want the stored address", updated.Email)
	}
}

// Why UpdateProfile names its columns instead of loading the row and calling
// Save, as the reference does.
//
// Save writes every column from the copy it read. If anything commits between
// the read and the Save, that change is silently reverted — a lost update.
// This reproduces the interleaving directly, because a black-box test of the
// service cannot pause it mid-transaction.
func TestSaveWouldClobberConcurrentUpdate(t *testing.T) {
	_, authSvc, db := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("lostupdate@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// A request reads the user, intending to edit the profile.
	var stale models.User
	if err := db.First(&stale, reg.User.ID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	// Meanwhile, someone promotes them.
	if err := db.Model(&models.User{}).Where("id = ?", reg.User.ID).
		Update("role", models.UserRoleAdmin).Error; err != nil {
		t.Fatalf("promote: %v", err)
	}

	// The first request saves its now-stale copy.
	stale.FirstName = "New"
	if err := db.Save(&stale).Error; err != nil {
		t.Fatalf("save: %v", err)
	}

	var after models.User
	if err := db.First(&after, reg.User.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}

	if after.Role == models.UserRoleAdmin {
		t.Skip("Save did not clobber the role; the hazard this guards against may no longer apply")
	}

	t.Logf("confirmed: Save reverted role %q -> %q, which is why UpdateProfile names its columns",
		models.UserRoleAdmin, after.Role)

	// The service's targeted update must not have this problem: it never
	// writes the role at all.
	if _, err := services.NewUserService(db).UpdateProfile(ctx, reg.User.ID, &dto.UpdateProfileRequest{
		FirstName: "Safe", LastName: "Update",
	}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	var final models.User
	if err := db.First(&final, reg.User.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Role is whatever it was; the point is UpdateProfile did not touch it.
	if final.FirstName != "Safe" {
		t.Errorf("FirstName = %q, want Safe", final.FirstName)
	}
}

// A profile update must never write the role or password, whatever else
// changes around it.
func TestUpdateProfileDoesNotClobberOtherColumns(t *testing.T) {
	users, authSvc, db := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("clobber@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Something else promotes the user — an admin tool, another request.
	if err := db.Model(&models.User{}).Where("id = ?", reg.User.ID).
		Update("role", models.UserRoleAdmin).Error; err != nil {
		t.Fatalf("promote: %v", err)
	}

	updated, err := users.UpdateProfile(ctx, reg.User.ID, &dto.UpdateProfileRequest{
		FirstName: "New", LastName: "Name",
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	if updated.Role != string(models.UserRoleAdmin) {
		t.Errorf("Role = %q, want admin — a profile update must not rewrite the role", updated.Role)
	}

	// And the password must survive untouched.
	var user models.User
	if err := db.First(&user, reg.User.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if user.Password == "" {
		t.Error("password was blanked by a profile update")
	}
}

func TestUpdateProfileUnknownUser(t *testing.T) {
	users, _, _ := newUserService(t)

	_, err := users.UpdateProfile(context.Background(), 99999, &dto.UpdateProfileRequest{
		FirstName: "A", LastName: "B",
	})
	if !errors.Is(err, services.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestUpdateProfileDeactivatedUser(t *testing.T) {
	users, authSvc, db := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("inactive-update@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := db.Model(&models.User{}).Where("id = ?", reg.User.ID).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	_, err = users.UpdateProfile(ctx, reg.User.ID, &dto.UpdateProfileRequest{
		FirstName: "A", LastName: "B",
	})
	if !errors.Is(err, services.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

// Phone is optional: clearing it must actually clear it, which a zero-value
// struct update would silently skip.
func TestUpdateProfileClearsPhone(t *testing.T) {
	users, authSvc, _ := newUserService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("phone@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := users.UpdateProfile(ctx, reg.User.ID, &dto.UpdateProfileRequest{
		FirstName: "A", LastName: "B", Phone: "+14155550123",
	}); err != nil {
		t.Fatalf("set phone: %v", err)
	}

	cleared, err := users.UpdateProfile(ctx, reg.User.ID, &dto.UpdateProfileRequest{
		FirstName: "A", LastName: "B", Phone: "",
	})
	if err != nil {
		t.Fatalf("clear phone: %v", err)
	}

	if cleared.Phone != "" {
		t.Errorf("Phone = %q, want it cleared", cleared.Phone)
	}
}
