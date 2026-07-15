package config

import (
	"errors"
	"testing"
	"time"
)

// isolate points .env loading at an empty dir so a developer's real .env can't
// influence results, and clears the vars Load reads.
func isolate(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())

	for _, k := range []string{
		"PORT", "GIN_MODE",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"JWT_SECRET", "JWT_EXPIRES_IN", "REFRESH_TOKEN_EXPIRES_IN",
		"AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
		"AWS_S3_BUCKET", "AWS_S3_ENDPOINT",
		"UPLOAD_PATH", "MAX_UPLOAD_SIZE",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Server.Port, "8080"; got != want {
		t.Errorf("Port = %q, want %q", got, want)
	}
	if got, want := cfg.JWT.ExpiresIn, 24*time.Hour; got != want {
		t.Errorf("JWT.ExpiresIn = %v, want %v", got, want)
	}
	if got, want := cfg.Upload.MaxFileSize, int64(10<<20); got != want {
		t.Errorf("Upload.MaxFileSize = %d, want %d", got, want)
	}
}

func TestLoadReadsEnv(t *testing.T) {
	isolate(t)
	t.Setenv("PORT", "9999")
	t.Setenv("JWT_EXPIRES_IN", "90m")
	t.Setenv("MAX_UPLOAD_SIZE", "1024")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Server.Port, "9999"; got != want {
		t.Errorf("Port = %q, want %q", got, want)
	}
	if got, want := cfg.JWT.ExpiresIn, 90*time.Minute; got != want {
		t.Errorf("JWT.ExpiresIn = %v, want %v", got, want)
	}
	if got, want := cfg.Upload.MaxFileSize, int64(1024); got != want {
		t.Errorf("Upload.MaxFileSize = %d, want %d", got, want)
	}
}

// A malformed value must fail loudly rather than default to zero.
func TestLoadRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"bad duration", "JWT_EXPIRES_IN", "24 hours"},
		{"bad refresh duration", "REFRESH_TOKEN_EXPIRES_IN", "forever"},
		{"bad size", "MAX_UPLOAD_SIZE", "10MB"},
		{"negative size", "MAX_UPLOAD_SIZE", "-1"},
		{"unknown gin mode", "GIN_MODE", "production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			t.Setenv(tt.key, tt.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q: want error, got nil", tt.key, tt.value)
			}
		})
	}
}

func TestLoadRejectsDefaultSecretInRelease(t *testing.T) {
	isolate(t)
	t.Setenv("GIN_MODE", "release")

	_, err := Load()
	if !errors.Is(err, ErrInsecureJWTSecret) {
		t.Fatalf("Load() error = %v, want ErrInsecureJWTSecret", err)
	}
}

func TestLoadAllowsRealSecretInRelease(t *testing.T) {
	isolate(t)
	t.Setenv("GIN_MODE", "release")
	t.Setenv("JWT_SECRET", "a-real-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
}

func TestDatabaseDSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host: "db", Port: "5432", User: "u", Password: "p",
		Name: "shop", SSLMode: "disable",
	}

	want := "host=db user=u password=p dbname=shop port=5432 sslmode=disable TimeZone=UTC"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}
