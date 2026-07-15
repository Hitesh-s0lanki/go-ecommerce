package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/storage"
)

func newLocal(t *testing.T, baseURL string) (local *storage.Local, dir string) {
	t.Helper()

	dir = t.TempDir()

	local, err := storage.NewLocal(dir, baseURL)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	return local, dir
}

func TestLocalUpload(t *testing.T) {
	local, dir := newLocal(t, "")
	ctx := context.Background()

	url, err := local.Upload(ctx, "products/1/pic.png", strings.NewReader("image-bytes"), 11, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if url != "/uploads/products/1/pic.png" {
		t.Errorf("url = %q, want /uploads/products/1/pic.png", url)
	}

	// The bytes must actually be on disk, in the nested directory.
	got, err := os.ReadFile(filepath.Join(dir, "products", "1", "pic.png"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "image-bytes" {
		t.Errorf("contents = %q, want image-bytes", got)
	}
}

func TestLocalUploadUsesBaseURL(t *testing.T) {
	local, _ := newLocal(t, "https://cdn.example.com/")
	ctx := context.Background()

	url, err := local.Upload(ctx, "products/1/pic.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// The trailing slash on the configured base must not produce a double one.
	if url != "https://cdn.example.com/products/1/pic.png" {
		t.Errorf("url = %q, want the CDN base with a single slash", url)
	}
}

// The reference joins the caller's filename straight onto the base path, so a
// name of "../../../etc/cron.d/root" is written wherever it points. Keys are
// built internally here, but a check that only holds while every caller behaves
// is not a check.
func TestLocalUploadRejectsTraversal(t *testing.T) {
	local, dir := newLocal(t, "")
	ctx := context.Background()

	keys := []string{
		"../escaped.png",
		"products/../../escaped.png",
		"/../../escaped.png",
		"",
		"/",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			_, err := local.Upload(ctx, key, strings.NewReader("pwned"), 5, "image/png")
			if !errors.Is(err, storage.ErrUnsafeKey) {
				t.Fatalf("err = %v, want ErrUnsafeKey", err)
			}
		})
	}

	// And nothing may have been written above the base directory.
	parent := filepath.Dir(dir)

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}

	for _, e := range entries {
		if e.Name() == "escaped.png" {
			t.Fatal("a file escaped the upload directory")
		}
	}
}

// A key is meant to be unique, so a collision means something is wrong;
// overwriting one product's image with another's is the worse answer.
func TestLocalUploadRefusesOverwrite(t *testing.T) {
	local, _ := newLocal(t, "")
	ctx := context.Background()

	if _, err := local.Upload(ctx, "a.png", strings.NewReader("first"), 5, "image/png"); err != nil {
		t.Fatalf("first Upload: %v", err)
	}

	if _, err := local.Upload(ctx, "a.png", strings.NewReader("second"), 6, "image/png"); err == nil {
		t.Fatal("second Upload succeeded, want it to refuse to overwrite")
	}
}

func TestLocalDelete(t *testing.T) {
	local, dir := newLocal(t, "")
	ctx := context.Background()

	if _, err := local.Upload(ctx, "products/1/pic.png", strings.NewReader("x"), 1, "image/png"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if err := local.Delete(ctx, "products/1/pic.png"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "products", "1", "pic.png")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file still exists after Delete (err = %v)", err)
	}
}

// The caller wanted it gone and it is gone: an absent key is not a failure.
func TestLocalDeleteMissingKey(t *testing.T) {
	local, _ := newLocal(t, "")

	if err := local.Delete(context.Background(), "products/1/never-existed.png"); err != nil {
		t.Errorf("Delete: %v, want deleting an absent key to succeed", err)
	}
}

func TestLocalDeleteRejectsTraversal(t *testing.T) {
	local, _ := newLocal(t, "")

	if err := local.Delete(context.Background(), "../../etc/passwd"); !errors.Is(err, storage.ErrUnsafeKey) {
		t.Fatalf("err = %v, want ErrUnsafeKey", err)
	}
}
