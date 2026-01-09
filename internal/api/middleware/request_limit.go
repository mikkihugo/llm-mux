// Package middleware provides HTTP middleware components for the CLI Proxy API server.
// This file contains the request size limit middleware that protects against
// oversized payloads that could cause memory exhaustion (DoS/OOM).
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Default request size limits for different endpoint types.
const (
	// DefaultMaxChatRequestSize is the maximum request body size for chat endpoints.
	// Set to 50MB to accommodate:
	// - Claude 200K tokens (~2MB text)
	// - Gemini 1M tokens (~8MB text)
	// - Base64-encoded images (~13-20MB each)
	// - Multi-modal requests with multiple images + context
	DefaultMaxChatRequestSize = 50 * 1024 * 1024 // 50MB

	// DefaultMaxEmbedRequestSize is the maximum request body size for embedding endpoints.
	// Embeddings are text-only, so a smaller limit is appropriate.
	DefaultMaxEmbedRequestSize = 10 * 1024 * 1024 // 10MB

	// DefaultMaxResponseSize is the maximum response body size to read into memory.
	// Set larger than request size to handle verbose LLM responses.
	DefaultMaxResponseSize = 100 * 1024 * 1024 // 100MB
)

// RequestSizeLimitMiddleware creates a Gin middleware that limits request body size.
// It uses http.MaxBytesReader which automatically:
// - Returns HTTP 413 (Request Entity Too Large) when limit is exceeded
// - Closes the connection to prevent slow-reading attacks
// - Integrates properly with Gin's error handling
//
// Parameters:
//   - maxBytes: Maximum allowed request body size in bytes. Use 0 for default (50MB).
//
// Example usage:
//
//	v1 := r.Group("/v1")
//	v1.Use(middleware.RequestSizeLimitMiddleware(50 * 1024 * 1024)) // 50MB
func RequestSizeLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxChatRequestSize
	}

	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// RequestSizeLimitWithConfigMiddleware creates a middleware that uses a function
// to retrieve the current size limit. This allows for dynamic configuration
// via hot-reload without restarting the server.
//
// Parameters:
//   - getMaxBytes: Function that returns the current maximum size limit.
//     If it returns 0, DefaultMaxChatRequestSize is used.
func RequestSizeLimitWithConfigMiddleware(getMaxBytes func() int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := getMaxBytes()
		if maxBytes <= 0 {
			maxBytes = DefaultMaxChatRequestSize
		}

		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
