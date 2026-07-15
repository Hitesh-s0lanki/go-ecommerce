package services_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

func productReq(categoryID uint, sku string) *dto.CreateProductRequest {
	return &dto.CreateProductRequest{
		CategoryID:  categoryID,
		Name:        "A thing",
		Description: "made by a test",
		PriceCents:  1999,
		Stock:       5,
		SKU:         sku,
	}
}

func TestCreateProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(categoryID, "SKU-1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if product.ID == 0 {
		t.Error("ID = 0, want the assigned id")
	}
	if product.PriceCents != 1999 {
		t.Errorf("PriceCents = %d, want 1999", product.PriceCents)
	}
	if !product.IsActive {
		t.Error("IsActive = false, want a new product to be on sale")
	}
	// The insert cannot load the category, so a create that did not re-read
	// would answer without one.
	if product.Category == nil {
		t.Fatal("Category is nil, want it preloaded on the created product")
	}
	if product.Category.Name != "Books" {
		t.Errorf("Category.Name = %q, want Books", product.Category.Name)
	}
}

// The unique index is the only thing that can decide this without a race: a
// SELECT-then-INSERT just widens the window between the two.
func TestCreateProductDuplicateSKU(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	if _, err := products.Create(ctx, productReq(categoryID, "SKU-DUP")); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := products.Create(ctx, productReq(categoryID, "SKU-DUP"))
	if !errors.Is(err, services.ErrSKUTaken) {
		t.Fatalf("err = %v, want ErrSKUTaken", err)
	}
}

// The SKU index is partial on deleted_at IS NULL, so a soft-deleted product
// must not reserve its SKU forever.
func TestCreateProductReusesSKUOfDeletedProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	first, err := products.Create(ctx, productReq(categoryID, "SKU-REUSE"))
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	if err := products.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := products.Create(ctx, productReq(categoryID, "SKU-REUSE")); err != nil {
		t.Fatalf("Create after delete: %v, want the SKU to be free again", err)
	}
}

func TestCreateProductUnknownCategory(t *testing.T) {
	_, products, _ := newCatalogServices(t)

	_, err := products.Create(context.Background(), productReq(99999, "SKU-ORPHAN"))
	if !errors.Is(err, services.ErrCategoryNotFound) {
		t.Fatalf("err = %v, want ErrCategoryNotFound", err)
	}
}

func TestGetProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-GET"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := products.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.SKU != "SKU-GET" {
		t.Errorf("SKU = %q, want SKU-GET", got.SKU)
	}
	if got.Category == nil || got.Category.ID != categoryID {
		t.Error("Category was not preloaded")
	}
}

func TestGetProductUnknown(t *testing.T) {
	_, products, _ := newCatalogServices(t)

	if _, err := products.Get(context.Background(), 99999); !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

// Get backs an unauthenticated endpoint, so a withdrawn product must not stay
// readable to anyone who kept its id.
func TestGetProductInactive(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-HIDDEN"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	inactive := false
	if _, err := products.Update(ctx, created.ID, updateReq(categoryID, &inactive)); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if _, err := products.Get(ctx, created.ID); !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound for an inactive product", err)
	}
}

// An admin write answers with the row it wrote, including a deactivated one —
// otherwise deactivating a product would fail on its own response.
func TestUpdateProductReturnsDeactivatedProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-OFF"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	inactive := false

	updated, err := products.Update(ctx, created.ID, updateReq(categoryID, &inactive))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.IsActive {
		t.Error("IsActive = true, want false")
	}
}

func updateReq(categoryID uint, isActive *bool) *dto.UpdateProductRequest {
	return &dto.UpdateProductRequest{
		CategoryID:  categoryID,
		Name:        "A thing",
		Description: "made by a test",
		PriceCents:  1999,
		Stock:       5,
		IsActive:    isActive,
	}
}

func TestUpdateProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-UPD"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := products.Update(ctx, created.ID, &dto.UpdateProductRequest{
		CategoryID: categoryID,
		Name:       "A better thing",
		PriceCents: 2999,
		Stock:      2,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Name != "A better thing" || updated.PriceCents != 2999 || updated.Stock != 2 {
		t.Errorf("got %q/%d/%d, want A better thing/2999/2", updated.Name, updated.PriceCents, updated.Stock)
	}
	// Omitting is_active must leave it alone, not deactivate the product.
	if !updated.IsActive {
		t.Error("IsActive = false; omitting is_active must leave it unchanged")
	}
	// The SKU is not editable: it identifies the item for stock and orders.
	if updated.SKU != "SKU-UPD" {
		t.Errorf("SKU = %q, want it untouched by an update", updated.SKU)
	}
}

func TestUpdateProductUnknown(t *testing.T) {
	categories, products, _ := newCatalogServices(t)

	categoryID := mustCategory(t, categories, "Books")

	_, err := products.Update(context.Background(), 99999, updateReq(categoryID, nil))
	if !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

func TestUpdateProductUnknownCategory(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-MOVE"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = products.Update(ctx, created.ID, updateReq(99999, nil))
	if !errors.Is(err, services.ErrCategoryNotFound) {
		t.Fatalf("err = %v, want ErrCategoryNotFound", err)
	}
}

func TestDeleteProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-DEL"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := products.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := products.Get(ctx, created.ID); !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound after the delete", err)
	}
}

func TestDeleteProductUnknown(t *testing.T) {
	_, products, _ := newCatalogServices(t)

	if err := products.Delete(context.Background(), 99999); !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

func TestListProductsPaginates(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	const count = 5
	for i := range count {
		if _, err := products.Create(ctx, productReq(categoryID, fmt.Sprintf("SKU-P%d", i))); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	page1, meta, err := products.List(ctx, dto.ListQuery{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if meta.Total != count {
		t.Errorf("Total = %d, want %d", meta.Total, count)
	}
	// 5 rows at 2 per page is 3 pages, not 2: the ceiling division matters.
	if meta.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", meta.TotalPages)
	}
	if len(page1) != 2 {
		t.Fatalf("got %d products, want 2", len(page1))
	}

	page2, _, err := products.List(ctx, dto.ListQuery{Page: 2, Limit: 2})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}

	// The pages must be disjoint. Without an ORDER BY they need not be, and a
	// product can appear twice while another is never shown at all.
	for _, a := range page1 {
		for _, b := range page2 {
			if a.ID == b.ID {
				t.Fatalf("product %d appears on both pages; the pages must not overlap", a.ID)
			}
		}
	}
}

func TestListProductsHidesInactive(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	created, err := products.Create(ctx, productReq(categoryID, "SKU-LIST"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	inactive := false
	if _, err := products.Update(ctx, created.ID, updateReq(categoryID, &inactive)); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	list, meta, err := products.List(ctx, dto.ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 0 || meta.Total != 0 {
		t.Errorf("got %d products (total %d), want an inactive product to be hidden", len(list), meta.Total)
	}
}

func TestListProductsDefaultsAndCapsLimit(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	if _, err := products.Create(ctx, productReq(categoryID, "SKU-ONE")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A zero query must not mean "offset -20, limit 0".
	_, meta, err := products.List(ctx, dto.ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if meta.Page != 1 || meta.Limit != 20 {
		t.Errorf("page/limit = %d/%d, want the 1/20 defaults", meta.Page, meta.Limit)
	}

	// The HTTP layer rejects an oversized limit, but a service caller must not
	// be able to ask for the whole table either.
	_, meta, err = products.List(ctx, dto.ListQuery{Page: 1, Limit: 100000})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if meta.Limit != dto.MaxLimit {
		t.Errorf("Limit = %d, want it capped at %d", meta.Limit, dto.MaxLimit)
	}
}

// An empty catalogue must answer with an empty list, not a null the client has
// to special-case.
func TestListProductsEmpty(t *testing.T) {
	_, products, _ := newCatalogServices(t)

	list, meta, err := products.List(context.Background(), dto.ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if list == nil {
		t.Error("list is nil, want an empty slice")
	}
	if meta.Total != 0 || meta.TotalPages != 0 {
		t.Errorf("total/pages = %d/%d, want 0/0", meta.Total, meta.TotalPages)
	}
}
