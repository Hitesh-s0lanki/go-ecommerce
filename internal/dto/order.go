package dto

import (
	"time"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// AddToCartRequest is the body of POST /cart/items.
type AddToCartRequest struct {
	ProductID uint `json:"product_id" binding:"required"`
	Quantity  int  `json:"quantity"   binding:"required,min=1"`
}

// UpdateCartItemRequest is the body of PUT /cart/items/:id.
type UpdateCartItemRequest struct {
	Quantity int `json:"quantity" binding:"required,min=1"`
}

// CartResponse is a cart as returned to clients.
type CartResponse struct {
	ID          uint               `json:"id"`
	UserID      uint               `json:"user_id"`
	CartItems   []CartItemResponse `json:"cart_items"`
	TotalCents  int64              `json:"total_cents"`
	ItemCount   int                `json:"item_count"`
	LastUpdated time.Time          `json:"last_updated"`
}

// CartItemResponse is a line of a cart.
type CartItemResponse struct {
	ID            uint             `json:"id"`
	Product       *ProductResponse `json:"product,omitempty"`
	Quantity      int              `json:"quantity"`
	SubtotalCents int64            `json:"subtotal_cents"`
}

// OrderResponse is an order as returned to clients.
type OrderResponse struct {
	ID     uint   `json:"id"`
	UserID uint   `json:"user_id"`
	Status string `json:"status"`
	// Minor units, matching the model.
	TotalAmountCents int64               `json:"total_amount_cents"`
	OrderItems       []OrderItemResponse `json:"order_items,omitempty"`
	// A real timestamp, not a preformatted string: clients localise it
	// themselves, and encoding/json already emits RFC 3339.
	CreatedAt time.Time `json:"created_at"`
}

// OrderItemResponse is a line of an order.
type OrderItemResponse struct {
	ID      uint             `json:"id"`
	Product *ProductResponse `json:"product,omitempty"`
	// Quantity and the price paid at the time of purchase.
	Quantity       int   `json:"quantity"`
	UnitPriceCents int64 `json:"unit_price_cents"`
	SubtotalCents  int64 `json:"subtotal_cents"`
}

// NewCartResponse maps a model to its wire shape, summing the cart from the
// products' current prices.
//
// A cart holds no price of its own, so an unloaded Product contributes 0 —
// callers must preload it.
func NewCartResponse(c *models.Cart) CartResponse {
	resp := CartResponse{
		ID:          c.ID,
		UserID:      c.UserID,
		CartItems:   make([]CartItemResponse, 0, len(c.CartItems)),
		LastUpdated: c.UpdatedAt,
	}

	for i := range c.CartItems {
		item := NewCartItemResponse(&c.CartItems[i])
		resp.CartItems = append(resp.CartItems, item)
		resp.TotalCents += item.SubtotalCents
		resp.ItemCount += item.Quantity
	}

	return resp
}

// NewCartItemResponse maps a model to its wire shape.
func NewCartItemResponse(item *models.CartItem) CartItemResponse {
	resp := CartItemResponse{
		ID:       item.ID,
		Quantity: item.Quantity,
	}

	if item.Product != nil {
		product := NewProductResponse(item.Product)
		resp.Product = &product
		resp.SubtotalCents = item.Product.PriceCents * int64(item.Quantity)
	}

	return resp
}

// NewOrderResponse maps a model to its wire shape.
func NewOrderResponse(o *models.Order) OrderResponse {
	resp := OrderResponse{
		ID:               o.ID,
		UserID:           o.UserID,
		Status:           string(o.Status),
		TotalAmountCents: o.TotalAmountCents,
		CreatedAt:        o.CreatedAt,
	}

	for i := range o.OrderItems {
		resp.OrderItems = append(resp.OrderItems, NewOrderItemResponse(&o.OrderItems[i]))
	}

	return resp
}

// NewOrderItemResponse maps a model to its wire shape, using the price
// recorded on the line rather than the product's current price.
func NewOrderItemResponse(item *models.OrderItem) OrderItemResponse {
	resp := OrderItemResponse{
		ID:             item.ID,
		Quantity:       item.Quantity,
		UnitPriceCents: item.UnitPriceCents,
		SubtotalCents:  item.SubtotalCents(),
	}

	if item.Product != nil {
		product := NewProductResponse(item.Product)
		resp.Product = &product
	}

	return resp
}
