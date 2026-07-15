package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/dto"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// The password must be absent by construction, not by remembering a tag.
func TestUserResponseHasNoPassword(t *testing.T) {
	user := &models.User{
		ID: 1, Email: "a@example.com", Password: "super-secret-hash",
		FirstName: "A", LastName: "B", Role: models.UserRoleAdmin, IsActive: true,
	}

	body, err := json.Marshal(dto.NewUserResponse(user))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"password", "Password"} {
		if _, ok := fields[key]; ok {
			t.Errorf("%q present in UserResponse: %s", key, body)
		}
	}

	if got := fields["role"]; got != "admin" {
		t.Errorf("role = %v, want admin", got)
	}
}

// An unloaded relation must be absent, not an object of zero values a client
// cannot distinguish from real data.
func TestProductResponseOmitsUnloadedRelations(t *testing.T) {
	product := &models.Product{ID: 1, CategoryID: 2, Name: "Widget", PriceCents: 1999, SKU: "W-1"}

	body, err := json.Marshal(dto.NewProductResponse(product))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := fields["category"]; ok {
		t.Errorf("category present for an unloaded relation: %s", body)
	}
	if _, ok := fields["images"]; ok {
		t.Errorf("images present when there are none: %s", body)
	}
	if got := fields["price_cents"]; got != float64(1999) {
		t.Errorf("price_cents = %v, want 1999", got)
	}
}

func TestProductResponseIncludesLoadedRelations(t *testing.T) {
	product := &models.Product{
		ID: 1, CategoryID: 2, Name: "Widget", PriceCents: 1999, SKU: "W-1",
		Category: &models.Category{ID: 2, Name: "Widgets", IsActive: true},
		Images: []models.ProductImage{
			{ID: 7, URL: "https://cdn.example/a.png", IsPrimary: true},
		},
	}

	resp := dto.NewProductResponse(product)

	if resp.Category == nil {
		t.Fatal("Category is nil, want the loaded relation")
	}
	if resp.Category.Name != "Widgets" {
		t.Errorf("Category.Name = %q, want Widgets", resp.Category.Name)
	}
	if len(resp.Images) != 1 || resp.Images[0].ID != 7 {
		t.Errorf("Images = %+v, want the one loaded image", resp.Images)
	}
}

// A cart is priced from the product's current price, so the total must follow
// a price change.
func TestCartResponseSumsFromCurrentPrices(t *testing.T) {
	cart := &models.Cart{
		ID: 1, UserID: 2, UpdatedAt: time.Now(),
		CartItems: []models.CartItem{
			{ID: 1, Quantity: 2, Product: &models.Product{ID: 1, PriceCents: 1000}},
			{ID: 2, Quantity: 3, Product: &models.Product{ID: 2, PriceCents: 250}},
		},
	}

	resp := dto.NewCartResponse(cart)

	if want := int64(2*1000 + 3*250); resp.TotalCents != want {
		t.Errorf("TotalCents = %d, want %d", resp.TotalCents, want)
	}
	if want := 5; resp.ItemCount != want {
		t.Errorf("ItemCount = %d, want %d", resp.ItemCount, want)
	}
}

// An empty cart must serialise as [] rather than null, so clients can iterate
// without a nil check.
func TestEmptyCartSerialisesAsEmptyArray(t *testing.T) {
	body, err := json.Marshal(dto.NewCartResponse(&models.Cart{ID: 1, UserID: 2}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := string(fields["cart_items"]); got != "[]" {
		t.Errorf("cart_items = %s, want []", got)
	}
}

// An order line keeps the price recorded at purchase, even after the product's
// price changes — that is the whole point of the snapshot.
func TestOrderItemUsesRecordedPrice(t *testing.T) {
	order := &models.Order{
		ID: 1, UserID: 2, Status: models.OrderStatusConfirmed, TotalAmountCents: 3998,
		OrderItems: []models.OrderItem{
			{
				ID: 1, Quantity: 2, UnitPriceCents: 1999,
				// The product has since become more expensive.
				Product: &models.Product{ID: 1, PriceCents: 2999},
			},
		},
	}

	resp := dto.NewOrderResponse(order)

	if len(resp.OrderItems) != 1 {
		t.Fatalf("got %d order items, want 1", len(resp.OrderItems))
	}

	item := resp.OrderItems[0]
	if item.UnitPriceCents != 1999 {
		t.Errorf("UnitPriceCents = %d, want 1999 (the price paid, not the current one)", item.UnitPriceCents)
	}
	if item.SubtotalCents != 3998 {
		t.Errorf("SubtotalCents = %d, want 3998", item.SubtotalCents)
	}
	if resp.Status != "confirmed" {
		t.Errorf("Status = %q, want confirmed", resp.Status)
	}
}

// CreatedAt must be a real timestamp, so it marshals as RFC 3339.
func TestOrderResponseTimestampFormat(t *testing.T) {
	created := time.Date(2026, 7, 15, 10, 30, 0, 0, time.UTC)

	body, err := json.Marshal(dto.NewOrderResponse(&models.Order{ID: 1, CreatedAt: created}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got, want := fields["created_at"], "2026-07-15T10:30:00Z"; got != want {
		t.Errorf("created_at = %v, want %v", got, want)
	}
}
