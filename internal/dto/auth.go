// Package dto holds the request and response shapes of the HTTP API.
//
// These are deliberately separate from the models: a model describes what is
// stored, a DTO describes what crosses the wire. Binding requests straight
// onto models invites mass assignment (a caller setting Role or IsActive), and
// serialising models straight back leaks columns as they are added.
package dto

import "github.com/Hitesh-s0lanki/go-ecommerce/internal/models"

// RegisterRequest is the body of POST /auth/register.
//
// Note the absence of Role: it is assigned by the server, not the caller.
type RegisterRequest struct {
	Email     string `json:"email"      binding:"required,email"`
	Password  string `json:"password"   binding:"required,min=8,max=72"`
	FirstName string `json:"first_name" binding:"required,max=100"`
	LastName  string `json:"last_name"  binding:"required,max=100"`
	Phone     string `json:"phone"      binding:"omitempty,e164"`
}

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest is the body of POST /auth/refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UpdateProfileRequest is the body of PUT /users/me.
type UpdateProfileRequest struct {
	FirstName string `json:"first_name" binding:"required,max=100"`
	LastName  string `json:"last_name"  binding:"required,max=100"`
	Phone     string `json:"phone"      binding:"omitempty,e164"`
}

// AuthResponse is returned by register, login, and refresh.
type AuthResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	// ExpiresIn is the access token's lifetime in seconds, so a client need
	// not decode the token to schedule a refresh.
	ExpiresIn int64 `json:"expires_in"`
}

// UserResponse is a user as returned to clients. It has no password field by
// construction, rather than relying on a json:"-" tag on the model.
type UserResponse struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone,omitempty"`
	Role      string `json:"role"`
	IsActive  bool   `json:"is_active"`
}

// NewUserResponse maps a model to its wire shape.
func NewUserResponse(u *models.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
		Role:      string(u.Role),
		IsActive:  u.IsActive,
	}
}
