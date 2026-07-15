package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

// authTestServer builds a Server plus a router with protected routes, and
// returns a token manager sharing its secret so tests can mint real tokens.
func authTestServer(t *testing.T) (http.Handler, *auth.TokenManager) {
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

	router := gin.New()

	router.GET("/me", srv.Authenticate(), func(c *gin.Context) {
		id, _ := server.CurrentUserID(c)
		email, _ := server.CurrentUserEmail(c)
		role, _ := server.CurrentUserRole(c)
		c.JSON(http.StatusOK, gin.H{"id": id, "email": email, "role": role})
	})

	router.GET("/admin", srv.Authenticate(), srv.RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Deliberately missing Authenticate, to prove RequireAdmin fails closed.
	router.GET("/misconfigured", srv.RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return router, tokens
}

func getWithAuth(t *testing.T, h http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestAuthenticateAcceptsValidAccessToken(t *testing.T) {
	h, tokens := authTestServer(t)

	pair, err := tokens.GenerateTokenPair(7, "user@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	rec := getWithAuth(t, h, "/me", "Bearer "+pair.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}

	if body := rec.Body.String(); body == "" {
		t.Fatal("empty body")
	}
}

// The claims must reach the handler, or every downstream authorisation check
// is reading nothing.
func TestAuthenticateSetsContext(t *testing.T) {
	h, tokens := authTestServer(t)

	pair, err := tokens.GenerateTokenPair(7, "user@example.com", "admin")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	rec := getWithAuth(t, h, "/me", "Bearer "+pair.AccessToken)

	want := `{"email":"user@example.com","id":7,"role":"admin"}`
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// A refresh token is signed with the same secret and lives far longer. It must
// not open a protected route.
func TestAuthenticateRejectsRefreshToken(t *testing.T) {
	h, tokens := authTestServer(t)

	pair, err := tokens.GenerateTokenPair(7, "user@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	rec := getWithAuth(t, h, "/me", "Bearer "+pair.RefreshToken)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — a refresh token must not authenticate", rec.Code)
	}
}

func TestAuthenticateRejectsBadHeaders(t *testing.T) {
	h, tokens := authTestServer(t)

	pair, err := tokens.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	tests := []struct {
		name   string
		header string
	}{
		{"missing", ""},
		{"no scheme", pair.AccessToken},
		{"wrong scheme", "Basic " + pair.AccessToken},
		{"scheme only", "Bearer"},
		{"empty token", "Bearer "},
		{"extra segment", "Bearer " + pair.AccessToken + " extra"},
		{"garbage token", "Bearer not-a-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getWithAuth(t, h, "/me", tt.header)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			// RFC 9110: a 401 must tell the client how to authenticate.
			if got := rec.Header().Get("WWW-Authenticate"); got == "" {
				t.Error("WWW-Authenticate header missing on 401")
			}
		})
	}
}

// RFC 9110 defines the scheme as case-insensitive, and real clients send
// "bearer".
func TestAuthenticateSchemeIsCaseInsensitive(t *testing.T) {
	h, tokens := authTestServer(t)

	pair, err := tokens.GenerateTokenPair(1, "a@example.com", "customer")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "BeArEr"} {
		t.Run(scheme, func(t *testing.T) {
			rec := getWithAuth(t, h, "/me", scheme+" "+pair.AccessToken)
			if rec.Code != http.StatusOK {
				t.Errorf("scheme %q: status = %d, want 200", scheme, rec.Code)
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	h, tokens := authTestServer(t)

	tests := []struct {
		name string
		role string
		want int
	}{
		{"admin allowed", "admin", http.StatusOK},
		{"customer forbidden", "customer", http.StatusForbidden},
		{"unknown role forbidden", "superuser", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair, err := tokens.GenerateTokenPair(1, "a@example.com", tt.role)
			if err != nil {
				t.Fatalf("GenerateTokenPair: %v", err)
			}

			rec := getWithAuth(t, h, "/admin", "Bearer "+pair.AccessToken)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRequireAdminNeedsAuthentication(t *testing.T) {
	h, _ := authTestServer(t)

	rec := getWithAuth(t, h, "/admin", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// A route wired with RequireAdmin but no Authenticate is a routing mistake. It
// must fail closed rather than admit the request.
func TestRequireAdminFailsClosedWithoutAuthenticate(t *testing.T) {
	h, _ := authTestServer(t)

	rec := getWithAuth(t, h, "/misconfigured", "")
	if rec.Code == http.StatusOK {
		t.Fatal("misconfigured admin route allowed the request through")
	}
}

// Weak secrets must stop the server being built at all.
func TestServerNewRejectsWeakSecret(t *testing.T) {
	cfg := testConfig([]string{"*"})
	cfg.JWT.Secret = "short"

	log := zerolog.New(io.Discard)

	if _, err := server.New(cfg, nil, &log); err == nil {
		t.Fatal("server.New accepted a weak JWT secret, want an error")
	}
}
