package worker

import (
	"context"
	"log/slog"
	"time"

	"jokefactory/src/core/ports"
)

// OrphanSource lists batches that still need classification.
type OrphanSource interface {
	ListOrphanClassificationBatchIDs(ctx context.Context, limit int) ([]int64, error)
}

// Reconciler periodically re-enqueues orphan / incomplete classification jobs.
type Reconciler struct {
	source   OrphanSource
	dispatch ports.ClassificationDispatcher
	interval time.Duration
	limit    int
	log      *slog.Logger

	cancel context.CancelFunc
}

// NewReconciler builds a startup + periodic classification reconciler.
func NewReconciler(
	source OrphanSource,
	dispatch ports.ClassificationDispatcher,
	interval time.Duration,
	log *slog.Logger,
) *Reconciler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Reconciler{
		source:   source,
		dispatch: dispatch,
		interval: interval,
		limit:    50,
		log:      log,
	}
}

// Start runs an immediate sweep, then ticks on interval until ctx is cancelled.
func (r *Reconciler) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	go r.loop(runCtx)
}

// Sweep re-enqueues orphan classification batches once.
func (r *Reconciler) Sweep(ctx context.Context) {
	r.sweep(ctx)
}

// Stop cancels the reconciler loop.
func (r *Reconciler) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Reconciler) loop(ctx context.Context) {
	r.sweep(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sweep(ctx)
		}
	}
}

func (r *Reconciler) sweep(ctx context.Context) {
	ids, err := r.source.ListOrphanClassificationBatchIDs(ctx, r.limit)
	if err != nil {
		r.log.Error("classification reconciler list failed", "error", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	r.log.Info("classification reconciler re-enqueueing", "count", len(ids))
	for _, id := range ids {
		if err := r.dispatch.Enqueue(ctx, id); err != nil {
			r.log.Error("classification reconciler enqueue failed",
				"batch_id", id, "error", err)
			return
		}
	}
}
