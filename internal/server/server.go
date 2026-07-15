// Package server wires the HTTP routes and middleware.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	// Registers the generated spec with swag; imported only for that effect.
	_ "github.com/Hitesh-s0lanki/go-ecommerce/docs"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/auth"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/services"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/storage"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// readyTimeout bounds the readiness probe's database check.
const readyTimeout = 2 * time.Second

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	config     *config.Config
	db         *gorm.DB
	logger     zerolog.Logger
	tokens     *auth.TokenManager
	auth       *services.AuthService
	users      *services.UserService
	categories *services.CategoryService
	products   *services.ProductService
	uploads    *services.UploadService
	carts      *services.CartService
	orders     *services.OrderService
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

	// Built here so a misconfigured bucket or an unwritable upload directory
	// stops the process, rather than surfacing as a 500 on the first upload.
	provider, err := storage.New(context.Background(), &storage.Config{
		Provider:      cfg.Upload.Provider,
		Path:          cfg.Upload.Path,
		PublicBaseURL: cfg.Upload.PublicBaseURL,
		Region:        cfg.AWS.Region,
		AccessKeyID:   cfg.AWS.AccessKeyID,
		SecretKey:     cfg.AWS.SecretAccessKey,
		Bucket:        cfg.AWS.S3Bucket,
		Endpoint:      cfg.AWS.S3Endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("build upload provider: %w", err)
	}

	return &Server{
		config: cfg,
		db:     db,
		logger: *logger,
		tokens: tokens,
		// Built once, not per request as in the reference: they hold no
		// per-request state, so rebuilding them on every call is pure waste.
		auth:       services.NewAuthService(db, tokens, cfg.JWT.RefreshTokenExpires, logger),
		users:      services.NewUserService(db),
		categories: services.NewCategoryService(db),
		products:   services.NewProductService(db),
		uploads:    services.NewUploadService(provider, cfg.Upload.MaxFileSize),
		carts:      services.NewCartService(db),
		orders:     services.NewOrderService(db),
	}, nil
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

	// The interactive docs are development-only: they describe every endpoint
	// and its payloads, which is reconnaissance in production.
	if !s.config.IsProduction() {
		swagger := ginswagger.WrapHandler(swaggerfiles.Handler)

		router.GET("/swagger/*any", func(c *gin.Context) {
			// The wrapped handler only serves named files, so /swagger/ —
			// which is what gin redirects /swagger to, and what anyone
			// actually types — would 404. Send it to the real page.
			if p := c.Param("any"); p == "" || p == "/" {
				c.Redirect(http.StatusFound, "/swagger/index.html")
				return
			}

			swagger(c)
		})
	}

	api := router.Group("/api/v1")

	authRoutes := api.Group("/auth")
	authRoutes.POST("/register", s.register)
	authRoutes.POST("/login", s.login)
	authRoutes.POST("/refresh", s.refresh)
	// Logout takes the refresh token in the body and needs no access token:
	// a client whose access token has expired must still be able to log out.
	authRoutes.POST("/logout", s.logout)
	authRoutes.GET("/me", s.Authenticate(), s.me)

	// The catalogue is readable without an account: a shop whose products
	// only load once you log in has nothing to sell to a new visitor. These
	// read paths return active rows only, so nothing withdrawn is exposed.
	api.GET("/categories", s.getCategories)
	api.GET("/products", s.getProducts)
	api.GET("/products/:id", s.getProduct)

	// Everything below requires a valid access token.
	protected := api.Group("")
	protected.Use(s.Authenticate())

	users := protected.Group("/users")
	users.GET("/profile", s.getProfile)
	users.PUT("/profile", s.updateProfile)

	// A cart and an order belong to the customer who owns them, so these are
	// scoped to the caller's own id rather than taking one from the path:
	// there is no id to tamper with.
	cart := protected.Group("/cart")
	cart.GET("", s.getCart)
	cart.POST("/items", s.addToCart)
	cart.PUT("/items/:id", s.updateCartItem)
	cart.DELETE("/items/:id", s.removeFromCart)

	orders := protected.Group("/orders")
	orders.POST("", s.createOrder)
	orders.GET("", s.getOrders)
	orders.GET("/:id", s.getOrder)

	// Writing the catalogue is an admin power. RequireAdmin sits on the group
	// rather than on each route, so a later endpoint added here cannot ship
	// unguarded by forgetting one middleware argument.
	admin := protected.Group("")
	admin.Use(s.RequireAdmin())

	// "" rather than "/": with "/" the route is /api/v1/categories/, and a
	// client POSTing to /api/v1/categories gets a redirect instead of a
	// created row.
	adminCategories := admin.Group("/categories")
	adminCategories.POST("", s.createCategory)
	adminCategories.PUT("/:id", s.updateCategory)
	adminCategories.DELETE("/:id", s.deleteCategory)

	adminProducts := admin.Group("/products")
	adminProducts.POST("", s.createProduct)
	adminProducts.PUT("/:id", s.updateProduct)
	adminProducts.DELETE("/:id", s.deleteProduct)
	adminProducts.POST("/:id/images", s.uploadProductImage)

	s.mountUploads(router)

	return router
}

// mountUploads serves locally stored uploads.
//
// Only when this process is the one holding them: with s3, or with a CDN in
// front, the files are somewhere else entirely and this route would 404 for
// every request.
func (s *Server) mountUploads(router *gin.Engine) {
	if !s.config.Upload.UsesLocalProvider() || s.config.Upload.PublicBaseURL != "" {
		return
	}

	// false: no directory listing. The default would let anyone walk
	// /uploads/products and enumerate every image in the shop.
	fileServer := http.StripPrefix("/uploads",
		http.FileServer(gin.Dir(s.config.Upload.Path, false)))

	serve := func(c *gin.Context) {
		// These bytes came from a user and are served from the API's own
		// origin, so a file that a browser decides is HTML would run scripts
		// with this site's cookies. The upload path only stores files it has
		// identified as images, and these headers are what stops the browser
		// from second-guessing that.
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Security-Policy", "default-src 'none'; sandbox")

		fileServer.ServeHTTP(c.Writer, c.Request)
	}

	router.GET("/uploads/*filepath", serve)
	router.HEAD("/uploads/*filepath", serve)
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
