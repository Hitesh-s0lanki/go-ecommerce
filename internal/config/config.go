// Package config loads application configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the fully resolved application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	AWS      AWSConfig
	Upload   UploadConfig
	Events   EventsConfig
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Port    string
	GinMode string
	// AllowedOrigins are the CORS origins to accept. A single "*" allows any
	// origin, which is why credentials are never echoed alongside it.
	AllowedOrigins []string
}

// AllowsAnyOrigin reports whether CORS is wide open.
func (c *ServerConfig) AllowsAnyOrigin() bool {
	return len(c.AllowedOrigins) == 1 && c.AllowedOrigins[0] == "*"
}

// DatabaseConfig configures the Postgres connection.
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DSN renders the connection string for the Postgres driver.
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		c.Host, c.User, c.Password, c.Name, c.Port, c.SSLMode,
	)
}

// DefaultJWTSecret is the development placeholder. It is long enough to
// satisfy HS256's key-length requirement so local runs work out of the box,
// and is rejected outright in release mode.
const DefaultJWTSecret = "change_me_in_production_do_not_use"

// JWTConfig configures token signing and lifetimes.
type JWTConfig struct {
	Secret              string
	ExpiresIn           time.Duration
	RefreshTokenExpires time.Duration
}

// UsesDefaultSecret reports whether the JWT secret is still the placeholder.
func (c *JWTConfig) UsesDefaultSecret() bool {
	return c.Secret == DefaultJWTSecret
}

// AWSConfig configures the AWS (or LocalStack) endpoints.
type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	S3Bucket        string
	S3Endpoint      string
	// SQSEndpoint is separate from S3Endpoint. They are the same host under
	// LocalStack, which is why the reference gets away with feeding the S3
	// endpoint to its SQS client, but they are different services and in any
	// real deployment different addresses.
	SQSEndpoint string
}

// EventsConfig configures domain event publishing.
type EventsConfig struct {
	// Enabled false discards every event, so the API runs with no queue.
	Enabled bool
	// QueueName must already exist: see events.NewSQS.
	QueueName string
}

// Upload provider names.
const (
	// UploadProviderLocal writes to the filesystem under UploadConfig.Path.
	UploadProviderLocal = "local"
	// UploadProviderS3 writes to the bucket in AWSConfig.
	UploadProviderS3 = "s3"
)

// UploadConfig configures file uploads.
type UploadConfig struct {
	// Provider is "local" or "s3". An unknown value is rejected at startup
	// rather than quietly falling back to local: silently writing to a disk
	// that no CDN serves, on a box that will be replaced, loses the files.
	Provider string
	// Path is the directory the local provider writes to. Unused for s3.
	Path string
	// PublicBaseURL is prepended to a stored key to build the URL clients
	// fetch. Empty means "serve local uploads from this API" — fine in
	// development, but in production this should point at the CDN or bucket
	// in front of the files.
	PublicBaseURL string
	MaxFileSize   int64
}

// UsesLocalProvider reports whether uploads are written to the filesystem.
func (c *UploadConfig) UsesLocalProvider() bool {
	return c.Provider == UploadProviderLocal
}

// IsProduction reports whether the server is running in release mode.
func (c *Config) IsProduction() bool {
	return c.Server.GinMode == "release"
}

