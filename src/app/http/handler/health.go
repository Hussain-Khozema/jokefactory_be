// Package handler contains HTTP handlers for the API.
// Handlers are responsible for:
// - Parsing and validating HTTP requests
// - Calling use case methods
// - Converting results to HTTP responses
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jokefactory/src/core/usecase"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	healthService *usecase.HealthService
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(healthService *usecase.HealthService) *HealthHandler {
	return &HealthHandler{
		healthService: healthService,
	}
}

// HealthResponse is the response for the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// Health returns the health status of the application.
// GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	// For a simple health check, we just return ok
	// The full health check with component status can be on /health/detailed
	c.JSON(http.StatusOK, HealthResponse{
		Status: "ok",
	})
}

// DetailedHealth returns detailed health status including all components.
// GET /health/detailed
func (h *HealthHandler) DetailedHealth(c *gin.Context) {
	status := h.healthService.Check(c.Request.Context())
	c.JSON(http.StatusOK, status)
}

