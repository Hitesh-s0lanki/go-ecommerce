// Package utils holds small helpers shared by the HTTP layer.
package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the envelope every endpoint returns.
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// PaginatedResponse is a Response carrying page metadata.
type PaginatedResponse struct {
	Response
	Meta PaginationMeta `json:"meta"`
}

// PaginationMeta describes the current page of a list endpoint.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// SuccessResponse writes 200 with data.
func SuccessResponse(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Response{Success: true, Message: message, Data: data})
}

// CreatedResponse writes 201 with data.
func CreatedResponse(c *gin.Context, message string, data any) {
	c.JSON(http.StatusCreated, Response{Success: true, Message: message, Data: data})
}

// PaginatedSuccessResponse writes 200 with data and page metadata.
func PaginatedSuccessResponse(c *gin.Context, message string, data any, meta PaginationMeta) {
	c.JSON(http.StatusOK, PaginatedResponse{
		Response: Response{Success: true, Message: message, Data: data},
		Meta:     meta,
	})
}

// ErrorResponse writes an error with the given status.
//
// err is attached to the gin context for the logging middleware, and its text
// is only sent to the client for 4xx statuses: a 4xx describes what the caller
// did wrong, while a 5xx is an internal failure whose text can leak schema
// details, file paths, or driver internals.
func ErrorResponse(c *gin.Context, statusCode int, message string, err error) {
	if err != nil {
		// Recorded for the request log; never assumes it is client-safe.
		_ = c.Error(err)
	}

	response := Response{Success: false, Message: message}
	if err != nil && statusCode < http.StatusInternalServerError {
		response.Error = err.Error()
	}

	c.AbortWithStatusJSON(statusCode, response)
}

// BadRequestResponse writes 400.
func BadRequestResponse(c *gin.Context, message string, err error) {
	ErrorResponse(c, http.StatusBadRequest, message, err)
}

// UnauthorizedResponse writes 401.
func UnauthorizedResponse(c *gin.Context, message string) {
	ErrorResponse(c, http.StatusUnauthorized, message, nil)
}

// ForbiddenResponse writes 403.
func ForbiddenResponse(c *gin.Context, message string) {
	ErrorResponse(c, http.StatusForbidden, message, nil)
}

// NotFoundResponse writes 404.
func NotFoundResponse(c *gin.Context, message string) {
	ErrorResponse(c, http.StatusNotFound, message, nil)
}

// InternalServerErrorResponse writes 500. err is logged, not returned to the
// client.
func InternalServerErrorResponse(c *gin.Context, message string, err error) {
	ErrorResponse(c, http.StatusInternalServerError, message, err)
}
