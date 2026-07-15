package services_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

// newCatalogServices reuses the auth fixture for its database: the catalogue is
// foreign keys, partial unique indexes, and soft deletes — the parts a fake
// database reproduces least faithfully. These skip without TEST_DATABASE_DSN;
// run them with `make test-integration`.
func newCatalogServices(t *testing.T) (*services.CategoryService, *services.ProductService, *gorm.DB) {
	t.Helper()

	_, db := newAuthService(t)

	return services.NewCategoryService(db), services.NewProductService(db), db
}

// mustCategory creates a category and returns its id.
func mustCategory(t *testing.T, categories *services.CategoryService, name string) uint {
	t.Helper()

	category, err := categories.Create(context.Background(), &dto.CreateCategoryRequest{
		Name:        name,
		Description: "made by a test",
	})
	if err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}

	return category.ID
}

func TestCreateCategory(t *testing.T) {
	categories, _, _ := newCatalogServices(t)

	category, err := categories.Create(context.Background(), &dto.CreateCategoryRequest{
		Name:        "Books",
		Description: "Paper ones",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if category.ID == 0 {
		t.Error("ID = 0, want the assigned id")
	}
	if category.Name != "Books" {
		t.Errorf("Name = %q, want Books", category.Name)
	}
	// A category that arrives inactive would be invisible the moment it is
	// created, which is never what an admin meant.
	if !category.IsActive {
		t.Error("IsActive = false, want a new category to be active")
	}
}

func TestListCategoriesOrdersByNameAndHidesInactive(t *testing.T) {
	categories, _, _ := newCatalogServices(t)
	ctx := context.Background()

	mustCategory(t, categories, "Toys")
	mustCategory(t, categories, "Books")
	hidden := mustCategory(t, categories, "Archive")

	inactive := false
	if _, err := categories.Update(ctx, hidden, &dto.UpdateCategoryRequest{
		Name: "Archive", IsActive: &inactive,
	}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	list, err := categories.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("got %d categories, want 2 — an inactive category must not be listed", len(list))
	}
	if list[0].Name != "Books" || list[1].Name != "Toys" {
		t.Errorf("order = %q, %q; want Books, Toys", list[0].Name, list[1].Name)
	}
}

func TestUpdateCategory(t *testing.T) {
	categories, _, _ := newCatalogServices(t)
	ctx := context.Background()

	id := mustCategory(t, categories, "Books")

	updated, err := categories.Update(ctx, id, &dto.UpdateCategoryRequest{
		Name:        "Literature",
		Description: "Still paper",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Name != "Literature" {
		t.Errorf("Name = %q, want Literature", updated.Name)
	}
	// Omitting is_active must not deactivate the category — the whole reason
	// the field is a pointer.
	if !updated.IsActive {
		t.Error("IsActive = false; omitting is_active must leave it unchanged")
	}
}

func TestUpdateCategoryTogglesActive(t *testing.T) {
	categories, _, _ := newCatalogServices(t)
	ctx := context.Background()

	id := mustCategory(t, categories, "Books")

	inactive := false

	updated, err := categories.Update(ctx, id, &dto.UpdateCategoryRequest{
		Name: "Books", IsActive: &inactive,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.IsActive {
		t.Error("IsActive = true, want false")
	}
}

func TestUpdateCategoryUnknown(t *testing.T) {
	categories, _, _ := newCatalogServices(t)

	_, err := categories.Update(context.Background(), 99999, &dto.UpdateCategoryRequest{Name: "Nope"})
	if !errors.Is(err, services.ErrCategoryNotFound) {
		t.Fatalf("err = %v, want ErrCategoryNotFound", err)
	}
}

func TestDeleteCategory(t *testing.T) {
	categories, _, _ := newCatalogServices(t)
	ctx := context.Background()

	id := mustCategory(t, categories, "Books")

	if err := categories.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, err := categories.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("got %d categories, want 0 after the delete", len(list))
	}
}

// Delete reports no error for an id that was never there, so without the
// RowsAffected check a bogus id would answer 200.
func TestDeleteCategoryUnknown(t *testing.T) {
	categories, _, _ := newCatalogServices(t)

	if err := categories.Delete(context.Background(), 99999); !errors.Is(err, services.ErrCategoryNotFound) {
		t.Fatalf("err = %v, want ErrCategoryNotFound", err)
	}
}

// products.category_id is NOT NULL with a foreign key, so soft-deleting a
// category out from under its products leaves rows pointing at something no
// read path will load. Refuse instead.
func TestDeleteCategoryWithProducts(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	id := mustCategory(t, categories, "Books")

	if _, err := products.Create(ctx, productReq(id, "SKU-KEEP")); err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := categories.Delete(ctx, id); !errors.Is(err, services.ErrCategoryHasProducts) {
		t.Fatalf("err = %v, want ErrCategoryHasProducts", err)
	}

	// And the category must still be there: a refused delete that half-ran
	// would be worse than either outcome.
	list, err := categories.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("got %d categories, want the refused delete to leave it in place", len(list))
	}
}

// Once the products are gone the category is deletable: the count must respect
// soft deletes rather than block forever on rows nobody can see.
func TestDeleteCategoryAfterProductsDeleted(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	id := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(id, "SKU-GONE"))
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if err := products.Delete(ctx, product.ID); err != nil {
		t.Fatalf("delete product: %v", err)
	}

	if err := categories.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v, want the category to be deletable once its products are", err)
	}
}
