package server_test

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
)

// testSecret is long enough to satisfy the token manager's key-length check.
const testSecret = "test-secret-that-is-at-least-32-bytes-long"

func testConfig(origins []string) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Port:           "8080",
			GinMode:        gin.TestMode,
			AllowedOrigins: origins,
		},
		JWT: config.JWTConfig{
			Secret:              testSecret,
			ExpiresIn:           time.Hour,
			RefreshTokenExpires: 24 * time.Hour,
		},
	}
}
