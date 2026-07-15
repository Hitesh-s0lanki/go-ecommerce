package services_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

// stockOf reads a product's current stock.
func (f *shopFixture) stockOf(t *testing.T, productID uint) int {
	t.Helper()

	var stock int
	if err := f.db.Model(&models.Product{}).Select("stock").
		Where("id = ?", productID).Scan(&stock).Error; err != nil {
		t.Fatalf("read stock: %v", err)
	}

	return stock
}

func TestCreateOrder(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 3,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	order, err := shop.orders.Create(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if order.ID == 0 {
		t.Error("ID = 0, want the assigned id")
	}
	if order.Status != string(models.OrderStatusPending) {
		t.Errorf("Status = %q, want pending", order.Status)
	}
	if len(order.OrderItems) != 1 {
		t.Fatalf("got %d lines, want 1", len(order.OrderItems))
	}
	if order.OrderItems[0].Quantity != 3 {
		t.Errorf("Quantity = %d, want 3", order.OrderItems[0].Quantity)
	}
	// productReq prices at 1999.
	if order.OrderItems[0].UnitPriceCents != 1999 {
		t.Errorf("UnitPriceCents = %d, want 1999", order.OrderItems[0].UnitPriceCents)
	}
	if order.TotalAmountCents != 3*1999 {
		t.Errorf("TotalAmountCents = %d, want %d", order.TotalAmountCents, 3*1999)
	}

	// Stock came down by exactly what was ordered.
	if got := shop.stockOf(t, shop.productID); got != 7 {
		t.Errorf("stock = %d, want 7", got)
	}

	// And the cart is empty: leaving it would let the same cart be ordered twice.
	cart, err := shop.carts.Get(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Get cart: %v", err)
	}
	if len(cart.CartItems) != 0 {
		t.Errorf("got %d cart items, want the cart emptied by checkout", len(cart.CartItems))
	}
}

// The reference creates the order inside the loop over cart items, so a
// three-line cart writes three orders, each with a running subtotal.
func TestCreateOrderMultipleItemsMakesOneOrder(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	// Two more products, so the cart has three distinct lines.
	var categoryID uint
	if err := shop.db.Model(&models.Product{}).Select("category_id").
		Where("id = ?", shop.productID).Scan(&categoryID).Error; err != nil {
		t.Fatalf("read category: %v", err)
	}

	ids := []uint{shop.productID}

	for _, sku := range []string{"SKU-SECOND", "SKU-THIRD"} {
		req := productReq(categoryID, sku)
		req.Stock = 10

		p, err := shop.products.Create(ctx, req)
		if err != nil {
			t.Fatalf("create %s: %v", sku, err)
		}

		ids = append(ids, p.ID)
	}

	for _, id := range ids {
		if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
			ProductID: id, Quantity: 2,
		}); err != nil {
			t.Fatalf("AddItem: %v", err)
		}
	}

	order, err := shop.orders.Create(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if len(order.OrderItems) != 3 {
		t.Errorf("got %d lines, want all 3 on one order", len(order.OrderItems))
	}
	if order.TotalAmountCents != 3*2*1999 {
		t.Errorf("TotalAmountCents = %d, want %d", order.TotalAmountCents, 3*2*1999)
	}

	// Exactly one order exists.
	var count int64
	if err := shop.db.Model(&models.Order{}).Where("user_id = ?", shop.userID).
		Count(&count).Error; err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d orders, want 1 — a cart is one order, not one per line", count)
	}
}

func TestCreateOrderEmptyCart(t *testing.T) {
	shop := newShop(t, 10)

	if _, err := shop.orders.Create(context.Background(), shop.userID); !errors.Is(err, services.ErrCartEmpty) {
		t.Fatalf("err = %v, want ErrCartEmpty", err)
	}
}

