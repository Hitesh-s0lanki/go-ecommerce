package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// register godoc
//
//	@Summary		Register a new account
//	@Description	Creates a customer account and returns an initial token pair. The role is always "customer"; it cannot be set by the caller.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.RegisterRequest	true	"Registration details"
//	@Success		201		{object}	dto.AuthEnvelope	"Account created"
//	@Failure		400		{object}	dto.ErrorEnvelope	"Validation failed, or the email is unavailable"
//	@Failure		500		{object}	dto.ErrorEnvelope	"Internal error"
//	@Router			/auth/register [post]
func (s *Server) register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	resp, err := s.auth.Register(c.Request.Context(), &req)
	if err != nil {
		// Only the vague, deliberate errors reach the client; anything else
		// is ours and becomes a 500 with the detail in the log.
		if errors.Is(err, services.ErrEmailTaken) {
			utils.BadRequestResponse(c, "registration failed", err)
			return
		}

		utils.InternalServerErrorResponse(c, "registration failed", err)

		return
	}

	utils.CreatedResponse(c, "account created", resp)
}

// login godoc
//
//	@Summary		Log in
//	@Description	Exchanges email and password for an access/refresh token pair.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.LoginRequest	true	"Credentials"
//	@Success		200		{object}	dto.AuthEnvelope	"Logged in"
//	@Failure		400		{object}	dto.ErrorEnvelope	"Validation failed"
//	@Failure		401		{object}	dto.ErrorEnvelope	"Invalid credentials"
//	@Router			/auth/login [post]
func (s *Server) login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	resp, err := s.auth.Login(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			utils.UnauthorizedResponse(c, "invalid credentials")
			return
		}

		utils.InternalServerErrorResponse(c, "login failed", err)

		return
	}

	utils.SuccessResponse(c, "logged in", resp)
}

// refresh godoc
//
//	@Summary		Refresh tokens
//	@Description	Exchanges a refresh token for a new pair. The presented token is revoked, so each refresh token works once. Replaying a used token revokes every session for that user.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.RefreshTokenRequest	true	"Refresh token"
//	@Success		200		{object}	dto.AuthEnvelope		"New token pair"
//	@Failure		400		{object}	dto.ErrorEnvelope		"Validation failed"
//	@Failure		401		{object}	dto.ErrorEnvelope		"Invalid, expired, or already-used refresh token"
//	@Router			/auth/refresh [post]
func (s *Server) refresh(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	resp, err := s.auth.Refresh(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidRefreshToken) {
			utils.UnauthorizedResponse(c, "invalid refresh token")
			return
		}

		utils.InternalServerErrorResponse(c, "token refresh failed", err)

		return
	}

	utils.SuccessResponse(c, "tokens refreshed", resp)
}

// logout godoc
//
//	@Summary		Log out
//	@Description	Revokes a refresh token. Succeeds even if the token is already gone, since the caller's intent is satisfied either way.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.RefreshTokenRequest	true	"Refresh token to revoke"
//	@Success		200		{object}	dto.MessageEnvelope		"Logged out"
//	@Failure		400		{object}	dto.ErrorEnvelope		"Validation failed"
//	@Failure		500		{object}	dto.ErrorEnvelope		"Internal error"
//	@Router			/auth/logout [post]
func (s *Server) logout(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	if err := s.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		utils.InternalServerErrorResponse(c, "logout failed", err)
		return
	}

	utils.SuccessResponse(c, "logged out", nil)
}

// me godoc
//
//	@Summary		Current account
//	@Description	Returns the authenticated user.
//	@Tags			auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.UserEnvelope	"The current user"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Router			/auth/me [get]
func (s *Server) me(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	user, err := s.auth.CurrentUser(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			// The token is valid but the account is gone or disabled.
			utils.ErrorResponse(c, http.StatusUnauthorized, "account unavailable", nil)
			return
		}

		utils.InternalServerErrorResponse(c, "failed to load account", err)

		return
	}

	utils.SuccessResponse(c, "ok", user)
}
