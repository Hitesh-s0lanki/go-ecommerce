package server

import (
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// requestIDHeader carries the correlation id in and out of the service.
const requestIDHeader = "X-Request-ID"

// requestIDKey is the gin context key holding the request id.
const requestIDKey = "request_id"

// RequestID returns the correlation id for the request, or "" if unset.
func RequestID(c *gin.Context) string {
	id, _ := c.Get(requestIDKey)

	s, _ := id.(string)

	return s
}

// requestID reuses an inbound X-Request-ID when present so a correlation id
// set by a proxy or caller survives, and mints one otherwise.
func (s *Server) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		c.Set(requestIDKey, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// requestLogger logs one structured line per request through zerolog.
//
// gin.Logger() is deliberately not used: it writes its own text format to
// stdout, which would sit alongside the JSON logs in release mode rather than
// being parseable with them.
func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		status := c.Writer.Status()

		event := s.logger.Info()

		switch {
		case status >= http.StatusInternalServerError:
			event = s.logger.Error()
		case status >= http.StatusBadRequest:
			event = s.logger.Warn()
		}

		event = event.
			Str("request_id", RequestID(c)).
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Dur("latency", time.Since(start)).
			Str("ip", c.ClientIP())

		if query != "" {
			event = event.Str("query", query)
		}

		// Errors recorded via c.Error, including the internal detail that
		// ErrorResponse withholds from 5xx clients.
		if len(c.Errors) > 0 {
			event = event.Str("errors", c.Errors.String())
		}

		event.Msg("request")
	}
}

// cors applies the configured origin policy.
//
// Unlike a blanket "Access-Control-Allow-Origin: *", a configured origin is
// echoed back with Vary: Origin so caches do not serve one origin's response
// to another.
func (s *Server) cors() gin.HandlerFunc {
	allowAny := s.config.Server.AllowsAnyOrigin()
	allowed := s.config.Server.AllowedOrigins

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		switch {
		case allowAny:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "" && slices.Contains(allowed, origin):
			c.Header("Access-Control-Allow-Origin", origin)
			// Credentials are only ever allowed for an explicit origin: the
			// browser rejects them alongside "*" anyway.
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		// The response body varies by Origin even when the request is denied.
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, "+requestIDHeader)
		c.Header("Access-Control-Max-Age", "300")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
