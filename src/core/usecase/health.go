package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/ports"
)

// HealthService handles health check logic.
// In a real application, this would check all critical dependencies.
type HealthService struct {
	log *slog.Logger
	// TODO: Add repository/service dependencies to check their health
	// db ports.Repository
}

// NewHealthService creates a new HealthService.
func NewHealthService(log *slog.Logger) *HealthService {
	return &HealthService{
		log: log,
	}
}

// HealthStatus represents the health of the application.
type HealthStatus struct {
	Status     string                    `json:"status"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

// ComponentHealth represents the health of a single component.
type ComponentHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Check performs a health check of all application components.
// Returns the overall health status.
func (s *HealthService) Check(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:     "ok",
		Components: make(map[string]ComponentHealth),
	}

	// TODO: Add database health check
	// if s.db != nil {
	//     if err := s.db.Health(ctx); err != nil {
	//         status.Status = "degraded"
	//         status.Components["database"] = ComponentHealth{
	//             Status:  "unhealthy",
	//             Message: err.Error(),
	//         }
	//     } else {
	//         status.Components["database"] = ComponentHealth{Status: "healthy"}
	//     }
	// }

	return status
}

// Ensure HealthService can use ports (compile-time check)
var _ ports.ExternalService = (*healthServiceAdapter)(nil)

// healthServiceAdapter adapts HealthService to the ExternalService interface.
// This is just for demonstration - typically you'd have real external services.
type healthServiceAdapter struct {
	*HealthService
}

func (h *healthServiceAdapter) Health(ctx context.Context) error {
	status := h.Check(ctx)
	if status.Status != "ok" {
		return &healthError{status: status.Status}
	}
	return nil
}

type healthError struct {
	status string
}

func (e *healthError) Error() string {
	return "health check failed: " + e.status
}

