package server_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

func swaggerServer(t *testing.T, ginMode string) http.Handler {
	t.Helper()

	cfg := testConfig([]string{"*"})
	cfg.Server.GinMode = ginMode

	log := zerolog.New(io.Discard)

	srv, err := server.New(cfg, nil, &log)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	return srv.Routes()
}

// The URL a person actually types is /swagger or /swagger/, not
// /swagger/index.html. Both must land on the docs rather than a 404.
func TestSwaggerEntryPoints(t *testing.T) {
	h := swaggerServer(t, gin.TestMode)

	tests := []struct {
		path string
		want []int
	}{
		{"/swagger/", []int{http.StatusFound}},
		// gin redirects the bare path to /swagger/, which then redirects on.
		{"/swagger", []int{http.StatusMovedPermanently, http.StatusFound}},
		{"/swagger/index.html", []int{http.StatusOK}},
		{"/swagger/doc.json", []int{http.StatusOK}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, tt.path, nil)

			for _, want := range tt.want {
				if rec.Code == want {
					return
				}
			}

			t.Errorf("GET %s = %d, want one of %v", tt.path, rec.Code, tt.want)
		})
	}
}

// Following the redirect must actually reach the page, not loop.
func TestSwaggerRedirectTargetWorks(t *testing.T) {
	h := swaggerServer(t, gin.TestMode)

	rec := do(t, h, http.MethodGet, "/swagger/", nil)

	target := rec.Header().Get("Location")
	if target != "/swagger/index.html" {
		t.Fatalf("Location = %q, want /swagger/index.html", target)
	}

	if got := do(t, h, http.MethodGet, target, nil); got.Code != http.StatusOK {
		t.Errorf("GET %s = %d, want 200", target, got.Code)
	}
}

// The docs describe every endpoint and payload: that is reconnaissance in
// production.
func TestSwaggerDisabledInRelease(t *testing.T) {
	h := swaggerServer(t, gin.ReleaseMode)

	for _, path := range []string{"/swagger/", "/swagger/index.html", "/swagger/doc.json"} {
		if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s in release = %d, want 404", path, rec.Code)
		}
	}
}
