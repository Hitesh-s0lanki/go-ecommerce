package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// createCategory godoc
//
//	@Summary		Create a category
//	@Description	Admin only. The category is created active.
//	@Tags			categories
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.CreateCategoryRequest	true	"Category details"
//	@Success		201		{object}	dto.CategoryEnvelope		"Category created"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Validation failed"
//	@Failure		401		{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		403		{object}	dto.ErrorEnvelope			"Not an administrator"
//	@Router			/categories [post]
func (s *Server) createCategory(c *gin.Context) {
	var req dto.CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	category, err := s.categories.Create(c.Request.Context(), &req)
	if err != nil {
		s.respondCategoryError(c, err, "failed to create category")
		return
	}

	utils.CreatedResponse(c, "category created", category)
}

// getCategories godoc
//
//	@Summary		List categories
//	@Description	Public. Returns the active categories, ordered by name.
//	@Tags			categories
//	@Produce		json
//	@Success		200	{object}	dto.CategoryListEnvelope	"Categories"
//	@Failure		500	{object}	dto.ErrorEnvelope			"Internal error"
//	@Router			/categories [get]
func (s *Server) getCategories(c *gin.Context) {
	categories, err := s.categories.List(c.Request.Context())
	if err != nil {
		s.respondCategoryError(c, err, "failed to list categories")
		return
	}

	utils.SuccessResponse(c, "ok", categories)
}

// updateCategory godoc
//
//	@Summary		Update a category
//	@Description	Admin only. Omitting is_active leaves it unchanged.
//	@Tags			categories
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Category ID"
//	@Param			request	body		dto.UpdateCategoryRequest	true	"Category details"
//	@Success		200		{object}	dto.CategoryEnvelope		"Updated category"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Validation failed, or the id is not a number"
//	@Failure		401		{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		403		{object}	dto.ErrorEnvelope			"Not an administrator"
//	@Failure		404		{object}	dto.ErrorEnvelope			"No such category"
//	@Router			/categories/{id} [put]
func (s *Server) updateCategory(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}

	var req dto.UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	category, err := s.categories.Update(c.Request.Context(), id, &req)
	if err != nil {
		s.respondCategoryError(c, err, "failed to update category")
		return
	}

	utils.SuccessResponse(c, "category updated", category)
}

// deleteCategory godoc
//
//	@Summary		Delete a category
//	@Description	Admin only. A category that still has products is refused; deactivate it instead, or move the products first.
//	@Tags			categories
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int					true	"Category ID"
//	@Success		200	{object}	dto.MessageEnvelope	"Category deleted"
//	@Failure		400	{object}	dto.ErrorEnvelope	"The id is not a number"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		403	{object}	dto.ErrorEnvelope	"Not an administrator"
//	@Failure		404	{object}	dto.ErrorEnvelope	"No such category"
//	@Failure		409	{object}	dto.ErrorEnvelope	"The category still has products"
//	@Router			/categories/{id} [delete]
func (s *Server) deleteCategory(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}

	if err := s.categories.Delete(c.Request.Context(), id); err != nil {
		s.respondCategoryError(c, err, "failed to delete category")
		return
	}

	utils.SuccessResponse(c, "category deleted", nil)
}

// respondCategoryError keeps the status mapping in one place: only the
// deliberate service errors reach the caller, and anything else is our bug and
// becomes a 500 with the detail confined to the log.
func (s *Server) respondCategoryError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, services.ErrCategoryNotFound):
		utils.NotFoundResponse(c, "category not found")
	case errors.Is(err, services.ErrCategoryHasProducts):
		// 409, not 400: the request is well-formed, it just conflicts with
		// the current state — and retrying it verbatim will work once the
		// products are moved.
		utils.ErrorResponse(c, http.StatusConflict, "category still has products", err)
	default:
		utils.InternalServerErrorResponse(c, message, err)
	}
}
