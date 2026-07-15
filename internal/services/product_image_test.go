package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

func TestAddImage(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(categoryID, "SKU-IMG"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	image, err := products.AddImage(ctx, product.ID, "/uploads/products/1/a.png", "A book cover")
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	if image.ID == 0 {
		t.Error("ID = 0, want the assigned id")
	}
	if image.URL != "/uploads/products/1/a.png" {
		t.Errorf("URL = %q, want the stored url", image.URL)
	}
	// A product's first image is what any listing will show as its thumbnail.
	if !image.IsPrimary {
		t.Error("IsPrimary = false, want a product's first image to be primary")
	}
}

func TestAddImageOnlyFirstIsPrimary(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(categoryID, "SKU-IMG2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	first, err := products.AddImage(ctx, product.ID, "/uploads/a.png", "")
	if err != nil {
		t.Fatalf("first AddImage: %v", err)
	}

	second, err := products.AddImage(ctx, product.ID, "/uploads/b.png", "")
	if err != nil {
		t.Fatalf("second AddImage: %v", err)
	}

	if !first.IsPrimary {
		t.Error("the first image is not primary")
	}
	if second.IsPrimary {
		t.Error("the second image is primary too; a product has one primary image")
	}
}

// The images must come back on the product, primary first, so a client can take
// images[0] as the thumbnail.
func TestAddImageAppearsOnProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(categoryID, "SKU-IMG3"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := products.AddImage(ctx, product.ID, "/uploads/first.png", "first"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	if _, err := products.AddImage(ctx, product.ID, "/uploads/second.png", "second"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	got, err := products.Get(ctx, product.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(got.Images) != 2 {
		t.Fatalf("got %d images, want 2", len(got.Images))
	}
	if !got.Images[0].IsPrimary {
		t.Error("images[0] is not the primary one")
	}
	if got.Images[0].URL != "/uploads/first.png" {
		t.Errorf("images[0].URL = %q, want the primary image", got.Images[0].URL)
	}
}

func TestAddImageUnknownProduct(t *testing.T) {
	_, products, _ := newCatalogServices(t)

	_, err := products.AddImage(context.Background(), 99999, "/uploads/a.png", "")
	if !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

// The foreign key only proves the product row exists, and a soft-deleted
// product still has one — so without the explicit check this would attach an
// image to a product nobody can fetch.
func TestAddImageDeletedProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(categoryID, "SKU-IMG4"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := products.Delete(ctx, product.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = products.AddImage(ctx, product.ID, "/uploads/a.png", "")
	if !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound for a deleted product", err)
	}
}

// An image on a deactivated product is fine: an admin stages a product before
// putting it on sale.
func TestAddImageInactiveProduct(t *testing.T) {
	categories, products, _ := newCatalogServices(t)
	ctx := context.Background()

	categoryID := mustCategory(t, categories, "Books")

	product, err := products.Create(ctx, productReq(categoryID, "SKU-IMG5"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	inactive := false
	if _, err := products.Update(ctx, product.ID, updateReq(categoryID, &inactive)); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if _, err := products.AddImage(ctx, product.ID, "/uploads/a.png", ""); err != nil {
		t.Fatalf("AddImage: %v, want staging an image on an inactive product to work", err)
	}
}
