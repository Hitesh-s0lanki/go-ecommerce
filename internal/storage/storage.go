// Package storage stores uploaded files, on the local filesystem or in S3.
//
// There is no `interfaces` package holding the contract: a package named for a
// layer rather than a thing collects unrelated types and forces every consumer
// to import all of them. Provider lives beside the implementations that satisfy
// it, and the services package depends on this interface rather than on either
// concrete provider.
package storage

import (
	"context"
	"fmt"
	"io"
)

// Provider stores and removes files.
//
// It takes an io.Reader rather than the *multipart.FileHeader the reference
// passes down: where the bytes came from is the HTTP layer's business, and a
// provider that knows about multipart forms cannot be used by anything else —
// or tested without building one.
type Provider interface {
	// Upload stores size bytes read from r under key, and returns the URL a
	// client can fetch it from.
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
	// Delete removes the object at key. Deleting an absent key is not an
	// error: the caller wanted it gone, and it is.
	Delete(ctx context.Context, key string) error
}

// Config is what a provider needs to be built, so this package does not depend
// on the whole application configuration.
type Config struct {
	Provider string
	// Path is the local provider's base directory.
	Path string
	// PublicBaseURL is prepended to a key to build the URL clients fetch.
	PublicBaseURL string
	Region        string
	AccessKeyID   string
	SecretKey     string
	Bucket        string
	// Endpoint points the S3 client somewhere other than AWS, e.g. LocalStack.
	Endpoint string
}

// Provider names, mirroring the config package's.
const (
	providerLocal = "local"
	providerS3    = "s3"
)

// New builds the provider named by cfg.
//
// The choice is made once, at startup, so an unknown provider stops the process
// rather than failing the first upload of the day.
func New(ctx context.Context, cfg *Config) (Provider, error) {
	switch cfg.Provider {
	case providerLocal:
		return NewLocal(cfg.Path, cfg.PublicBaseURL)
	case providerS3:
		return NewS3(ctx, cfg)
	default:
		// config.validate rejects this first; this is the belt to its braces.
		return nil, fmt.Errorf("unknown upload provider %q", cfg.Provider)
	}
}
