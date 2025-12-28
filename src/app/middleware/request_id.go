// Package middleware contains HTTP middleware for the Gin router.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the HTTP header used for request tracing.
const RequestIDHeader = "X-Request-ID"

// RequestIDKey is the context key for storing the request ID.
const RequestIDKey = "request_id"

// RequestID is a middleware that injects a unique request ID into each request.
// If the incoming request already has an X-Request-ID header, it will be reused.
// Otherwise, a new UUID is generated.
//
// The request ID is:
// 1. Stored in the Gin context (accessible via c.GetString(RequestIDKey))
// 2. Added to the response headers
//
// Usage:
//
//	router.Use(middleware.RequestID())
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request already has an ID (from load balancer, API gateway, etc.)
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store in context for later use (logging, error responses, etc.)
		c.Set(RequestIDKey, requestID)

		// Add to response headers for client-side tracing
		c.Header(RequestIDHeader, requestID)

		c.Next()
	}
}

// GetRequestID retrieves the request ID from the Gin context.
// Returns empty string if not set.
func GetRequestID(c *gin.Context) string {
	if id, exists := c.Get(RequestIDKey); exists {
		if requestID, ok := id.(string); ok {
			return requestID
		}
	}
	return ""
}

