package services_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
)

// fakeProvider records what it was asked to store, so the tests can assert on
// the bytes and the key without touching a disk or a bucket.
type fakeProvider struct {
	stored    map[string][]byte
	types     map[string]string
	deleted   []string
	uploadErr error
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{stored: map[string][]byte{}, types: map[string]string{}}
}

func (f *fakeProvider) Upload(_ context.Context, key string, r io.Reader, _ int64, contentType string) (string, error) {
	if f.uploadErr != nil {
		return "", f.uploadErr
	}

	body, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	f.stored[key] = body
	f.types[key] = contentType

	return "/uploads/" + key, nil
}

func (f *fakeProvider) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	delete(f.stored, key)

	return nil
}

// Real magic bytes: the service identifies files by content, so a plausible
// header is the whole point.
var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte("\xff\xd8\xff\xe0")
	gifMagic  = []byte("GIF89a")
)

// webpBytes builds a RIFF/WEBP header, which needs its length field in place.
func webpBytes() []byte {
	b := []byte("RIFF")
	b = append(b, 0, 0, 0, 0)
	b = append(b, "WEBPVP8 "...)

	return b
}

// padded pads content out to n bytes so a test can prove the whole file was
// stored, not just what the sniffer happened to read.
func padded(magic []byte, n int) []byte {
	b := make([]byte, n)
	copy(b, magic)

	for i := len(magic); i < n; i++ {
		b[i] = 'x'
	}

	return b
}

func fileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	var buf bytes.Buffer

	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("image", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}

	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(int64(len(content)) + 1024)
	if err != nil {
		t.Fatalf("ReadForm: %v", err)
	}

	t.Cleanup(func() { _ = form.RemoveAll() })

	return form.File["image"][0]
}

const maxTestBytes = 1 << 20

func TestUploadProductImage(t *testing.T) {
	provider := newFakeProvider()
	uploads := services.NewUploadService(provider, maxTestBytes)

	// Deliberately longer than the 512-byte sniff window: detection consumes
	// the head of the file, so without a rewind only the tail would be stored.
	content := padded(pngMagic, 2000)

	stored, err := uploads.UploadProductImage(context.Background(), 7, fileHeader(t, "pic.png", content))
	if err != nil {
		t.Fatalf("UploadProductImage: %v", err)
	}

	if !strings.HasPrefix(stored.Key, "products/7/") {
		t.Errorf("Key = %q, want it under the product's prefix", stored.Key)
	}
	if !strings.HasSuffix(stored.Key, ".png") {
		t.Errorf("Key = %q, want a .png extension", stored.Key)
	}
	if stored.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", stored.ContentType)
	}
	if stored.URL != "/uploads/"+stored.Key {
		t.Errorf("URL = %q, want the provider's url", stored.URL)
	}

	if got := provider.stored[stored.Key]; !bytes.Equal(got, content) {
		t.Errorf("stored %d bytes, want all %d — the reader must be rewound after sniffing",
			len(got), len(content))
	}
}

// The filename is chosen by the caller. Two products uploading "image.jpg" must
// not collide, and the name must not steer where the file lands.
func TestUploadProductImageKeyIsUnique(t *testing.T) {
	provider := newFakeProvider()
	uploads := services.NewUploadService(provider, maxTestBytes)
	ctx := context.Background()

	first, err := uploads.UploadProductImage(ctx, 1, fileHeader(t, "image.png", padded(pngMagic, 600)))
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	second, err := uploads.UploadProductImage(ctx, 1, fileHeader(t, "image.png", padded(pngMagic, 600)))
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}

	if first.Key == second.Key {
		t.Fatalf("both uploads got key %q; the second would overwrite the first", first.Key)
	}
}

// A path in the filename must not reach the key. The reference interpolates
// file.Filename straight into the storage path.
func TestUploadProductImageIgnoresFilename(t *testing.T) {
	provider := newFakeProvider()
	uploads := services.NewUploadService(provider, maxTestBytes)

	stored, err := uploads.UploadProductImage(context.Background(), 1,
		fileHeader(t, "../../../etc/cron.d/root", padded(pngMagic, 600)))
	if err != nil {
		t.Fatalf("UploadProductImage: %v", err)
	}

	if strings.Contains(stored.Key, "..") || strings.Contains(stored.Key, "cron") {
		t.Errorf("Key = %q, want the caller's filename to have no say in it", stored.Key)
	}
}