// Stock can fall between adding to the cart and checking out — the cart is not
// a reservation.
func TestCreateOrderInsufficientStock(t *testing.T) {
	shop := newShop(t, 5)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 5,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	// Someone else buys four of them.
	if err := shop.db.Model(&models.Product{}).Where("id = ?", shop.productID).
		UpdateColumn("stock", 1).Error; err != nil {
		t.Fatalf("reduce stock: %v", err)
	}

	_, err := shop.orders.Create(ctx, shop.userID)
	if !errors.Is(err, services.ErrInsufficientStock) {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}

	// The whole checkout rolls back: no order, stock untouched, cart intact.
	var orders int64
	if err := shop.db.Model(&models.Order{}).Count(&orders).Error; err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 0 {
		t.Errorf("got %d orders, want none from a failed checkout", orders)
	}

	if got := shop.stockOf(t, shop.productID); got != 1 {
		t.Errorf("stock = %d, want 1 — a failed checkout must not consume stock", got)
	}

	cart, err := shop.carts.Get(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Get cart: %v", err)
	}
	if len(cart.CartItems) != 1 {
		t.Error("the cart was emptied by a checkout that failed")
	}
}

// A product withdrawn while it sat in the cart must stop the checkout.
func TestCreateOrderInactiveProduct(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := shop.db.Model(&models.Product{}).Where("id = ?", shop.productID).
		UpdateColumn("is_active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	if _, err := shop.orders.Create(ctx, shop.userID); !errors.Is(err, services.ErrProductUnavailable) {
		t.Fatalf("err = %v, want ErrProductUnavailable", err)
	}
}

