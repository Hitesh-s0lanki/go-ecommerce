package server_test

import (
	"net/http"
	"testing"
)

// A cart and an order belong to whoever is holding the token. None of these
// routes may answer an anonymous caller.
func TestCartAndOrderRoutesRequireAuth(t *testing.T) {
	h, _ := catalogServer(t)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/cart", ""},
		{http.MethodPost, "/api/v1/cart/items", `{"product_id":1,"quantity":1}`},
		{http.MethodPut, "/api/v1/cart/items/1", `{"quantity":1}`},
		{http.MethodDelete, "/api/v1/cart/items/1", ""},
		{http.MethodPost, "/api/v1/orders", ""},
		{http.MethodGet, "/api/v1/orders", ""},
		{http.MethodGet, "/api/v1/orders/1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := send(t, h, tt.method, tt.path, tt.body, "")

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// Shopping is not an admin power: a customer token must reach these.
func TestCartAndOrderRoutesAreOpenToCustomers(t *testing.T) {
	h, tokens := catalogServer(t)
	customer := tokenFor(t, tokens, "customer")

	// These reach the nil database, so the status is not the point — 403 is,
	// and it must not be that.
	for _, path := range []string{"/api/v1/cart", "/api/v1/orders"} {
		t.Run(path, func(t *testing.T) {
			rec := send(t, h, http.MethodGet, path, "", customer)

			if rec.Code == http.StatusForbidden {
				t.Errorf("status = 403; a customer must be able to use their own %s", path)
			}
		})
	}
}

// The id is parsed before the handler touches the database.
func TestCartAndOrderRoutesRejectInvalidID(t *testing.T) {
	h, tokens := catalogServer(t)
	customer := tokenFor(t, tokens, "customer")

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"cart item not a number", http.MethodDelete, "/api/v1/cart/items/abc", ""},
		{"cart item zero", http.MethodDelete, "/api/v1/cart/items/0", ""},
		{"order not a number", http.MethodGet, "/api/v1/orders/abc", ""},
		{"order zero", http.MethodGet, "/api/v1/orders/0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := send(t, h, tt.method, tt.path, tt.body, customer)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// A quantity of zero or less is not a way to remove a line — DELETE is.
func TestAddToCartRejectsBadQuantity(t *testing.T) {
	h, tokens := catalogServer(t)
	customer := tokenFor(t, tokens, "customer")

	tests := []struct {
		name string
		body string
	}{
		{"zero quantity", `{"product_id":1,"quantity":0}`},
		{"negative quantity", `{"product_id":1,"quantity":-5}`},
		{"missing product", `{"quantity":1}`},
		{"empty body", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := send(t, h, http.MethodPost, "/api/v1/cart/items", tt.body, customer)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// The collection routes must not need a trailing slash: the 400 here is the
// pagination binding, which proves the request reached the handler rather than
// a redirect.
func TestOrderCollectionRouteHasNoTrailingSlash(t *testing.T) {
	h, tokens := catalogServer(t)
	customer := tokenFor(t, tokens, "customer")

	rec := send(t, h, http.MethodGet, "/api/v1/orders?limit=99999", "", customer)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 from pagination validation", rec.Code)
	}
}
