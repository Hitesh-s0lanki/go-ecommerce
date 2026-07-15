// Package server wires the HTTP routes and middleware.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// readyTimeout bounds the readiness probe's database check.
const readyTimeout = 2 * time.Second

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	config *config.Config
	db     *gorm.DB
	logger zerolog.Logger
	tokens *auth.TokenManager
}

// New builds a Server.
//
// It returns an error because the token manager validates the JWT secret: a
// weak secret must stop startup rather than surface later as a forged token.
func New(cfg *config.Config, db *gorm.DB, logger *zerolog.Logger) (*Server, error) {
	tokens, err := auth.NewTokenManager(&cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("build token manager: %w", err)
	}

	return &Server{config: cfg, db: db, logger: *logger, tokens: tokens}, nil
}

// Routes builds the gin engine.
func (s *Server) Routes() *gin.Engine {
	router := gin.New()

	// Off by default, which would make NoMethod dead code and answer a wrong
	// method with 404 instead of 405.
	router.HandleMethodNotAllowed = true

	router.Use(s.requestID())
	router.Use(gin.Recovery())
	router.Use(s.requestLogger())
	router.Use(s.cors())

	router.NoRoute(func(c *gin.Context) {
		utils.NotFoundResponse(c, "route not found")
	})

	router.NoMethod(func(c *gin.Context) {
		utils.ErrorResponse(c, http.StatusMethodNotAllowed, "method not allowed", nil)
	})

	// Liveness: the process is up. Kubernetes restarts the pod if this fails,
	// so it must not depend on the database — a database blip should not cause
	// a restart loop.
	router.GET("/health", s.health)

	// Readiness: the process can serve traffic. This one does check the
	// database, so an instance with a dead pool is pulled from the load
	// balancer instead of serving errors.
	router.GET("/health/ready", s.ready)

	return router
}

func (s *Server) health(c *gin.Context) {
	utils.SuccessResponse(c, "ok", gin.H{"status": "ok"})
}

func (s *Server) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readyTimeout)
	defer cancel()

	sqlDB, err := s.db.DB()
	if err != nil {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "database unavailable", err)
		return
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "database unavailable", err)
		return
	}

	utils.SuccessResponse(c, "ready", gin.H{"status": "ready", "database": "up"})
}
