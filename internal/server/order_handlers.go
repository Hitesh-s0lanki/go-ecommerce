package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// createOrder godoc
//
//	@Summary		Place an order
//	@Description	Turns your cart into an order: stock comes down, the prices you pay are recorded on the order, and the cart is emptied. All of it happens together or not at all. Fails if any line is out of stock or no longer on sale.
//	@Tags			orders
//	@Produce		json
//	@Security		BearerAuth
//	@Success		201	{object}	dto.OrderEnvelope	"The placed order"
//	@Failure		400	{object}	dto.ErrorEnvelope	"The cart is empty, or a product is no longer on sale"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		404	{object}	dto.ErrorEnvelope	"A product in the cart no longer exists"
//	@Failure		409	{object}	dto.ErrorEnvelope	"Not enough stock for one of the lines"
//	@Router			/orders [post]
func (s *Server) createOrder(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	order, err := s.orders.Create(c.Request.Context(), id)
	if err != nil {
		s.respondOrderError(c, err, "failed to place order")
		return
	}

	utils.CreatedResponse(c, "order placed", order)
}

// getOrders godoc
//
//	@Summary		List your orders
//	@Description	Returns a page of your own orders, newest first.
//	@Tags			orders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page	query		int						false	"Page number, from 1"	default(1)
//	@Param			limit	query		int						false	"Page size, up to 100"	default(20)
//	@Success		200		{object}	dto.OrderListEnvelope	"A page of orders"
//	@Failure		400		{object}	dto.ErrorEnvelope		"Invalid page or limit"
//	@Failure		401		{object}	dto.ErrorEnvelope		"Missing or invalid access token"
//	@Router			/orders [get]
func (s *Server) getOrders(c *gin.Context) {
	id, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	var query dto.ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		utils.BadRequestResponse(c, "invalid pagination", err)
		return
	}

	orders, meta, err := s.orders.List(c.Request.Context(), id, query)
	if err != nil {
		s.respondOrderError(c, err, "failed to list orders")
		return
	}

	utils.PaginatedSuccessResponse(c, "ok", orders, utils.PaginationMeta{
		Page:       meta.Page,
		Limit:      meta.Limit,
		Total:      meta.Total,
		TotalPages: meta.TotalPages,
	})
}

// getOrder godoc
//
//	@Summary		Get one of your orders
//	@Description	Returns an order you placed. Someone else's order id reads as not found.
//	@Tags			orders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int					true	"Order ID"
//	@Success		200	{object}	dto.OrderEnvelope	"The order"
//	@Failure		400	{object}	dto.ErrorEnvelope	"The id is not a number"
//	@Failure		401	{object}	dto.ErrorEnvelope	"Missing or invalid access token"
//	@Failure		404	{object}	dto.ErrorEnvelope	"No such order of yours"
//	@Router			/orders/{id} [get]
func (s *Server) getOrder(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		utils.UnauthorizedResponse(c, "not authenticated")
		return
	}

	orderID, ok := idParam(c, "id")
	if !ok {
		return
	}

	order, err := s.orders.Get(c.Request.Context(), userID, orderID)
	if err != nil {
		s.respondOrderError(c, err, "failed to load order")
		return
	}

	utils.SuccessResponse(c, "ok", order)
}

// respondOrderError maps the order service's deliberate errors to statuses.
func (s *Server) respondOrderError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, services.ErrOrderNotFound):
		utils.NotFoundResponse(c, "order not found")
	case errors.Is(err, services.ErrProductNotFound):
		utils.NotFoundResponse(c, "a product in your cart no longer exists")
	case errors.Is(err, services.ErrCartEmpty):
		utils.BadRequestResponse(c, "your cart is empty", err)
	case errors.Is(err, services.ErrProductUnavailable):
		utils.BadRequestResponse(c, "a product in your cart is no longer available", err)
	case errors.Is(err, services.ErrInsufficientStock):
		utils.ErrorResponse(c, http.StatusConflict, "insufficient stock", err)
	default:
		utils.InternalServerErrorResponse(c, message, err)
	}
}
