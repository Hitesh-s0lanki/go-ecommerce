// Package database opens and configures the Postgres connection.
package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
)

// Pool defaults. Postgres allows 100 connections by default, so leave headroom
// for migrations, psql sessions, and other replicas.
const (
	maxOpenConns    = 25
	maxIdleConns    = 5
	connMaxLifetime = time.Hour
	connMaxIdleTime = 10 * time.Minute
	pingTimeout     = 5 * time.Second
)

// New opens a Postgres connection, configures the pool, and verifies the
// database is actually reachable before returning.
//
// gorm.Open alone establishes a lazy pool: without the ping, an unreachable
// database only surfaces on the first query.
func New(ctx context.Context, cfg *config.DatabaseConfig, ginMode string) (*gorm.DB, error) {
	logLevel := gormlogger.Info
	if ginMode == "release" {
		// Statement logging is far too noisy for production.
		logLevel = gormlogger.Warn
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("open connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		// Don't leak the pool when the caller never gets the handle.
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

// Close releases the connection pool.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("access underlying sql.DB: %w", err)
	}

	return sqlDB.Close()
}
