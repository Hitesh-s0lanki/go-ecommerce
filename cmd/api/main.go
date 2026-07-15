// Command api is the entrypoint for the go-ecommerce API server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/database"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/logger"
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

	log.Info().
		Str("mode", cfg.Server.GinMode).
		Str("port", cfg.Server.Port).
		Str("database", cfg.Database.Name).
		Msg("database connected; HTTP server not implemented yet")

	<-ctx.Done()
	log.Info().Msg("shutting down")

	return nil
}

// logStartupWarnings surfaces insecure defaults that are tolerable in
// development but must not reach production unnoticed.
func logStartupWarnings(log *zerolog.Logger, cfg *config.Config) {
	if !cfg.IsProduction() && cfg.JWT.Secret == "change_me_in_production" {
		log.Warn().Msg("JWT_SECRET is the default value; set a real secret before deploying")
	}
}
