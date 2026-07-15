package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// createProduct godoc
//
//	@Summary		Create a product
//	@Description	Admin only. The product is created active. Prices are in minor units: 1999 means $19.99.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.CreateProductRequest	true	"Product details"
//	@Success		201		{object}	dto.ProductEnvelope			"Product created"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Validation failed, or the category does not exist"
//	@Failure		401		{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		403		{object}	dto.ErrorEnvelope			"Not an administrator"
//	@Failure		409		{object}	dto.ErrorEnvelope			"The SKU is already in use"
//	@Router			/products [post]
func (s *Server) createProduct(c *gin.Context) {
	var req dto.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	product, err := s.products.Create(c.Request.Context(), &req)
	if err != nil {
		s.respondProductError(c, err, "failed to create product")
		return
	}

	utils.CreatedResponse(c, "product created", product)
}

// getProducts godoc
//
//	@Summary		List products
//	@Description	Public. Returns a page of active products, newest first, with their category and images.
//	@Tags			products
//	@Produce		json
//	@Param			page	query		int							false	"Page number, from 1"	default(1)
//	@Param			limit	query		int							false	"Page size, up to 100"	default(20)
//	@Success		200		{object}	dto.ProductListEnvelope		"A page of products"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Invalid page or limit"
//	@Failure		500		{object}	dto.ErrorEnvelope			"Internal error"
//	@Router			/products [get]
func (s *Server) getProducts(c *gin.Context) {
	var query dto.ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		utils.BadRequestResponse(c, "invalid pagination", err)
		return
	}

	products, meta, err := s.products.List(c.Request.Context(), query)
	if err != nil {
		s.respondProductError(c, err, "failed to list products")
		return
	}

	utils.PaginatedSuccessResponse(c, "ok", products, utils.PaginationMeta{
		Page:       meta.Page,
		Limit:      meta.Limit,
		Total:      meta.Total,
		TotalPages: meta.TotalPages,
	})
}

// getProduct godoc
//
//	@Summary		Get a product
//	@Description	Public. An inactive product is reported as not found.
//	@Tags			products
//	@Produce		json
//	@Param			id	path		int					true	"Product ID"
//	@Success		200	{object}	dto.ProductEnvelope	"The product"
//	@Failure		400	{object}	dto.ErrorEnvelope	"The id is not a number"
//	@Failure		404	{object}	dto.ErrorEnvelope	"No such product, or it is not on sale"
//	@Router			/products/{id} [get]
func (s *Server) getProduct(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}

	product, err := s.products.Get(c.Request.Context(), id)
	if err != nil {
		s.respondProductError(c, err, "failed to load product")
		return
	}

	utils.SuccessResponse(c, "ok", product)
}

// updateProduct godoc
//
//	@Summary		Update a product
//	@Description	Admin only. The SKU cannot be changed. Omitting is_active leaves it unchanged.
//	@Tags			products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Product ID"
//	@Param			request	body		dto.UpdateProductRequest	true	"Product details"
//	@Success		200		{object}	dto.ProductEnvelope			"Updated product"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Validation failed, or the category does not exist"
//	@Failure		401		{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		403		{object}	dto.ErrorEnvelope			"Not an administrator"
//	@Failure		404		{object}	dto.ErrorEnvelope			"No such product"
//	@Router			/products/{id} [put]
func (s *Server) updateProduct(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}

	var req dto.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	product, err := s.products.Update(c.Request.Context(), id, &req)
	if err != nil {
		s.respondProductError(c, err, "failed to update product")
		return
	}

	utils.SuccessResponse(c, "product updated", product)
}

// deleteProduct godoc
//
//	@Summary		Delete a product
//	@Description	Admin only. The product is soft-deleted and its SKU becomes available again.
//	@Tags			products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int					true	"Product ID"
//	@Success		200	{object}	dto.MessageEnvelope	"Product deleted"
//	@Failure		400	{object}	dto.ErrorEnvelope	"The id is not a number"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		403	{object}	dto.ErrorEnvelope	"Not an administrator"
//	@Failure		404	{object}	dto.ErrorEnvelope	"No such product"
//	@Router			/products/{id} [delete]
func (s *Server) deleteProduct(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}

	if err := s.products.Delete(c.Request.Context(), id); err != nil {
		s.respondProductError(c, err, "failed to delete product")
		return
	}

	utils.SuccessResponse(c, "product deleted", nil)
}

