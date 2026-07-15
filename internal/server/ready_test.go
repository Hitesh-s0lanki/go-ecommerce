package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

// The readiness probe is only meaningful against a real pool, so it needs a
// database. Run with `make test-integration`; skips otherwise.
func newServerWithDB(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set; skipping database integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	log := zerolog.New(io.Discard)

	srv, err := server.New(testConfig([]string{"*"}), db, &log)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	return srv.Routes(), db
}

func TestReadyWithDatabase(t *testing.T) {
	h, _ := newServerWithDB(t)

	rec := do(t, h, http.MethodGet, "/health/ready", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got := body.Data["database"]; got != "up" {
		t.Errorf("data.database = %v, want up", got)
	}
}

// The point of readiness: a dead pool must report 503, so the instance is
// pulled from the load balancer rather than serving errors.
func TestReadyFailsWhenDatabaseDown(t *testing.T) {
	h, db := newServerWithDB(t)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/health/ready", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	// A 5xx must not leak the driver's error text to the client.
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["error"]; ok {
		t.Errorf("error key present on 503: %v — internal detail must not leak", body["error"])
	}
}

// Liveness must stay up even when the database is gone, or a database blip
// would restart-loop every pod.
func TestHealthStaysUpWhenDatabaseDown(t *testing.T) {
	h, db := newServerWithDB(t)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — liveness must not depend on the database", rec.Code, http.StatusOK)
	}
}
