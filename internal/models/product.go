package models

import (
	"time"

	"gorm.io/gorm"
)

// Category groups products.
type Category struct {
	ID          uint   `json:"id"                    gorm:"primaryKey"`
	Name        string `json:"name"                  gorm:"not null"`
	Description string `json:"description,omitempty"`
	IsActive    bool   `json:"is_active"             gorm:"not null;default:true"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Products []Product `json:"-"`
}

// Product is an item for sale.
type Product struct {
	ID          uint   `json:"id"                    gorm:"primaryKey"`
	CategoryID  uint   `json:"category_id"           gorm:"not null;index"`
	Name        string `json:"name"                  gorm:"not null"`
	Description string `json:"description,omitempty"`
	// PriceCents is the price in minor units (1999 = $19.99). Money is never
	// float: binary floating point cannot represent decimal cents exactly, so
	// summing line items drifts.
	PriceCents int64 `json:"price_cents" gorm:"not null;check:price_cents >= 0"`
	Stock      int   `json:"stock"       gorm:"not null;default:0;check:stock >= 0"`
	// SKU's unique index is partial, so a soft-deleted product does not
	// reserve its SKU forever.
	SKU      string `json:"sku"       gorm:"not null;uniqueIndex:uniq_products_sku,where:deleted_at IS NULL"`
	IsActive bool   `json:"is_active" gorm:"not null;default:true"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Category   *Category      `json:"category,omitempty"`
	Images     []ProductImage `json:"images,omitempty"`
	OrderItems []OrderItem    `json:"-"`
	CartItems  []CartItem     `json:"-"`
}

// InStock reports whether at least qty units are available.
func (p *Product) InStock(qty int) bool {
	return p.IsActive && p.Stock >= qty
}

// ProductImage is a picture of a product.
type ProductImage struct {
	ID        uint   `json:"id"                 gorm:"primaryKey"`
	ProductID uint   `json:"product_id"         gorm:"not null;index"`
	URL       string `json:"url"                gorm:"not null"`
	AltText   string `json:"alt_text,omitempty"`
	IsPrimary bool   `json:"is_primary"         gorm:"not null;default:false"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Product *Product `json:"-"`
}
