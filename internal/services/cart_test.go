package services_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

// shopFixture is a registered customer plus a product to buy.
type shopFixture struct {
	carts     *services.CartService
	orders    *services.OrderService
	products  *services.ProductService
	db        *gorm.DB
	userID    uint
	productID uint
}

// newShop builds the whole purchase path against real Postgres: carts are
// upserts, row locks, and partial unique indexes, none of which a fake database
// reproduces. Skips without TEST_DATABASE_DSN; run with `make test-integration`.
func newShop(t *testing.T, stock int) *shopFixture {
	t.Helper()

	authSvc, db := newAuthService(t)
	ctx := context.Background()

	reg, err := authSvc.Register(ctx, registerReq("shopper@example.com"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	categories := services.NewCategoryService(db)
	products := services.NewProductService(db)

	categoryID := mustCategory(t, categories, "Books")

	req := productReq(categoryID, "SKU-SHOP")
	req.Stock = stock

	product, err := products.Create(ctx, req)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	return &shopFixture{
		carts:     services.NewCartService(db),
		orders:    services.NewOrderService(db),
		products:  products,
		db:        db,
		userID:    reg.User.ID,
		productID: product.ID,
	}
}

// A new customer has an empty cart, not a missing one. The reference answers
// their first GET /cart with "record not found".
func TestGetCartCreatesEmptyCart(t *testing.T) {
	shop := newShop(t, 10)

	cart, err := shop.carts.Get(context.Background(), shop.userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if cart.ID == 0 {
		t.Error("ID = 0, want a cart")
	}
	if len(cart.CartItems) != 0 {
		t.Errorf("got %d items, want an empty cart", len(cart.CartItems))
	}
	if cart.TotalCents != 0 {
		t.Errorf("TotalCents = %d, want 0", cart.TotalCents)
	}
}

func TestAddToCart(t *testing.T) {
	shop := newShop(t, 10)

	cart, err := shop.carts.AddItem(context.Background(), shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if len(cart.CartItems) != 1 {
		t.Fatalf("got %d items, want 1", len(cart.CartItems))
	}
	if cart.CartItems[0].Quantity != 2 {
		t.Errorf("Quantity = %d, want 2", cart.CartItems[0].Quantity)
	}
	// productReq prices at 1999.
	if cart.TotalCents != 2*1999 {
		t.Errorf("TotalCents = %d, want %d", cart.TotalCents, 2*1999)
	}
	if cart.ItemCount != 2 {
		t.Errorf("ItemCount = %d, want 2", cart.ItemCount)
	}
	// The cart renders the product, so it must come back loaded.
	if cart.CartItems[0].Product == nil {
		t.Error("Product is nil, want it preloaded on the cart line")
	}
}

// Adding the same product twice is one line with the quantities summed, not two
// lines. The reference reads then writes, so concurrent adds duplicate the line.
func TestAddToCartSameProductAccumulates(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 2,
	}); err != nil {
		t.Fatalf("first AddItem: %v", err)
	}

	cart, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 3,
	})
	if err != nil {
		t.Fatalf("second AddItem: %v", err)
	}

	if len(cart.CartItems) != 1 {
		t.Fatalf("got %d lines, want 1 line holding both adds", len(cart.CartItems))
	}
	if cart.CartItems[0].Quantity != 5 {
		t.Errorf("Quantity = %d, want 5", cart.CartItems[0].Quantity)
	}
}

// The stock check must test the resulting quantity, not just what this request
// added: three adds of 2 against a stock of 5 must not leave 6 in the cart.
func TestAddToCartAccumulatedQuantityChecksStock(t *testing.T) {
	shop := newShop(t, 5)
	ctx := context.Background()

	for i := range 2 {
		if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
			ProductID: shop.productID, Quantity: 2,
		}); err != nil {
			t.Fatalf("AddItem %d: %v", i, err)
		}
	}

	_, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 2,
	})
	if !errors.Is(err, services.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}

	// And the refused add must not have been applied.
	cart, err := shop.carts.Get(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cart.CartItems[0].Quantity != 4 {
		t.Errorf("Quantity = %d, want the failed add rolled back to 4", cart.CartItems[0].Quantity)
	}
}

func TestAddToCartInsufficientStock(t *testing.T) {
	shop := newShop(t, 3)

	_, err := shop.carts.AddItem(context.Background(), shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 4,
	})
	if !errors.Is(err, services.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
}

