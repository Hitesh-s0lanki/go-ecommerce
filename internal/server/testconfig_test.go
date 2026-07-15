package server_test

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/config"
)

// testSecret is long enough to satisfy the token manager's key-length check.
const testSecret = "test-secret-that-is-at-least-32-bytes-long"

// testUploadsDir backs the local upload provider that server.New builds.
//
// A shared temp directory rather than t.TempDir(): testConfig has no *testing.T
// and is called from every test that builds a Server. Nothing writes here
// unless a test uploads, and those pass their own directory. TestMain removes
// it.
var testUploadsDir = func() string {
	dir, err := os.MkdirTemp("", "go-ecommerce-server-test")
	if err != nil {
		panic("create temp upload dir: " + err.Error())
	}

	return dir
}()

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
		// Off: these tests are about HTTP, and a publisher would have them
		// reaching for a queue that is not there.
		Events: config.EventsConfig{Enabled: false},
		Upload: config.UploadConfig{
			Provider:    config.UploadProviderLocal,
			Path:        testUploadsDir,
			MaxFileSize: 10 << 20,
		},
	}
}
