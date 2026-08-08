package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/ports"
)

// HealthService checks application dependencies.
type HealthService struct {
	repo ports.Store
	log  *slog.Logger
}

func NewHealthService(repo ports.Store, log *slog.Logger) *HealthService {
	return &HealthService{repo: repo, log: log}
}

type HealthStatus struct {
	Status     string                     `json:"status"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

type ComponentHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (s *HealthService) Check(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:     "ok",
		Components: make(map[string]ComponentHealth),
	}

	if err := s.repo.Health(ctx); err != nil {
		status.Status = "degraded"
		status.Components["database"] = ComponentHealth{
			Status:  "unhealthy",
			Message: err.Error(),
		}
	} else {
		status.Components["database"] = ComponentHealth{Status: "healthy"}
	}

	return status
}