func TestAddToCartUnknownProduct(t *testing.T) {
	shop := newShop(t, 10)

	_, err := shop.carts.AddItem(context.Background(), shop.userID, &dto.AddToCartRequest{
		ProductID: 99999, Quantity: 1,
	})
	if !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

// A withdrawn product must not be addable. The reference checks neither
// is_active nor the soft delete, so it can be carted and bought.
func TestAddToCartInactiveProduct(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	var categoryID uint
	if err := shop.db.Model(&models.Product{}).Select("category_id").
		Where("id = ?", shop.productID).Scan(&categoryID).Error; err != nil {
		t.Fatalf("read category: %v", err)
	}

	inactive := false
	if _, err := shop.products.Update(ctx, shop.productID, updateReq(categoryID, &inactive)); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	_, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	})
	if !errors.Is(err, services.ErrProductUnavailable) {
		t.Fatalf("err = %v, want ErrProductUnavailable", err)
	}
}

func TestAddToCartDeletedProduct(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if err := shop.products.Delete(ctx, shop.productID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	})
	if !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

func TestUpdateCartItem(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	added, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	itemID := added.CartItems[0].ID

	cart, err := shop.carts.UpdateItem(ctx, shop.userID, itemID, &dto.UpdateCartItemRequest{Quantity: 7})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	if cart.CartItems[0].Quantity != 7 {
		t.Errorf("Quantity = %d, want 7 — a set, not an add", cart.CartItems[0].Quantity)
	}
}

func TestUpdateCartItemInsufficientStock(t *testing.T) {
	shop := newShop(t, 5)
	ctx := context.Background()

	added, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	_, err = shop.carts.UpdateItem(ctx, shop.userID, added.CartItems[0].ID,
		&dto.UpdateCartItemRequest{Quantity: 6})
	if !errors.Is(err, services.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
}

func TestUpdateCartItemUnknown(t *testing.T) {
	shop := newShop(t, 10)

	_, err := shop.carts.UpdateItem(context.Background(), shop.userID, 99999,
		&dto.UpdateCartItemRequest{Quantity: 1})
	if !errors.Is(err, services.ErrCartItemNotFound) {
		t.Fatalf("err = %v, want ErrCartItemNotFound", err)
	}
}

func TestRemoveFromCart(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	added, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := shop.carts.RemoveItem(ctx, shop.userID, added.CartItems[0].ID); err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}

	cart, err := shop.carts.Get(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(cart.CartItems) != 0 {
		t.Errorf("got %d items, want the cart emptied", len(cart.CartItems))
	}
}

// Delete reports no error for a line that was never there, which the reference
// returns as a 200.
func TestRemoveFromCartUnknown(t *testing.T) {
	shop := newShop(t, 10)

	if err := shop.carts.RemoveItem(context.Background(), shop.userID, 99999); !errors.Is(err, services.ErrCartItemNotFound) {
		t.Fatalf("err = %v, want ErrCartItemNotFound", err)
	}
}

// otherUser creates a second customer against the same database.
//
// Inserted directly rather than registered: these tests are about who may touch
// whose cart, and a second row is all that takes.
func (f *shopFixture) otherUser(t *testing.T, email string) uint {
	t.Helper()

	user := models.User{
		Email:     email,
		Password:  "not-a-real-hash",
		FirstName: "Other",
		LastName:  "User",
		Role:      models.UserRoleCustomer,
		IsActive:  true,
	}

	if err := f.db.Create(&user).Error; err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}

	return user.ID
}

// One customer must not be able to touch another's cart by guessing a line id.
func TestCartItemsAreScopedToTheirOwner(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	added, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	itemID := added.CartItems[0].ID
	intruder := shop.otherUser(t, "intruder@example.com")

	if _, err := shop.carts.UpdateItem(ctx, intruder, itemID,
		&dto.UpdateCartItemRequest{Quantity: 99}); !errors.Is(err, services.ErrCartItemNotFound) {
		t.Errorf("UpdateItem err = %v, want ErrCartItemNotFound for someone else's line", err)
	}

	if err := shop.carts.RemoveItem(ctx, intruder, itemID); !errors.Is(err, services.ErrCartItemNotFound) {
		t.Errorf("RemoveItem err = %v, want ErrCartItemNotFound for someone else's line", err)
	}

	// And the owner's line must be untouched by either attempt.
	cart, err := shop.carts.Get(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(cart.CartItems) != 1 || cart.CartItems[0].Quantity != 2 {
		t.Error("the owner's cart was modified by another user")
	}
}
