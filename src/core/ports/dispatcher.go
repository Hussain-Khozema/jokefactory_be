// Package ports defines driven ports (repositories, classifiers, dispatchers)
// consumed by core usecases.
package ports

import "context"

// ClassificationDispatcher queues a published batch for async classification.
type ClassificationDispatcher interface {
	Enqueue(ctx context.Context, batchID int64) error
}
