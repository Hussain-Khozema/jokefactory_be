package middleware

import "github.com/gin-gonic/gin"

// CORS adds basic CORS headers and short-circuits OPTIONS preflight requests.
// It is deliberately permissive for local development; tighten the allowed
// origin and headers as needed for production.
func CORS() gin.HandlerFunc {
	const (
		allowedOrigin = "*"
		allowedMethods = "GET, POST, PATCH, PUT, DELETE, OPTIONS"
		allowedHeaders = "Content-Type, X-User-Id"
		maxAge         = "600"
	)

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Max-Age", maxAge)

		// For preflight requests, return immediately.
		if c.Request.Method == "OPTIONS" {
			c.Status(204)
			c.Abort()
			return
		}

		c.Next()
	}
}

