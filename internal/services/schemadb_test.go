package services_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
)

// newSchemaDB gives a test its own Postgres schema, migrated and dropped
// afterwards, so tests can run in any order without colliding.
//
// The schema is selected through the DSN rather than with `SET search_path`.
// That matters: SET applies to one session, and a *gorm.DB is a pool. A test
// that only ever uses one connection at a time never notices, but the moment
// two goroutines run queries the second gets a different connection — one that
// never ran the SET, and so quietly reads the public schema instead. Passing it
// on the connection string makes every connection in the pool start in the
// right place.
func newSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set; skipping database integration test")
	}

	// A separate connection to create and drop the schema: the pool below
	// cannot create the schema it is already pointed at.
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	schema := fmt.Sprintf("test_svc_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Registered before the pool's cleanup, so it runs after it: t.Cleanup is
	// LIFO, and the schema cannot be dropped while connections are still in it.
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop schema: %v", err)
		}

		closeDB(t, admin)
	})

	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}

	t.Cleanup(func() { closeDB(t, db) })

	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	return db
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Errorf("get sql db: %v", err)
		return
	}

	if err := sqlDB.Close(); err != nil {
		t.Errorf("close db: %v", err)
	}
}
