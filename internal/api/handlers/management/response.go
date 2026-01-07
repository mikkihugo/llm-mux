// Package management provides handlers for the management API.
package management

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nghyane/llm-mux/internal/buildinfo"
)

// APIResponse is the standard response envelope for v1 management API.
type APIResponse struct {
	Data interface{} `json:"data"`
	Meta APIMeta     `json:"meta"`
}

// APIMeta contains response metadata.
type APIMeta struct {
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// APIError is the standard error response for v1 management API.
type APIError struct {
	Error APIErrorDetail `json:"error"`
}

// APIErrorDetail contains error details.
type APIErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Standard error codes for management API.
const (
	ErrCodeInvalidRequest = "INVALID_REQUEST"
	ErrCodeInvalidConfig  = "INVALID_CONFIG"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeInternalError  = "INTERNAL_ERROR"
	ErrCodeWriteFailed    = "WRITE_FAILED"
	ErrCodeReloadFailed   = "RELOAD_FAILED"
	ErrCodeValidation     = "VALIDATION_ERROR"
)

// respondOK sends a successful response with data envelope.
func respondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Data: data,
		Meta: APIMeta{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Version:   buildinfo.Version,
		},
	})
}

// respondError sends an error response with the given status code.
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, APIError{
		Error: APIErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// respondBadRequest sends a 400 Bad Request error.
func respondBadRequest(c *gin.Context, message string) {
	respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, message)
}

// respondNotFound sends a 404 Not Found error.
func respondNotFound(c *gin.Context, message string) {
	respondError(c, http.StatusNotFound, ErrCodeNotFound, message)
}

// respondInternalError sends a 500 Internal Server Error.
func respondInternalError(c *gin.Context, message string) {
	respondError(c, http.StatusInternalServerError, ErrCodeInternalError, message)
}

// respondUnauthorized sends a 401 Unauthorized error.
func respondUnauthorized(c *gin.Context, message string) {
	respondError(c, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}
