package ports

import (
	"context"
)

// ExternalService is the base interface for external service adapters.
type ExternalService interface {
	// Health checks if the external service is reachable.
	Health(ctx context.Context) error
}