// multipartOverhead is the slack allowed above the file limit for the
// multipart envelope — boundaries, part headers, the alt_text field. Without it
// a file of exactly MAX_UPLOAD_SIZE would be refused for the few hundred bytes
// wrapped around it.
const multipartOverhead = 1 << 20 // 1 MiB

// uploadProductImage godoc
//
//	@Summary		Upload a product image
//	@Description	Admin only. Multipart upload of an image file. The file is identified by its contents, not its name; jpeg, png, gif, and webp are accepted. The first image a product gets becomes its primary one.
//	@Tags			products
//	@Accept			mpfd
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		int							true	"Product ID"
//	@Param			image		formData	file						true	"Image file"
//	@Param			alt_text	formData	string						false	"Alt text for screen readers"
//	@Success		201			{object}	dto.ProductImageEnvelope	"Image stored"
//	@Failure		400			{object}	dto.ErrorEnvelope			"No file, or it is not an image we accept"
//	@Failure		401			{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		403			{object}	dto.ErrorEnvelope			"Not an administrator"
//	@Failure		404			{object}	dto.ErrorEnvelope			"No such product"
//	@Failure		413			{object}	dto.ErrorEnvelope			"File exceeds the upload limit"
//	@Router			/products/{id}/images [post]
func (s *Server) uploadProductImage(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}

	// The real ceiling. Without it gin buffers the whole body before any size
	// check runs, so the reference — which never enforces MAX_UPLOAD_SIZE at
	// all — will happily read a multi-gigabyte request into memory and disk.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.uploads.MaxBytes()+multipartOverhead)

	file, err := c.FormFile("image")
	if err != nil {
		// MaxBytesReader tripped: the body is larger than we agreed to read.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			utils.ErrorResponse(c, http.StatusRequestEntityTooLarge, "file too large", err)
			return
		}

		utils.BadRequestResponse(c, "an image file is required in the 'image' field", err)

		return
	}

	stored, err := s.uploads.UploadProductImage(c.Request.Context(), id, file)
	if err != nil {
		s.respondUploadError(c, err)
		return
	}

	// The bytes are stored but nothing references them yet. If recording them
	// fails, take them back out: an orphaned object is invisible, costs money
	// forever, and nothing will ever come looking for it.
	//
	// The product's existence is checked here rather than before the upload —
	// a prior check would still race with a delete, so the cleanup path has to
	// exist either way, and one code path is better than two.
	image, err := s.products.AddImage(c.Request.Context(), id, stored.URL, c.PostForm("alt_text"))
	if err != nil {
		if removeErr := s.uploads.Remove(c.Request.Context(), stored.Key); removeErr != nil {
			// Logged, not returned: the caller's request failed for the
			// original reason, and the leak is ours to notice.
			s.logger.Error().Err(removeErr).Str("key", stored.Key).
				Msg("failed to remove orphaned upload")
		}

		s.respondProductError(c, err, "failed to record image")

		return
	}

	utils.CreatedResponse(c, "image uploaded", image)
}

// respondUploadError maps the upload service's deliberate errors to statuses.
func (s *Server) respondUploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrFileTooLarge):
		utils.ErrorResponse(c, http.StatusRequestEntityTooLarge, "file too large", err)
	case errors.Is(err, services.ErrUnsupportedFileType):
		utils.BadRequestResponse(c, "unsupported file type: jpeg, png, gif and webp are accepted", err)
	case errors.Is(err, services.ErrEmptyFile):
		utils.BadRequestResponse(c, "the uploaded file is empty", err)
	default:
		utils.InternalServerErrorResponse(c, "failed to store image", err)
	}
}

// respondProductError keeps the status mapping in one place. Only the
// deliberate service errors reach the caller; anything else is a 500 whose
// detail stays in the log.
func (s *Server) respondProductError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, services.ErrProductNotFound):
		utils.NotFoundResponse(c, "product not found")
	case errors.Is(err, services.ErrCategoryNotFound):
		// The product write named a category that does not exist. That is the
		// caller's payload, not a missing product, so 400 rather than 404.
		utils.BadRequestResponse(c, "unknown category", err)
	case errors.Is(err, services.ErrSKUTaken):
		utils.ErrorResponse(c, http.StatusConflict, "sku is not available", err)
	default:
		utils.InternalServerErrorResponse(c, message, err)
	}
}