// Load reads configuration from .env (if present) and the environment.
// Real environment variables win over .env.
//
// Every value is validated: a malformed duration or size is returned as an
// error rather than silently falling back to a zero value.
func Load() (*Config, error) {
	// A missing .env is fine — deployed environments set real env vars.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	jwtExpiresIn, err := getEnvDuration("JWT_EXPIRES_IN", 24*time.Hour)
	if err != nil {
		return nil, err
	}

	refreshExpires, err := getEnvDuration("REFRESH_TOKEN_EXPIRES_IN", 72*time.Hour)
	if err != nil {
		return nil, err
	}

	maxUploadSize, err := getEnvInt64("MAX_UPLOAD_SIZE", 10<<20) // 10 MiB
	if err != nil {
		return nil, err
	}

	eventsEnabled, err := getEnvBool("EVENTS_ENABLED", true)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:           getEnv("PORT", "8080"),
			GinMode:        getEnv("GIN_MODE", "debug"),
			AllowedOrigins: getEnvList("ALLOWED_ORIGINS", []string{"*"}),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "password"),
			Name:     getEnv("DB_NAME", "ecommerce_shop"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		JWT: JWTConfig{
			Secret:              getEnv("JWT_SECRET", DefaultJWTSecret),
			ExpiresIn:           jwtExpiresIn,
			RefreshTokenExpires: refreshExpires,
		},
		AWS: AWSConfig{
			Region:          getEnv("AWS_REGION", "us-east-1"),
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", "test"),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", "test"),
			S3Bucket:        getEnv("AWS_S3_BUCKET", "ecommerce-uploads"),
			S3Endpoint:      getEnv("AWS_S3_ENDPOINT", "http://localhost:4566"),
			SQSEndpoint:     getEnv("AWS_SQS_ENDPOINT", "http://localhost:4566"),
		},
		Upload: UploadConfig{
			Provider:      getEnv("UPLOAD_PROVIDER", UploadProviderLocal),
			Path:          getEnv("UPLOAD_PATH", "./uploads"),
			PublicBaseURL: strings.TrimSuffix(getEnv("UPLOAD_PUBLIC_BASE_URL", ""), "/"),
			MaxFileSize:   maxUploadSize,
		},
	}

	cfg.Events = EventsConfig{
		Enabled:   eventsEnabled,
		QueueName: getEnv("AWS_EVENT_QUEUE_NAME", "ecommerce-events"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

var validGinModes = map[string]bool{"debug": true, "release": true, "test": true}

// ErrInsecureJWTSecret is returned when the default JWT secret is left in place
// in release mode.
var ErrInsecureJWTSecret = errors.New("JWT_SECRET must be set to a non-default value in release mode")

func (c *Config) validate() error {
	if !validGinModes[c.Server.GinMode] {
		return fmt.Errorf("GIN_MODE must be debug, release or test, got %q", c.Server.GinMode)
	}

	// Catching this at startup beats discovering it in production.
	if c.IsProduction() && c.JWT.UsesDefaultSecret() {
		return ErrInsecureJWTSecret
	}

	if c.Upload.MaxFileSize <= 0 {
		return fmt.Errorf("MAX_UPLOAD_SIZE must be positive, got %d", c.Upload.MaxFileSize)
	}

	// A publisher with no queue to publish to would fail on every event.
	if c.Events.Enabled && c.Events.QueueName == "" {
		return errors.New("AWS_EVENT_QUEUE_NAME must be set when EVENTS_ENABLED is true")
	}

	if !validUploadProviders[c.Upload.Provider] {
		return fmt.Errorf("UPLOAD_PROVIDER must be %q or %q, got %q",
			UploadProviderLocal, UploadProviderS3, c.Upload.Provider)
	}

	// A bucket name is what makes an s3 upload land somewhere; an empty one
	// fails on the first request rather than at startup.
	if c.Upload.Provider == UploadProviderS3 && c.AWS.S3Bucket == "" {
		return errors.New("AWS_S3_BUCKET must be set when UPLOAD_PROVIDER is s3")
	}

	return nil
}

var validUploadProviders = map[string]bool{
	UploadProviderLocal: true,
	UploadProviderS3:    true,
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvList reads a comma-separated list, trimming spaces and dropping empty
// entries so "a, b," yields exactly ["a", "b"].
func getEnvList(key string, fallback []string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	var out []string

	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	if len(out) == 0 {
		return fallback
	}

	return out
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, raw, err)
	}

	return d, nil
}

// getEnvBool reads a boolean. An unparseable value is an error rather than a
// silent false: "EVENTS_ENABLED=yes" meaning "off" is the kind of thing found
// weeks later.
func getEnvBool(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q: %w", key, raw, err)
	}

	return v, nil
}

func getEnvInt64(key string, fallback int64) (int64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q: %w", key, raw, err)
	}

	return n, nil
}
