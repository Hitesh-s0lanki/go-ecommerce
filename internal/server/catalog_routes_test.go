package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

// catalogServer builds the real route tree with no database, plus a token
// manager sharing its secret.
//
// s.db is nil, so these cover only what happens before a handler reaches the
// database: routing, authentication, authorisation, and request validation.
// The service behaviour behind them is covered against real Postgres in
// internal/services.
func catalogServer(t *testing.T) (http.Handler, *auth.TokenManager) {
	t.Helper()

	cfg := testConfig([]string{"*"})
	log := zerolog.New(io.Discard)

	srv, err := server.New(cfg, nil, &log)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	tokens, err := auth.NewTokenManager(&cfg.JWT)
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}

	return srv.Routes(), tokens
}

func tokenFor(t *testing.T, tokens *auth.TokenManager, role string) string {
	t.Helper()

	pair, err := tokens.GenerateTokenPair(1, "user@example.com", role)
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	return "Bearer " + pair.AccessToken
}

func send(t *testing.T, h http.Handler, method, path, body, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader = http.NoBody
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

// Every catalogue write is an admin power. A customer token reaching one would
// let any registered user edit the shop.
func TestCatalogWritesRequireAdmin(t *testing.T) {
	h, tokens := catalogServer(t)

	customer := tokenFor(t, tokens, "customer")

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/categories"},
		{http.MethodPut, "/api/v1/categories/1"},
		{http.MethodDelete, "/api/v1/categories/1"},
		{http.MethodPost, "/api/v1/products"},
		{http.MethodPut, "/api/v1/products/1"},
		{http.MethodDelete, "/api/v1/products/1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			// No token at all.
			if rec := send(t, h, tt.method, tt.path, "{}", ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("anonymous: status = %d, want 401", rec.Code)
			}

			// A valid token for a non-admin.
			if rec := send(t, h, tt.method, tt.path, "{}", customer); rec.Code != http.StatusForbidden {
				t.Errorf("customer: status = %d, want 403", rec.Code)
			}
		})
	}
}

// Browsing must not need an account. These reach the nil database, so the
// status is not the point — 401 would be, and it must not be that.
func TestCatalogReadsArePublic(t *testing.T) {
	h, _ := catalogServer(t)

	for _, path := range []string{"/api/v1/categories", "/api/v1/products", "/api/v1/products/1"} {
		t.Run(path, func(t *testing.T) {
			rec := send(t, h, http.MethodGet, path, "", "")

			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusNotFound {
				t.Errorf("status = %d; the catalogue must be readable without a token", rec.Code)
			}
		})
	}
}

// A collection route registered as "/" answers /api/v1/categories with a
// redirect instead of doing the work — the reference's POST "/" does exactly
// that. The 400 here is the empty body failing validation, which proves the
// request reached the handler.
func TestCatalogCollectionRoutesHaveNoTrailingSlash(t *testing.T) {
	h, tokens := catalogServer(t)

	admin := tokenFor(t, tokens, "admin")

	for _, path := range []string{"/api/v1/categories", "/api/v1/products"} {
		t.Run(path, func(t *testing.T) {
			rec := send(t, h, http.MethodPost, path, "{}", admin)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 from validation — a redirect or 404 means the route is misregistered", rec.Code)
			}
		})
	}
}

// The id is parsed before the handler touches the database, so a junk id is a
// 400 rather than a 500 from a failed query.
func TestCatalogRejectsInvalidID(t *testing.T) {
	h, tokens := catalogServer(t)

	admin := tokenFor(t, tokens, "admin")

	tests := []struct {
		name       string
		method     string
		path       string
		authHeader string
	}{
		{"not a number", http.MethodGet, "/api/v1/products/abc", ""},
		{"zero", http.MethodGet, "/api/v1/products/0", ""},
		{"negative", http.MethodGet, "/api/v1/products/-1", ""},
		{"admin delete", http.MethodDelete, "/api/v1/products/abc", admin},
		{"admin category delete", http.MethodDelete, "/api/v1/categories/abc", admin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := send(t, h, tt.method, tt.path, "", tt.authHeader)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// Pagination is bound, not parsed with a discarded error, so nonsense is
// reported rather than silently turned into the default.
func TestListProductsRejectsBadPagination(t *testing.T) {
	h, _ := catalogServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"limit above the cap", "/api/v1/products?limit=1000000"},
		{"zero limit", "/api/v1/products?limit=0"},
		{"negative page", "/api/v1/products?page=-1"},
		{"non-numeric limit", "/api/v1/products?limit=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := send(t, h, http.MethodGet, tt.path, "", "")

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}
