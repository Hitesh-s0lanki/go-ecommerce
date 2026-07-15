package server

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// getProfile godoc
//
//	@Summary		Get your profile
//	@Description	Returns the authenticated user. Identical to /auth/me.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.UserEnvelope	"The current user"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		404	{object}	dto.ErrorEnvelope	"Account no longer exists or is deactivated"
//	@Router			/users/profile [get]
func (s *Server) getProfile(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	profile, err := s.users.GetProfile(c.Request.Context(), id)
	if err != nil {
		s.respondUserError(c, err, "failed to load profile")
		return
	}

	utils.SuccessResponse(c, "ok", profile)
}

// updateProfile godoc
//
//	@Summary		Update your profile
//	@Description	Replaces your name and phone. Role, email, and status are not editable here.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.UpdateProfileRequest	true	"Profile details"
//	@Success		200		{object}	dto.UserEnvelope			"Updated user"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Validation failed"
//	@Failure		401		{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		404		{object}	dto.ErrorEnvelope			"Account no longer exists or is deactivated"
//	@Router			/users/profile [put]
func (s *Server) updateProfile(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	profile, err := s.users.UpdateProfile(c.Request.Context(), id, &req)
	if err != nil {
		s.respondUserError(c, err, "failed to update profile")
		return
	}

	utils.SuccessResponse(c, "profile updated", profile)
}

// respondUserError keeps the 404-vs-500 split in one place: a missing account
// is the caller's answer, anything else is our bug and must not reach them.
func (s *Server) respondUserError(c *gin.Context, err error, message string) {
	if errors.Is(err, services.ErrUserNotFound) {
		utils.NotFoundResponse(c, "account not found")
		return
	}

	utils.InternalServerErrorResponse(c, message, err)
}
