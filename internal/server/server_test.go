package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// newServer builds a Server with no database. Handlers that do not touch the
// database work fine; s.db is nil, so /health/ready is covered separately in
// the integration test.
func newServer(t *testing.T, origins []string) http.Handler {
	t.Helper()

	log := zerolog.New(io.Discard)

	srv, err := server.New(testConfig(origins), nil, &log)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	return srv.Routes()
}

func do(t *testing.T, h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, http.NoBody)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestHealth(t *testing.T) {
	rec := do(t, newServer(t, []string{"*"}), http.MethodGet, "/health", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}

	if !body.Success {
		t.Error("success = false, want true")
	}
	if got := body.Data["status"]; got != "ok" {
		t.Errorf("data.status = %v, want ok", got)
	}
}

// Liveness must not depend on the database: s.db is nil here, and a panic
// would mean a database blip could restart-loop the pod.
func TestHealthDoesNotTouchDatabase(t *testing.T) {
	rec := do(t, newServer(t, []string{"*"}), http.MethodGet, "/health", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNotFoundReturnsEnvelope(t *testing.T) {
	rec := do(t, newServer(t, []string{"*"}), http.MethodGet, "/nope", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Success {
		t.Error("success = true, want false")
	}
	if body.Message != "route not found" {
		t.Errorf("message = %q, want %q", body.Message, "route not found")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, newServer(t, []string{"*"}), http.MethodDelete, "/health", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRequestIDGenerated(t *testing.T) {
	rec := do(t, newServer(t, []string{"*"}), http.MethodGet, "/health", nil)

	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Error("X-Request-ID header is empty, want a generated id")
	}
}

// An inbound correlation id must survive, so logs join up across services.
func TestRequestIDPropagated(t *testing.T) {
	const id = "test-correlation-id"

	rec := do(t, newServer(t, []string{"*"}), http.MethodGet, "/health",
		map[string]string{"X-Request-ID": id})

	if got := rec.Header().Get("X-Request-ID"); got != id {
		t.Errorf("X-Request-ID = %q, want %q (inbound id must be reused)", got, id)
	}
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name            string
		origins         []string
		requestOrigin   string
		wantAllowOrigin string
		wantCredentials string
	}{
		{
			name:            "wildcard allows any origin",
			origins:         []string{"*"},
			requestOrigin:   "https://anything.example",
			wantAllowOrigin: "*",
			// Never credentials alongside "*": browsers reject the pair.
			wantCredentials: "",
		},
		{
			name:            "configured origin is echoed",
			origins:         []string{"https://shop.example"},
			requestOrigin:   "https://shop.example",
			wantAllowOrigin: "https://shop.example",
			wantCredentials: "true",
		},
		{
			name:            "unlisted origin is not allowed",
			origins:         []string{"https://shop.example"},
			requestOrigin:   "https://evil.example",
			wantAllowOrigin: "",
			wantCredentials: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newServer(t, tt.origins), http.MethodGet, "/health",
				map[string]string{"Origin": tt.requestOrigin})

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantAllowOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantAllowOrigin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != tt.wantCredentials {
				t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, tt.wantCredentials)
			}
			// Vary matters even when denied, or a cache may serve one origin's
			// response to another.
			if got := rec.Header().Get("Vary"); got != "Origin" {
				t.Errorf("Vary = %q, want %q", got, "Origin")
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	rec := do(t, newServer(t, []string{"*"}), http.MethodOptions, "/health",
		map[string]string{"Origin": "https://shop.example"})

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
