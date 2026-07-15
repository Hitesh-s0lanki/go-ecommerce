package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrUnsafeKey is returned when a key would escape the base directory.
var ErrUnsafeKey = errors.New("unsafe storage key")

// Local writes uploads to a directory on this machine's disk.
//
// Suitable for development and for a single box with a persistent volume. It is
// not suitable behind a load balancer: a second instance cannot serve what this
// one wrote, and a redeploy that replaces the disk loses every file.
type Local struct {
	basePath string
	baseURL  string
}

// NewLocal builds a Local provider rooted at basePath.
//
// baseURL is prepended to a key to build the public URL; when empty, the URL is
// the /uploads path this API serves itself.
func NewLocal(basePath, baseURL string) (*Local, error) {
	// Resolved once so every later key can be checked against a real absolute
	// prefix rather than a relative string that ".." can walk out of.
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve upload path: %w", err)
	}

	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create upload path: %w", err)
	}

	return &Local{basePath: abs, baseURL: strings.TrimSuffix(baseURL, "/")}, nil
}

// Upload writes the bytes to basePath/key.
//
// contentType is unused: the filesystem has nowhere to record it, and the
// extension the key carries is what the file server will type it from.
func (l *Local) Upload(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	fullPath, err := l.resolve(key)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	// O_EXCL: a key is meant to be unique, so an existing file means a
	// collision, and overwriting one product's image with another's is worse
	// than failing.
	//
	// 0600: this process both writes and serves these files, so nothing else
	// needs to read them.
	// #nosec G304 -- fullPath comes from resolve, which rejects any key that
	// is not a canonical relative path and re-checks the result is under the
	// base directory.
	dst, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}

	if _, err := io.Copy(dst, r); err != nil {
		_ = dst.Close()
		// A partial file is worse than none: it would be served as a corrupt
		// image forever.
		_ = os.Remove(fullPath)

		return "", fmt.Errorf("write file: %w", err)
	}

	// Closed explicitly rather than deferred: a deferred Close's error is
	// discarded, and on a write that is exactly where a full disk shows up.
	if err := dst.Close(); err != nil {
		_ = os.Remove(fullPath)
		return "", fmt.Errorf("close file: %w", err)
	}

	return l.publicURL(key), nil
}

// Delete removes the file at key. A key that is already gone is a success.
func (l *Local) Delete(_ context.Context, key string) error {
	fullPath, err := l.resolve(key)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove file: %w", err)
	}

	return nil
}

// publicURL builds the URL a client fetches the key from.
func (l *Local) publicURL(key string) string {
	if l.baseURL != "" {
		return l.baseURL + "/" + key
	}

	// Served by this API. Relative, so it keeps working across environments
	// without the host being baked into the database.
	return "/uploads/" + key
}

// resolve turns a key into an absolute path, refusing anything that would land
// outside the base directory.
//
// The reference joins the key straight onto the base path, so a filename of
// "../../../etc/cron.d/root" is written wherever it points. Callers here build
// keys themselves, but a check that only holds while every caller behaves is
// not a check.
//
// A non-canonical key is rejected rather than quietly cleaned. Cleaning would
// contain the traversal — "../x.png" resolves back inside the base — but the
// key is also what the public URL is built from, so the file would be stored at
// one path and advertised at another. A refusal is the honest answer.
func (l *Local) resolve(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: key is empty", ErrUnsafeKey)
	}

	// Keys are slash-separated on every platform, so they are checked as
	// paths, not filepaths — on Windows filepath.Clean would not treat a
	// forward slash as a separator.
	if key != path.Clean(key) ||
		strings.HasPrefix(key, "/") ||
		key == ".." ||
		strings.HasPrefix(key, "../") {
		return "", fmt.Errorf("%w: %q is not a canonical relative path", ErrUnsafeKey, key)
	}

	fullPath := filepath.Join(l.basePath, filepath.FromSlash(key))

	// Belt to the braces above: the result must still be under the base
	// directory. The separator matters — without it, a base of /var/uploads
	// would also accept /var/uploads-evil.
	if !strings.HasPrefix(fullPath, l.basePath+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q escapes the upload directory", ErrUnsafeKey, key)
	}

	return fullPath, nil
}
