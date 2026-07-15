// Command api is the entrypoint for the go-ecommerce API server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/database"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/logger"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/server"
)

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
	// Bounds the time an idle connection is held open between requests.
	idleTimeout = 60 * time.Second
	// Separate from readTimeout so a client cannot hold a connection open by
	// dribbling headers (Slowloris).
	readHeaderTimeout = 5 * time.Second
	// How long in-flight requests get to finish during shutdown.
	shutdownTimeout = 15 * time.Second
)

func main() {
	// All the real work happens in run so that its defers actually execute:
	// Fatal calls os.Exit, which skips them.
	if err := run(); err != nil {
		// Bound to a variable because zerolog's Fatal has a pointer receiver.
		log := logger.New(os.Getenv("GIN_MODE"))
		log.Fatal().Err(err).Msg("startup failed")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Server.GinMode)
	gin.SetMode(cfg.Server.GinMode)

	// Cancels on Ctrl-C or SIGTERM, so a slow connect can be interrupted.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.New(ctx, &cfg.Database, cfg.Server.GinMode)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	defer func() {
		if closeErr := database.Close(db); closeErr != nil {
			log.Error().Err(closeErr).Msg("failed to close database")
		}
	}()

	logStartupWarnings(&log, cfg)

	srv, err := server.New(cfg, db, &log)
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	httpServer := &http.Server{
		Addr:              net.JoinHostPort("", cfg.Server.Port),
		Handler:           srv.Routes(),
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Buffered so the goroutine can exit even if nothing reads the error.
	serverErr := make(chan error, 1)

	go func() {
		log.Info().Str("port", cfg.Server.Port).Msg("http server listening")

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Whichever comes first: the server dies, or we are asked to stop.
	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	}

	// A fresh context: ctx is already cancelled, and Shutdown needs a live one
	// to bound the grace period.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	log.Info().Msg("shutdown complete")

	return nil
}

// logStartupWarnings surfaces insecure defaults that are tolerable in
// development but must not reach production unnoticed.
func logStartupWarnings(log *zerolog.Logger, cfg *config.Config) {
	if !cfg.IsProduction() && cfg.JWT.UsesDefaultSecret() {
		log.Warn().Msg("JWT_SECRET is the default value; set a real secret before deploying")
	}

	if cfg.IsProduction() && cfg.Server.AllowsAnyOrigin() {
		log.Warn().Msg("ALLOWED_ORIGINS is '*' in release mode; set explicit origins")
	}
}
