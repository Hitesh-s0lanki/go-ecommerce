package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Hitesh-s0lanki/go-ecommerce/internal/models"
	"github.com/Hitesh-s0lanki/go-ecommerce/internal/utils"
)

// Context keys set by Authenticate. Unexported, with accessors below, so a
// handler cannot typo "user_id" and silently read nothing.
const (
	ctxUserID    = "auth.user_id"
	ctxUserEmail = "auth.user_email"
	ctxUserRole  = "auth.user_role"
)

// CurrentUserID returns the authenticated user's id, and false if the request
// did not pass through Authenticate.
func CurrentUserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get(ctxUserID)
	if !ok {
		return 0, false
	}

	id, ok := v.(uint)

	return id, ok
}

// CurrentUserEmail returns the authenticated user's email.
func CurrentUserEmail(c *gin.Context) (string, bool) {
	v, ok := c.Get(ctxUserEmail)
	if !ok {
		return "", false
	}

	email, ok := v.(string)

	return email, ok
}

// CurrentUserRole returns the authenticated user's role.
func CurrentUserRole(c *gin.Context) (models.UserRole, bool) {
	v, ok := c.Get(ctxUserRole)
	if !ok {
		return "", false
	}

	role, ok := v.(models.UserRole)

	return role, ok
}

// Authenticate rejects any request without a valid access token.
func (s *Server) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			// RFC 9110: a 401 must say how to authenticate.
			c.Header("WWW-Authenticate", `Bearer realm="api"`)
			utils.UnauthorizedResponse(c, "authorization header must be 'Bearer <token>'")

			return
		}

		// ParseAccessToken, not a generic parse: a refresh token is signed
		// with the same secret and would otherwise be accepted here, which
		// would hand a long-lived token the rights of a short-lived one.
		claims, err := s.tokens.ParseAccessToken(token)
		if err != nil {
			c.Header("WWW-Authenticate", `Bearer realm="api", error="invalid_token"`)
			utils.UnauthorizedResponse(c, "invalid or expired token")

			return
		}

		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxUserEmail, claims.Email)
		c.Set(ctxUserRole, models.UserRole(claims.Role))

		c.Next()
	}
}

// RequireAdmin rejects a request whose user is not an administrator. It must
// run after Authenticate.
func (s *Server) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := CurrentUserRole(c)
		if !ok {
			// No role means Authenticate did not run: a routing mistake, not
			// a client error. Fail closed and make the cause visible.
			s.logger.Error().
				Str("path", c.FullPath()).
				Msg("RequireAdmin used without Authenticate")
			utils.ErrorResponse(c, http.StatusInternalServerError, "server misconfiguration", nil)

			return
		}

		if role != models.UserRoleAdmin {
			utils.ForbiddenResponse(c, "admin access required")
			return
		}

		c.Next()
	}
}

// bearerToken extracts the credential from an Authorization header.
//
// The scheme is matched case-insensitively: RFC 9110 defines it as
// case-insensitive, and clients do send "bearer".
func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found {
		return "", false
	}

	if !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	// A second space means a malformed header, e.g. "Bearer a b".
	if strings.Contains(token, " ") {
		return "", false
	}

	return token, true
}
