package models_test

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

// These tests need a real Postgres: they assert on partial indexes and check
// constraints, which no in-memory stand-in reproduces faithfully.
//
// Run them with:
//
//	make test-integration
//
// or set TEST_DATABASE_DSN directly. Without it they skip, so `go test ./...`
// stays green on a machine (or CI) with no database.
func openTestDB(t *testing.T) *gorm.DB {
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

	// Each test gets its own schema, so they can run in any order without
	// colliding and clean up after themselves.
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})

	if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	return db
}

// AutoMigrate covering every model is the baseline: a bad tag fails here.
func TestAutoMigrate(t *testing.T) {
	db := openTestDB(t)

	for _, model := range models.All() {
		if !db.Migrator().HasTable(model) {
			t.Errorf("table for %T was not created", model)
		}
	}
}

// The point of the partial unique index: a live duplicate is rejected, but a
// soft-deleted row releases its email.
func TestUserEmailUniquePartial(t *testing.T) {
	db := openTestDB(t)

	user := models.User{Email: "a@example.com", Password: "x", FirstName: "A", LastName: "B"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create first user: %v", err)
	}

	dup := models.User{Email: "a@example.com", Password: "x", FirstName: "C", LastName: "D"}
	if err := db.Create(&dup).Error; err == nil {
		t.Fatal("creating a duplicate live email: want error, got nil")
	}

	if err := db.Delete(&user).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	reuse := models.User{Email: "a@example.com", Password: "x", FirstName: "E", LastName: "F"}
	if err := db.Create(&reuse).Error; err != nil {
		t.Fatalf("reusing a soft-deleted email should be allowed, got: %v", err)
	}
}

func TestProductCheckConstraints(t *testing.T) {
	db := openTestDB(t)

	category := models.Category{Name: "Widgets"}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}

	tests := []struct {
		name    string
		product models.Product
		wantErr bool
	}{
		{
			name:    "valid",
			product: models.Product{CategoryID: category.ID, Name: "O", SKU: "SKU-1", PriceCents: 1999, Stock: 5},
		},
		{
			name:    "negative price",
			product: models.Product{CategoryID: category.ID, Name: "N", SKU: "SKU-2", PriceCents: -1, Stock: 1},
			wantErr: true,
		},
		{
			name:    "negative stock",
			product: models.Product{CategoryID: category.ID, Name: "S", SKU: "SKU-3", PriceCents: 100, Stock: -1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Create(&tt.product).Error
			if tt.wantErr && err == nil {
				t.Error("want constraint violation, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("want success, got %v", err)
			}
		})
	}
}

// Prices are copied onto the order line, so history survives a price change.
func TestOrderItemPriceIsSnapshot(t *testing.T) {
	db := openTestDB(t)

	category := models.Category{Name: "Widgets"}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}

	product := models.Product{CategoryID: category.ID, Name: "Widget", SKU: "W-1", PriceCents: 1999, Stock: 10}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}

	user := models.User{Email: "buyer@example.com", Password: "x", FirstName: "B", LastName: "C"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	order := models.Order{
		UserID:           user.ID,
		TotalAmountCents: 3998,
		OrderItems: []models.OrderItem{
			{ProductID: product.ID, Quantity: 2, UnitPriceCents: product.PriceCents},
		},
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}

	if err := db.Model(&product).Update("price_cents", 2999).Error; err != nil {
		t.Fatalf("update price: %v", err)
	}

	var got models.Order
	if err := db.Preload("OrderItems").First(&got, order.ID).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}

	if len(got.OrderItems) != 1 {
		t.Fatalf("got %d order items, want 1", len(got.OrderItems))
	}
	if want := int64(1999); got.OrderItems[0].UnitPriceCents != want {
		t.Errorf("UnitPriceCents = %d, want %d (price change must not rewrite history)",
			got.OrderItems[0].UnitPriceCents, want)
	}
	if want := int64(3998); got.OrderItems[0].SubtotalCents() != want {
		t.Errorf("SubtotalCents() = %d, want %d", got.OrderItems[0].SubtotalCents(), want)
	}
}

func TestCartOneLivePerUser(t *testing.T) {
	db := openTestDB(t)

	user := models.User{Email: "cart@example.com", Password: "x", FirstName: "A", LastName: "B"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&models.Cart{UserID: user.ID}).Error; err != nil {
		t.Fatalf("create first cart: %v", err)
	}

	if err := db.Create(&models.Cart{UserID: user.ID}).Error; err == nil {
		t.Fatal("creating a second live cart: want error, got nil")
	}
}
