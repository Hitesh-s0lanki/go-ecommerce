package dto

import "github.com/Hitesh-s0lanki/go-ecommerce/internal/models"

// CreateCategoryRequest is the body of POST /categories.
type CreateCategoryRequest struct {
	Name        string `json:"name"        binding:"required,max=255"`
	Description string `json:"description" binding:"omitempty,max=2000"`
}

// UpdateCategoryRequest is the body of PUT /categories/:id.
type UpdateCategoryRequest struct {
	Name        string `json:"name"        binding:"required,max=255"`
	Description string `json:"description" binding:"omitempty,max=2000"`
	// A pointer so an omitted field is distinguishable from false: without
	// it, every update would silently deactivate the category.
	IsActive *bool `json:"is_active"`
}

// CategoryResponse is a category as returned to clients.
type CategoryResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsActive    bool   `json:"is_active"`
}

// NewCategoryResponse maps a model to its wire shape.
func NewCategoryResponse(c *models.Category) CategoryResponse {
	return CategoryResponse{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		IsActive:    c.IsActive,
	}
}

// CreateProductRequest is the body of POST /products.
//
// Prices are in minor units to match the model: accepting a decimal here would
// reintroduce the float rounding the schema exists to avoid.
type CreateProductRequest struct {
	CategoryID  uint   `json:"category_id" binding:"required"`
	Name        string `json:"name"        binding:"required,max=255"`
	Description string `json:"description" binding:"omitempty,max=5000"`
	PriceCents  int64  `json:"price_cents" binding:"required,gt=0"`
	Stock       int    `json:"stock"       binding:"min=0"`
	SKU         string `json:"sku"         binding:"required,max=100"`
}

// UpdateProductRequest is the body of PUT /products/:id.
type UpdateProductRequest struct {
	CategoryID  uint   `json:"category_id" binding:"required"`
	Name        string `json:"name"        binding:"required,max=255"`
	Description string `json:"description" binding:"omitempty,max=5000"`
	PriceCents  int64  `json:"price_cents" binding:"required,gt=0"`
	Stock       int    `json:"stock"       binding:"min=0"`
	IsActive    *bool  `json:"is_active"`
}

// ProductResponse is a product as returned to clients.
type ProductResponse struct {
	ID          uint   `json:"id"`
	CategoryID  uint   `json:"category_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PriceCents  int64  `json:"price_cents"`
	Stock       int    `json:"stock"`
	SKU         string `json:"sku"`
	IsActive    bool   `json:"is_active"`
	// Pointer and omitempty: an unloaded relation is absent rather than an
	// object full of zero values that a client cannot tell from real data.
	Category *CategoryResponse      `json:"category,omitempty"`
	Images   []ProductImageResponse `json:"images,omitempty"`
}

// ProductImageResponse is a product image as returned to clients.
type ProductImageResponse struct {
	ID        uint   `json:"id"`
	URL       string `json:"url"`
	AltText   string `json:"alt_text,omitempty"`
	IsPrimary bool   `json:"is_primary"`
}

// NewProductResponse maps a model to its wire shape, including any relations
// that were preloaded.
func NewProductResponse(p *models.Product) ProductResponse {
	resp := ProductResponse{
		ID:          p.ID,
		CategoryID:  p.CategoryID,
		Name:        p.Name,
		Description: p.Description,
		PriceCents:  p.PriceCents,
		Stock:       p.Stock,
		SKU:         p.SKU,
		IsActive:    p.IsActive,
	}

	if p.Category != nil {
		category := NewCategoryResponse(p.Category)
		resp.Category = &category
	}

	for i := range p.Images {
		resp.Images = append(resp.Images, NewProductImageResponse(&p.Images[i]))
	}

	return resp
}

// NewProductImageResponse maps a model to its wire shape.
func NewProductImageResponse(img *models.ProductImage) ProductImageResponse {
	return ProductImageResponse{
		ID:        img.ID,
		URL:       img.URL,
		AltText:   img.AltText,
		IsPrimary: img.IsPrimary,
	}
}
