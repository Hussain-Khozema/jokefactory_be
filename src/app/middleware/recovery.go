package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/response"
)

// Recovery is a middleware that recovers from panics and returns a 500 error.
// It logs the panic with stack trace for debugging.
//
// This should be one of the first middleware in the chain to catch all panics.
//
// Usage:
//
//	router.Use(middleware.Recovery(logger))
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get request ID for correlation
				requestID := GetRequestID(c)

				// Log the panic with stack trace
				log.Error("panic recovered",
					"request_id", requestID,
					"error", err,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"stack", string(debug.Stack()),
				)

				// Return a generic error to the client
				// Don't expose internal details for security
				c.AbortWithStatusJSON(http.StatusInternalServerError, response.Error{
					Error: response.ErrorDetail{
						Code:      "INTERNAL_ERROR",
						Message:   "An unexpected error occurred",
						RequestID: requestID,
					},
				})
			}
		}()

		c.Next()
	}
}

