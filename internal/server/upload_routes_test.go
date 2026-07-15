package server_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

// uploadServer builds the route tree with a local provider rooted in a temp
// directory, so an upload that gets as far as writing bytes writes them
// somewhere the test owns rather than into the repository.
func uploadServer(t *testing.T, maxUploadSize int64) (http.Handler, *auth.TokenManager) {
	t.Helper()

	cfg := testConfig([]string{"*"})
	cfg.Upload = config.UploadConfig{
		Provider:    config.UploadProviderLocal,
		Path:        t.TempDir(),
		MaxFileSize: maxUploadSize,
	}

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

// multipartImage builds a multipart body with the given file contents.
func multipartImage(t *testing.T, field, filename string, content []byte) (body *bytes.Buffer, contentType string) {
	t.Helper()

	body = &bytes.Buffer{}
	w := multipart.NewWriter(body)

	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}

	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	return body, w.FormDataContentType()
}

func postUpload(t *testing.T, h http.Handler, path string, body *bytes.Buffer, contentType, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", contentType)

	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func pngContent(n int) []byte {
	b := make([]byte, n)
	copy(b, "\x89PNG\r\n\x1a\n")

	for i := 8; i < n; i++ {
		b[i] = 'x'
	}

	return b
}

// Uploading an image is an admin power, like every other catalogue write.
func TestUploadProductImageRequiresAdmin(t *testing.T) {
	h, tokens := uploadServer(t, 1<<20)

	body, ct := multipartImage(t, "image", "a.png", pngContent(600))
	if rec := postUpload(t, h, "/api/v1/products/1/images", body, ct, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: status = %d, want 401", rec.Code)
	}

	body, ct = multipartImage(t, "image", "a.png", pngContent(600))

	customer := tokenFor(t, tokens, "customer")
	if rec := postUpload(t, h, "/api/v1/products/1/images", body, ct, customer); rec.Code != http.StatusForbidden {
		t.Errorf("customer: status = %d, want 403", rec.Code)
	}
}

// The body is capped before it is buffered, so an oversized upload is refused
// rather than read into memory. The reference never enforces MAX_UPLOAD_SIZE.
func TestUploadProductImageTooLarge(t *testing.T) {
	// A tiny limit, so the multipart overhead allowance is still exceeded.
	h, tokens := uploadServer(t, 1024)
	admin := tokenFor(t, tokens, "admin")

	body, ct := multipartImage(t, "image", "big.png", pngContent(3<<20))

	rec := postUpload(t, h, "/api/v1/products/1/images", body, ct, admin)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (body %q)", rec.Code, rec.Body.String())
	}
}

// The name says image, the bytes say script. The bytes win.
func TestUploadProductImageRejectsNonImage(t *testing.T) {
	h, tokens := uploadServer(t, 1<<20)
	admin := tokenFor(t, tokens, "admin")

	body, ct := multipartImage(t, "image", "innocent.jpg", []byte("<html><script>alert(1)</script></html>"))

	rec := postUpload(t, h, "/api/v1/products/1/images", body, ct, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
}

func TestUploadProductImageMissingFile(t *testing.T) {
	h, tokens := uploadServer(t, 1<<20)
	admin := tokenFor(t, tokens, "admin")

	// Right shape, wrong field name.
	body, ct := multipartImage(t, "not_image", "a.png", pngContent(600))

	rec := postUpload(t, h, "/api/v1/products/1/images", body, ct, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
}

func TestUploadProductImageInvalidID(t *testing.T) {
	h, tokens := uploadServer(t, 1<<20)
	admin := tokenFor(t, tokens, "admin")

	body, ct := multipartImage(t, "image", "a.png", pngContent(600))

	rec := postUpload(t, h, "/api/v1/products/abc/images", body, ct, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Uploaded bytes are served from this API's own origin under the local
// provider, so a file a browser decides is HTML would run with the site's
// cookies. nosniff is what stops it re-typing an image.
func TestUploadsAreServedWithSniffingDisabled(t *testing.T) {
	h, _ := uploadServer(t, 1<<20)

	rec := do(t, h, http.MethodGet, "/uploads/nothing-here.png", nil)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("Content-Security-Policy is empty, want a restrictive policy on user-supplied files")
	}
}

// With s3 the files live in a bucket, so serving /uploads off this box would
// answer every request with a 404 dressed up as a route.
func TestUploadsAreNotServedForS3Provider(t *testing.T) {
	cfg := testConfig([]string{"*"})
	cfg.Upload = config.UploadConfig{
		Provider:    config.UploadProviderS3,
		MaxFileSize: 1 << 20,
	}
	cfg.AWS.S3Bucket = "test-bucket"
	cfg.AWS.S3Endpoint = "http://localhost:4566"

	log := zerolog.New(io.Discard)

	srv, err := server.New(cfg, nil, &log)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	rec := do(t, srv.Routes(), http.MethodGet, "/uploads/anything.png", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — /uploads must not be mounted for s3", rec.Code)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got == "nosniff" {
		t.Error("the local upload route is mounted despite the s3 provider")
	}
}
