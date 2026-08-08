package worker

import (
	"context"
	"log/slog"
	"sync"
)

// BatchProcessor classifies a published batch (implemented by usecase.ClassificationService).
type BatchProcessor interface {
	ProcessBatch(ctx context.Context, batchID int64) error
}

// DispatcherConfig tunes the in-memory classification queue.
type DispatcherConfig struct {
	Workers int
	Buffer  int
}

// DefaultDispatcherConfig returns sensible pool defaults.
func DefaultDispatcherConfig() DispatcherConfig {
	return DispatcherConfig{Workers: 2, Buffer: 64}
}

// Dispatcher is a buffered-channel ClassificationDispatcher with a worker pool.
type Dispatcher struct {
	jobs    chan int64
	proc    BatchProcessor
	log     *slog.Logger
	workers int

	wg     sync.WaitGroup
	cancel context.CancelFunc
	once   sync.Once
}

// NewDispatcher builds a ClassificationDispatcher backed by an in-memory queue.
func NewDispatcher(proc BatchProcessor, cfg DispatcherConfig, log *slog.Logger) *Dispatcher {
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.Buffer <= 0 {
		cfg.Buffer = 64
	}
	return &Dispatcher{
		jobs:    make(chan int64, cfg.Buffer),
		proc:    proc,
		log:     log,
		workers: cfg.Workers,
	}
}

// Start launches the worker pool. Cancel ctx (or call Stop) to shut down.
func (d *Dispatcher) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.loop(runCtx, i)
	}
	d.log.Info("classification dispatcher started", "workers", d.workers, "buffer", cap(d.jobs))
}

// Stop cancels workers and waits for them to exit. Safe to call once.
func (d *Dispatcher) Stop() {
	d.once.Do(func() {
		if d.cancel != nil {
			d.cancel()
		}
		d.wg.Wait()
		d.log.Info("classification dispatcher stopped")
	})
}

// Enqueue queues a batch for classification.
func (d *Dispatcher) Enqueue(ctx context.Context, batchID int64) error {
	select {
	case d.jobs <- batchID:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dispatcher) loop(ctx context.Context, workerID int) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case batchID := <-d.jobs:
			if err := d.proc.ProcessBatch(ctx, batchID); err != nil {
				d.log.Error("classification worker error",
					"worker", workerID,
					"batch_id", batchID,
					"error", err,
				)
			}
		}
	}
}
