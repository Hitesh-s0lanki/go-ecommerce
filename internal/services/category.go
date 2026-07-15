package services

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// Category errors.
var (
	// ErrCategoryNotFound is returned when a category does not exist. It also
	// answers a product write that names an unknown category, since the
	// foreign key is the only thing that can tell us.
	ErrCategoryNotFound = errors.New("category not found")
	// ErrCategoryHasProducts is returned when a delete would orphan products.
	ErrCategoryHasProducts = errors.New("category still has products")
)

// CategoryService manages the product categories.
type CategoryService struct {
	db *gorm.DB
}

// NewCategoryService builds a CategoryService.
func NewCategoryService(db *gorm.DB) *CategoryService {
	return &CategoryService{db: db}
}

// Create adds a category.
func (s *CategoryService) Create(ctx context.Context, req *dto.CreateCategoryRequest) (*dto.CategoryResponse, error) {
	category := models.Category{
		Name:        req.Name,
		Description: req.Description,
		// Set explicitly rather than left to the column default: gorm omits
		// zero-valued fields that have a default tag, so the row would come
		// back active either way — but only by a rule that is easy to miss.
		IsActive: true,
	}

	if err := s.db.WithContext(ctx).Create(&category).Error; err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}

	resp := dto.NewCategoryResponse(&category)

	return &resp, nil
}

// List returns the active categories, for the public catalogue.
func (s *CategoryService) List(ctx context.Context) ([]dto.CategoryResponse, error) {
	var categories []models.Category

	// Ordered by name: without an ORDER BY, Postgres may return rows in any
	// order, so the same request can answer differently between calls.
	if err := s.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("name").
		Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	resp := make([]dto.CategoryResponse, len(categories))
	for i := range categories {
		resp[i] = dto.NewCategoryResponse(&categories[i])
	}

	return resp, nil
}

// Update replaces a category's editable fields.
//
// The named columns are written directly rather than loading the row and
// calling Save, which rewrites every column from the copy it read and so
// reverts anything committed in between. See UserService.UpdateProfile.
func (s *CategoryService) Update(ctx context.Context, id uint, req *dto.UpdateCategoryRequest) (*dto.CategoryResponse, error) {
	fields := map[string]any{
		"name":        req.Name,
		"description": req.Description,
	}

	// Absent means "leave it alone". A plain bool could not say that: every
	// update that omitted the field would deactivate the category.
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}

	var category models.Category

	// Postgres returns the updated row, so this is one round trip rather than
	// an update followed by a re-read.
	result := s.db.WithContext(ctx).
		Model(&category).
		Clauses(clause.Returning{}).
		Where("id = ?", id).
		Updates(fields)

	if result.Error != nil {
		return nil, fmt.Errorf("update category: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, ErrCategoryNotFound
	}

	resp := dto.NewCategoryResponse(&category)

	return &resp, nil
}

// Delete soft-deletes a category.
//
// A category with products is refused rather than deleted. products.category_id
// is NOT NULL with a foreign key, so a soft-deleted parent leaves rows pointing
// at a category that no read path will load — the products simply lose their
// category with no way to notice. Deactivate it instead to hide it, or move the
// products first.
func (s *CategoryService) Delete(ctx context.Context, id uint) error {
	// One transaction so the count cannot go stale between the check and the
	// delete.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64

		// Soft-deleted products are excluded by gorm, so a category whose
		// products are all deleted can still be removed.
		if err := tx.Model(&models.Product{}).
			Where("category_id = ?", id).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count products: %w", err)
		}

		if count > 0 {
			return ErrCategoryHasProducts
		}

		result := tx.Delete(&models.Category{}, id)
		if result.Error != nil {
			return fmt.Errorf("delete category: %w", result.Error)
		}

		// Delete reports no error for an id that was never there, which would
		// answer a bogus id with 200.
		if result.RowsAffected == 0 {
			return ErrCategoryNotFound
		}

		return nil
	})
}
