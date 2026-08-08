package worker

import "context"

// NoopDispatcher discards enqueue calls (useful in tests).
type NoopDispatcher struct{}

// Enqueue discards the job.
func (NoopDispatcher) Enqueue(context.Context, int64) error { return nil }
