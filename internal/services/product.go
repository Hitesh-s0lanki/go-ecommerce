package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// Product errors.
var (
	ErrProductNotFound = errors.New("product not found")
	// ErrSKUTaken is returned when a SKU is already in use by a live product.
	// The unique index is partial, so a soft-deleted product releases its SKU.
	ErrSKUTaken = errors.New("sku is not available")
)

// ProductService manages the catalogue.
type ProductService struct {
	db *gorm.DB
}

// NewProductService builds a ProductService.
func NewProductService(db *gorm.DB) *ProductService {
	return &ProductService{db: db}
}

// Create adds a product.
func (s *ProductService) Create(ctx context.Context, req *dto.CreateProductRequest) (*dto.ProductResponse, error) {
	product := models.Product{
		CategoryID:  req.CategoryID,
		Name:        req.Name,
		Description: req.Description,
		PriceCents:  req.PriceCents,
		Stock:       req.Stock,
		SKU:         req.SKU,
		IsActive:    true,
	}

	if err := s.db.WithContext(ctx).Create(&product).Error; err != nil {
		// The database is the only thing that can answer either question
		// without a race: a prior SELECT would just widen the window.
		switch {
		case errors.Is(err, gorm.ErrDuplicatedKey):
			return nil, ErrSKUTaken
		case errors.Is(err, gorm.ErrForeignKeyViolated):
			return nil, ErrCategoryNotFound
		}

		return nil, fmt.Errorf("create product: %w", err)
	}

	// Re-read so the response carries the category the client will expect,
	// which the insert did not load.
	return s.load(ctx, product.ID)
}

// List returns a page of active products, newest first.
func (s *ProductService) List(ctx context.Context, query dto.ListQuery) ([]dto.ProductResponse, dto.PageMeta, error) {
	query.Normalize()

	var total int64

	// Counted before the page is read; an error here is not ignored, since a
	// dropped count would report a total of zero on a full table.
	if err := s.db.WithContext(ctx).
		Model(&models.Product{}).
		Where("is_active = ?", true).
		Count(&total).Error; err != nil {
		return nil, dto.PageMeta{}, fmt.Errorf("count products: %w", err)
	}

	meta := dto.NewPageMeta(query, total)

	if total == 0 {
		return []dto.ProductResponse{}, meta, nil
	}

	var products []models.Product

	// ORDER BY is what makes the pages disjoint: without one, Postgres may
	// return rows in any order per query, so paging can repeat a product on
	// page 2 and never show another at all.
	if err := s.db.WithContext(ctx).
		Preload("Category").
		Preload("Images", primaryImageFirst).
		Where("is_active = ?", true).
		Order("id DESC").
		Offset(query.Offset()).
		Limit(query.Limit).
		Find(&products).Error; err != nil {
		return nil, dto.PageMeta{}, fmt.Errorf("list products: %w", err)
	}

	resp := make([]dto.ProductResponse, len(products))
	for i := range products {
		resp[i] = dto.NewProductResponse(&products[i])
	}

	return resp, meta, nil
}

// Get returns a single active product, for the public catalogue.
//
// An inactive product is reported as not found: the endpoint is unauthenticated,
// and a withdrawn product must not stay readable to anyone holding its id.
func (s *ProductService) Get(ctx context.Context, id uint) (*dto.ProductResponse, error) {
	var product models.Product

	err := s.db.WithContext(ctx).
		Preload("Category").
		Preload("Images", primaryImageFirst).
		Where("is_active = ?", true).
		First(&product, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}

		return nil, fmt.Errorf("find product: %w", err)
	}

	resp := dto.NewProductResponse(&product)

	return &resp, nil
}

// Update replaces a product's editable fields.
//
// The SKU is not among them: it identifies the item for stock and orders, so
// changing it in place would silently repoint existing references.
func (s *ProductService) Update(ctx context.Context, id uint, req *dto.UpdateProductRequest) (*dto.ProductResponse, error) {
	fields := map[string]any{
		"category_id": req.CategoryID,
		"name":        req.Name,
		"description": req.Description,
		"price_cents": req.PriceCents,
		"stock":       req.Stock,
	}

	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}

	// Named columns, not Save: see CategoryService.Update.
	result := s.db.WithContext(ctx).
		Model(&models.Product{}).
		Where("id = ?", id).
		Updates(fields)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrForeignKeyViolated) {
			return nil, ErrCategoryNotFound
		}

		return nil, fmt.Errorf("update product: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, ErrProductNotFound
	}

	// Loaded rather than returned from the UPDATE: the response needs the
	// category, which RETURNING cannot supply.
	return s.load(ctx, id)
}

// Delete soft-deletes a product. Its SKU becomes available again.
func (s *ProductService) Delete(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&models.Product{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete product: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrProductNotFound
	}

	return nil
}

// load reads a product and its relations whatever its active state, so an admin
// write can answer with the row it just wrote — including a deactivated one,
// which Get deliberately hides.
func (s *ProductService) load(ctx context.Context, id uint) (*dto.ProductResponse, error) {
	var product models.Product

	err := s.db.WithContext(ctx).
		Preload("Category").
		Preload("Images", primaryImageFirst).
		First(&product, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}

		return nil, fmt.Errorf("find product: %w", err)
	}

	resp := dto.NewProductResponse(&product)

	return &resp, nil
}

// primaryImageFirst orders a product's images so the primary one leads and the
// rest are stable, letting a client take images[0] as the thumbnail.
func primaryImageFirst(db *gorm.DB) *gorm.DB {
	return db.Order("is_primary DESC, id")
}
