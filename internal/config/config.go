// Package config loads application configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Port    string
	GinMode string
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

// JWTConfig configures token signing and lifetimes.
type JWTConfig struct {
	Secret              string
	ExpiresIn           time.Duration
	RefreshTokenExpires time.Duration
}

// AWSConfig configures the AWS (or LocalStack) endpoints.
type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	S3Bucket        string
	S3Endpoint      string
}

// UploadConfig configures file uploads.
type UploadConfig struct {
	Path        string
	MaxFileSize int64
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

	cfg := &Config{
		Server: ServerConfig{
			Port:    getEnv("PORT", "8080"),
			GinMode: getEnv("GIN_MODE", "debug"),
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
			Secret:              getEnv("JWT_SECRET", "change_me_in_production"),
			ExpiresIn:           jwtExpiresIn,
			RefreshTokenExpires: refreshExpires,
		},
		AWS: AWSConfig{
			Region:          getEnv("AWS_REGION", "us-east-1"),
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", "test"),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", "test"),
			S3Bucket:        getEnv("AWS_S3_BUCKET", "ecommerce-uploads"),
			S3Endpoint:      getEnv("AWS_S3_ENDPOINT", "http://localhost:4566"),
		},
		Upload: UploadConfig{
			Path:        getEnv("UPLOAD_PATH", "./uploads"),
			MaxFileSize: maxUploadSize,
		},
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
	if c.IsProduction() && c.JWT.Secret == "change_me_in_production" {
		return ErrInsecureJWTSecret
	}

	if c.Upload.MaxFileSize <= 0 {
		return fmt.Errorf("MAX_UPLOAD_SIZE must be positive, got %d", c.Upload.MaxFileSize)
	}

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
