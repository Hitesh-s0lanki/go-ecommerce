package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/storage"
)

// Upload errors.
var (
	// ErrUnsupportedFileType is returned when the bytes are not an image we
	// accept — whatever the filename claims.
	ErrUnsupportedFileType = errors.New("unsupported file type")
	// ErrFileTooLarge is returned when the file exceeds the configured limit.
	ErrFileTooLarge = errors.New("file too large")
	// ErrEmptyFile is returned for a zero-byte upload.
	ErrEmptyFile = errors.New("file is empty")
)

// sniffLen is what http.DetectContentType reads. It never looks further.
const sniffLen = 512

// imageExtensions maps an accepted media type to the extension we store it
// under.
//
// The extension comes from the detected type, never from the client's filename:
// the name is attacker-controlled, and a mismatch between what a file is called
// and what it contains is how a "profile picture" turns into a served script.
var imageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// StoredFile is the outcome of an upload.
type StoredFile struct {
	// Key identifies the object in the provider, and is what Remove needs.
	Key string
	// URL is where a client fetches it.
	URL string
	// ContentType is the media type detected from the bytes.
	ContentType string
}

// UploadService validates uploads and hands them to a storage provider.
type UploadService struct {
	provider storage.Provider
	maxBytes int64
}

// NewUploadService builds an UploadService.
func NewUploadService(provider storage.Provider, maxBytes int64) *UploadService {
	return &UploadService{provider: provider, maxBytes: maxBytes}
}

// MaxBytes is the largest file this service accepts, so the HTTP layer can cap
// the request body at the same figure rather than keeping its own copy.
func (s *UploadService) MaxBytes() int64 {
	return s.maxBytes
}

// UploadProductImage validates an uploaded image and stores it under the
// product's prefix.
//
// The stored key is a UUID plus the extension implied by the file's own bytes.
// The reference builds the path from file.Filename, which is chosen by the
// caller: "../../../etc/cron.d/root" escapes the upload directory, and two
// products uploading "image.jpg" overwrite each other.
func (s *UploadService) UploadProductImage(ctx context.Context, productID uint, file *multipart.FileHeader) (*StoredFile, error) {
	if file.Size == 0 {
		return nil, ErrEmptyFile
	}

	// Checked before a byte is read: the header is not trustworthy on its own,
	// but it lets an obviously oversized file be refused without streaming it
	// anywhere. The HTTP layer caps the body independently.
	if file.Size > s.maxBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds the %d byte limit", ErrFileTooLarge, file.Size, s.maxBytes)
	}

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload: %w", err)
	}
	defer func() {
		// Nothing was written, so a failure to close a file we only read has
		// nothing to report.
		_ = src.Close()
	}()

	contentType, ext, err := s.detectImage(src)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("products/%d/%s%s", productID, uuid.New().String(), ext)

	// A section over the file rather than the reader detection just consumed:
	// it reads from absolute offset 0, so no rewind is needed, it stops at the
	// declared size however much the body actually holds, and it stays
	// seekable — which is what lets the S3 client sign the payload without
	// buffering the whole image in memory.
	body := io.NewSectionReader(src, 0, file.Size)

	url, err := s.provider.Upload(ctx, key, body, file.Size, contentType)
	if err != nil {
		return nil, fmt.Errorf("store upload: %w", err)
	}

	return &StoredFile{Key: key, URL: url, ContentType: contentType}, nil
}

// Remove deletes a stored file. Used to clean up when the database write that
// should have recorded the file fails.
func (s *UploadService) Remove(ctx context.Context, key string) error {
	return s.provider.Delete(ctx, key)
}

// detectImage identifies the file from its leading bytes.
//
// The reference trusts the filename's extension, so upload.php renamed to
// upload.jpg sails through. Sniffing the content is what makes the check about
// the file rather than about its label.
func (s *UploadService) detectImage(src io.Reader) (contentType, ext string, err error) {
	header := make([]byte, sniffLen)

	n, err := io.ReadFull(src, header)
	// A file shorter than 512 bytes is normal — a tiny PNG is — and is not an
	// error to sniff.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", fmt.Errorf("read upload: %w", err)
	}

	// DetectContentType always returns something; for unrecognised bytes it
	// says application/octet-stream, which is not in the map.
	contentType = http.DetectContentType(header[:n])

	ext, ok := imageExtensions[contentType]
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrUnsupportedFileType, contentType)
	}

	return contentType, ext, nil
}