func TestCreateOrderDeletedProduct(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := shop.products.Delete(ctx, shop.productID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := shop.orders.Create(ctx, shop.userID); !errors.Is(err, services.ErrProductNotFound) {
		t.Fatalf("err = %v, want ErrProductNotFound", err)
	}
}

// The order records what the customer paid. A later price change must not
// rewrite their receipt.
func TestCreateOrderSnapshotsPrice(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 2,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	order, err := shop.orders.Create(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The product doubles in price after the sale.
	if err := shop.db.Model(&models.Product{}).Where("id = ?", shop.productID).
		UpdateColumn("price_cents", 3998).Error; err != nil {
		t.Fatalf("reprice: %v", err)
	}

	reloaded, err := shop.orders.Get(ctx, shop.userID, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if reloaded.OrderItems[0].UnitPriceCents != 1999 {
		t.Errorf("UnitPriceCents = %d, want the 1999 that was paid", reloaded.OrderItems[0].UnitPriceCents)
	}
	if reloaded.TotalAmountCents != 2*1999 {
		t.Errorf("TotalAmountCents = %d, want %d", reloaded.TotalAmountCents, 2*1999)
	}
}

// The point of the row lock.
//
// Two customers check out the last units at the same time. The reference reads
// stock, subtracts in Go, and saves the whole row, so both read 5, both write
// 3, and it sells 4 units out of 5 while reporting 3 left. Here one checkout
// wins and the other is refused, and stock never goes negative.
func TestConcurrentCheckoutsDoNotOversell(t *testing.T) {
	const (
		stock   = 5
		buyers  = 4
		perBuys = 2
	)

	shop := newShop(t, stock)
	ctx := context.Background()

	// Each buyer gets their own account and cart, all wanting the same product.
	userIDs := make([]uint, 0, buyers)
	for i := range buyers {
		id := shop.otherUser(t, fmtEmail(i))
		userIDs = append(userIDs, id)

		if _, err := shop.carts.AddItem(ctx, id, &dto.AddToCartRequest{
			ProductID: shop.productID, Quantity: perBuys,
		}); err != nil {
			t.Fatalf("AddItem for buyer %d: %v", i, err)
		}
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		failed    int
	)

	start := make(chan struct{})

	for _, id := range userIDs {
		wg.Add(1)

		go func(userID uint) {
			defer wg.Done()

			// Released together, to overlap the transactions as much as
			// possible.
			<-start

			_, err := shop.orders.Create(context.Background(), userID)

			mu.Lock()
			defer mu.Unlock()

			if err == nil {
				succeeded++
				return
			}

			if !errors.Is(err, services.ErrInsufficientStock) {
				t.Errorf("unexpected error: %v", err)
			}

			failed++
		}(id)
	}

	close(start)
	wg.Wait()

	// 5 in stock, 2 per order: at most two orders can be filled.
	if succeeded > stock/perBuys {
		t.Errorf("%d checkouts succeeded against stock %d at %d each — that is an oversell",
			succeeded, stock, perBuys)
	}
	if succeeded+failed != buyers {
		t.Errorf("accounted for %d of %d checkouts", succeeded+failed, buyers)
	}

	// The ledger must balance: what is left plus what was sold is what we had.
	remaining := shop.stockOf(t, shop.productID)
	if remaining < 0 {
		t.Fatalf("stock = %d, went negative", remaining)
	}

	if sold := stock - remaining; sold != succeeded*perBuys {
		t.Errorf("stock fell by %d but %d orders of %d were placed", sold, succeeded, perBuys)
	}
}

func fmtEmail(i int) string {
	return "buyer" + string(rune('a'+i)) + "@example.com"
}

func TestGetOrder(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	created, err := shop.orders.Create(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := shop.orders.Get(ctx, shop.userID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID = %d, want %d", got.ID, created.ID)
	}
	// The order renders its products, so they must come back loaded.
	if len(got.OrderItems) != 1 || got.OrderItems[0].Product == nil {
		t.Error("the order's product was not preloaded")
	}
}

func TestGetOrderUnknown(t *testing.T) {
	shop := newShop(t, 10)

	if _, err := shop.orders.Get(context.Background(), shop.userID, 99999); !errors.Is(err, services.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound", err)
	}
}

// Another customer's order must read as missing. Answering 403 would confirm it
// exists; answering it at all would hand over their purchase history.
func TestGetOrderOfAnotherUser(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	created, err := shop.orders.Create(ctx, shop.userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	intruder := shop.otherUser(t, "nosy@example.com")

	if _, err := shop.orders.Get(ctx, intruder, created.ID); !errors.Is(err, services.ErrOrderNotFound) {
		t.Fatalf("err = %v, want ErrOrderNotFound for someone else's order", err)
	}
}

func TestListOrders(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	// Three separate orders, one unit at a time.
	for range 3 {
		if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
			ProductID: shop.productID, Quantity: 1,
		}); err != nil {
			t.Fatalf("AddItem: %v", err)
		}

		if _, err := shop.orders.Create(ctx, shop.userID); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	orders, meta, err := shop.orders.List(ctx, shop.userID, dto.ListQuery{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if meta.Total != 3 {
		t.Errorf("Total = %d, want 3", meta.Total)
	}
	if meta.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", meta.TotalPages)
	}
	if len(orders) != 2 {
		t.Fatalf("got %d orders, want 2", len(orders))
	}
	// Newest first.
	if orders[0].ID < orders[1].ID {
		t.Errorf("orders came back oldest first (%d before %d)", orders[0].ID, orders[1].ID)
	}
}

// A customer's order list must hold only their own.
func TestListOrdersScopedToUser(t *testing.T) {
	shop := newShop(t, 10)
	ctx := context.Background()

	if _, err := shop.carts.AddItem(ctx, shop.userID, &dto.AddToCartRequest{
		ProductID: shop.productID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if _, err := shop.orders.Create(ctx, shop.userID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stranger := shop.otherUser(t, "stranger@example.com")

	orders, meta, err := shop.orders.List(ctx, stranger, dto.ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(orders) != 0 || meta.Total != 0 {
		t.Errorf("got %d orders (total %d) for a user with none", len(orders), meta.Total)
	}
}

func TestListOrdersEmpty(t *testing.T) {
	shop := newShop(t, 10)

	orders, meta, err := shop.orders.List(context.Background(), shop.userID, dto.ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if orders == nil {
		t.Error("orders is nil, want an empty slice")
	}
	if meta.Total != 0 {
		t.Errorf("Total = %d, want 0", meta.Total)
	}
}
