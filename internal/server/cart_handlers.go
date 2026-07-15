package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// getCart godoc
//
//	@Summary		Get your cart
//	@Description	Returns the authenticated user's cart, priced from the products' current prices. A user who has never had a cart gets an empty one rather than a 404.
//	@Tags			cart
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.CartEnvelope	"The cart"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		404	{object}	dto.ErrorEnvelope	"Account no longer exists"
//	@Router			/cart [get]
func (s *Server) getCart(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	cart, err := s.carts.Get(c.Request.Context(), id)
	if err != nil {
		s.respondCartError(c, err, "failed to load cart")
		return
	}

	utils.SuccessResponse(c, "ok", cart)
}

// addToCart godoc
//
//	@Summary		Add an item to your cart
//	@Description	Adds a product, or increases the quantity already in the cart. The cart is not a reservation — stock is only held once the order is placed — but a quantity beyond what is in stock is refused here rather than at checkout.
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		dto.AddToCartRequest	true	"Product and quantity"
//	@Success		200		{object}	dto.CartEnvelope		"The updated cart"
//	@Failure		400		{object}	dto.ErrorEnvelope		"Validation failed, or the product is not on sale"
//	@Failure		401		{object}	dto.ErrorEnvelope		"Missing or invalid access token"
//	@Failure		404		{object}	dto.ErrorEnvelope		"No such product"
//	@Failure		409		{object}	dto.ErrorEnvelope		"Not enough stock"
//	@Router			/cart/items [post]
func (s *Server) addToCart(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	var req dto.AddToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	cart, err := s.carts.AddItem(c.Request.Context(), id, &req)
	if err != nil {
		s.respondCartError(c, err, "failed to add to cart")
		return
	}

	utils.SuccessResponse(c, "item added", cart)
}

// updateCartItem godoc
//
//	@Summary		Set the quantity of a cart item
//	@Description	Replaces the quantity of one line. To remove it, use DELETE — a quantity of zero is refused.
//	@Tags			cart
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Cart item ID"
//	@Param			request	body		dto.UpdateCartItemRequest	true	"New quantity"
//	@Success		200		{object}	dto.CartEnvelope			"The updated cart"
//	@Failure		400		{object}	dto.ErrorEnvelope			"Validation failed, or the product is not on sale"
//	@Failure		401		{object}	dto.ErrorEnvelope			"Missing or invalid access token"
//	@Failure		404		{object}	dto.ErrorEnvelope			"No such item in your cart"
//	@Failure		409		{object}	dto.ErrorEnvelope			"Not enough stock"
//	@Router			/cart/items/{id} [put]
func (s *Server) updateCartItem(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	itemID, ok := idParam(c, "id")
	if !ok {
		return
	}

	var req dto.UpdateCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "invalid request body", err)
		return
	}

	cart, err := s.carts.UpdateItem(c.Request.Context(), userID, itemID, &req)
	if err != nil {
		s.respondCartError(c, err, "failed to update cart item")
		return
	}

	utils.SuccessResponse(c, "item updated", cart)
}

// removeFromCart godoc
//
//	@Summary		Remove an item from your cart
//	@Tags			cart
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int					true	"Cart item ID"
//	@Success		200	{object}	dto.MessageEnvelope	"Item removed"
//	@Failure		400	{object}	dto.ErrorEnvelope	"The id is not a number"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		404	{object}	dto.ErrorEnvelope	"No such item in your cart"
//	@Router			/cart/items/{id} [delete]
func (s *Server) removeFromCart(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	itemID, ok := idParam(c, "id")
	if !ok {
		return
	}

	if err := s.carts.RemoveItem(c.Request.Context(), userID, itemID); err != nil {
		s.respondCartError(c, err, "failed to remove cart item")
		return
	}

	utils.SuccessResponse(c, "item removed", nil)
}

// respondCartError maps the cart service's deliberate errors to statuses.
func (s *Server) respondCartError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, services.ErrCartItemNotFound):
		utils.NotFoundResponse(c, "cart item not found")
	case errors.Is(err, services.ErrProductNotFound):
		utils.NotFoundResponse(c, "product not found")
	case errors.Is(err, services.ErrUserNotFound):
		utils.NotFoundResponse(c, "account not found")
	case errors.Is(err, services.ErrProductUnavailable):
		utils.BadRequestResponse(c, "product is not available", err)
	case errors.Is(err, services.ErrInsufficientStock):
		// 409: the request is well-formed and will work once there is stock,
		// so it is a conflict with the current state rather than a bad ask.
		utils.ErrorResponse(c, http.StatusConflict, "insufficient stock", err)
	default:
		utils.InternalServerErrorResponse(c, message, err)
	}
}
