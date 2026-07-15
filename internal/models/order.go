package models

import (
	"time"

	"gorm.io/gorm"
)

// OrderStatus is the position of an order in its lifecycle.
type OrderStatus string

// Supported order statuses.
const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// Valid reports whether s is a known status.
func (s OrderStatus) Valid() bool {
	switch s {
	case OrderStatusPending, OrderStatusConfirmed, OrderStatusShipped,
		OrderStatusDelivered, OrderStatusCancelled:
		return true
	default:
		return false
	}
}

// Order is a placed order.
type Order struct {
	ID     uint        `json:"id"      gorm:"primaryKey"`
	UserID uint        `json:"user_id" gorm:"not null;index"`
	Status OrderStatus `json:"status"  gorm:"type:varchar(20);not null;default:pending;index"`
	// TotalAmountCents is the order total in minor units. See Product.PriceCents.
	TotalAmountCents int64 `json:"total_amount_cents" gorm:"not null;check:total_amount_cents >= 0"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	User       *User       `json:"user,omitempty"`
	OrderItems []OrderItem `json:"order_items,omitempty"`
}

// OrderItem is a single line of an order.
type OrderItem struct {
	ID        uint `json:"id"         gorm:"primaryKey"`
	OrderID   uint `json:"order_id"   gorm:"not null;index"`
	ProductID uint `json:"product_id" gorm:"not null;index"`
	Quantity  int  `json:"quantity"   gorm:"not null;check:quantity > 0"`
	// UnitPriceCents is the price at the time of purchase, copied from the
	// product so later price changes do not rewrite order history.
	UnitPriceCents int64 `json:"unit_price_cents" gorm:"not null;check:unit_price_cents >= 0"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Order   *Order   `json:"-"`
	Product *Product `json:"product,omitempty"`
}

// SubtotalCents is the line total.
func (i *OrderItem) SubtotalCents() int64 {
	return i.UnitPriceCents * int64(i.Quantity)
}

// Cart is a user's active shopping cart. A user has at most one.
type Cart struct {
	ID uint `json:"id" gorm:"primaryKey"`
	// Partial unique index: one live cart per user, but a soft-deleted cart
	// must not block creating a new one.
	UserID uint `json:"user_id" gorm:"not null;uniqueIndex:uniq_carts_user_id,where:deleted_at IS NULL"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	CartItems []CartItem `json:"cart_items,omitempty"`
}

// CartItem is a product held in a cart.
//
// Unlike OrderItem it stores no price: a cart is priced from the product at
// checkout, so it always reflects the current price.
type CartItem struct {
	ID        uint `json:"id"         gorm:"primaryKey"`
	CartID    uint `json:"cart_id"    gorm:"not null;index"`
	ProductID uint `json:"product_id" gorm:"not null;index"`
	Quantity  int  `json:"quantity"   gorm:"not null;check:quantity > 0"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Cart    *Cart    `json:"-"`
	Product *Product `json:"product,omitempty"`
}