func TestUploadProductImageAcceptsImageTypes(t *testing.T) {
	tests := []struct {
		name       string
		content    []byte
		wantType   string
		wantSuffix string
		filename   string
	}{
		{"png", padded(pngMagic, 600), "image/png", ".png", "a.png"},
		{"jpeg", padded(jpegMagic, 600), "image/jpeg", ".jpg", "a.jpg"},
		{"gif", padded(gifMagic, 600), "image/gif", ".gif", "a.gif"},
		{"webp", padded(webpBytes(), 600), "image/webp", ".webp", "a.webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploads := services.NewUploadService(newFakeProvider(), maxTestBytes)

			stored, err := uploads.UploadProductImage(context.Background(), 1, fileHeader(t, tt.filename, tt.content))
			if err != nil {
				t.Fatalf("UploadProductImage: %v", err)
			}

			if stored.ContentType != tt.wantType {
				t.Errorf("ContentType = %q, want %q", stored.ContentType, tt.wantType)
			}
			if !strings.HasSuffix(stored.Key, tt.wantSuffix) {
				t.Errorf("Key = %q, want suffix %q", stored.Key, tt.wantSuffix)
			}
		})
	}
}

// The core of it: the reference trusts the extension, so a script renamed
// .jpg is stored and later served as one.
func TestUploadProductImageRejectsNonImageContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{"html masquerading as jpg", []byte("<html><script>alert(1)</script></html>")},
		{"php masquerading as jpg", []byte("<?php system($_GET['c']); ?>")},
		{"plain text", []byte(strings.Repeat("just text ", 100))},
		{"pdf", []byte("%PDF-1.4 fake pdf content here")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newFakeProvider()
			uploads := services.NewUploadService(provider, maxTestBytes)

			// The name says image; the bytes do not.
			_, err := uploads.UploadProductImage(context.Background(), 1, fileHeader(t, "innocent.jpg", tt.content))
			if !errors.Is(err, services.ErrUnsupportedFileType) {
				t.Fatalf("err = %v, want ErrUnsupportedFileType", err)
			}

			if len(provider.stored) != 0 {
				t.Error("a rejected file was stored anyway")
			}
		})
	}
}

// The other direction: the bytes decide, so an image with a wrong or missing
// extension is accepted and stored under the right one.
func TestUploadProductImageTakesExtensionFromContent(t *testing.T) {
	uploads := services.NewUploadService(newFakeProvider(), maxTestBytes)

	stored, err := uploads.UploadProductImage(context.Background(), 1,
		fileHeader(t, "notes.txt", padded(pngMagic, 600)))
	if err != nil {
		t.Fatalf("UploadProductImage: %v", err)
	}

	if !strings.HasSuffix(stored.Key, ".png") {
		t.Errorf("Key = %q, want .png taken from the content, not .txt from the name", stored.Key)
	}
}

// MAX_UPLOAD_SIZE exists in the reference's config and is never read.
func TestUploadProductImageRejectsOversizedFile(t *testing.T) {
	provider := newFakeProvider()
	uploads := services.NewUploadService(provider, 1000)

	_, err := uploads.UploadProductImage(context.Background(), 1,
		fileHeader(t, "big.png", padded(pngMagic, 5000)))
	if !errors.Is(err, services.ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}

	if len(provider.stored) != 0 {
		t.Error("an oversized file was stored anyway")
	}
}

func TestUploadProductImageRejectsEmptyFile(t *testing.T) {
	uploads := services.NewUploadService(newFakeProvider(), maxTestBytes)

	_, err := uploads.UploadProductImage(context.Background(), 1, fileHeader(t, "empty.png", nil))
	if !errors.Is(err, services.ErrEmptyFile) {
		t.Fatalf("err = %v, want ErrEmptyFile", err)
	}
}

func TestUploadProductImageProviderFailure(t *testing.T) {
	provider := newFakeProvider()
	provider.uploadErr = errors.New("bucket is on fire")

	uploads := services.NewUploadService(provider, maxTestBytes)

	_, err := uploads.UploadProductImage(context.Background(), 1, fileHeader(t, "a.png", padded(pngMagic, 600)))
	if err == nil {
		t.Fatal("err = nil, want the provider's failure to surface")
	}
	// Wrapped, not swallowed: the cause must survive to the log.
	if !strings.Contains(err.Error(), "bucket is on fire") {
		t.Errorf("err = %v, want it to carry the provider's cause", err)
	}
}

func TestUploadServiceRemove(t *testing.T) {
	provider := newFakeProvider()
	uploads := services.NewUploadService(provider, maxTestBytes)
	ctx := context.Background()

	stored, err := uploads.UploadProductImage(ctx, 1, fileHeader(t, "a.png", padded(pngMagic, 600)))
	if err != nil {
		t.Fatalf("UploadProductImage: %v", err)
	}

	if err := uploads.Remove(ctx, stored.Key); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if len(provider.deleted) != 1 || provider.deleted[0] != stored.Key {
		t.Errorf("deleted = %v, want [%s]", provider.deleted, stored.Key)
	}
}
