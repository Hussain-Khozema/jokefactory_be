// Package db provides database connection management for PostgreSQL.
// It uses pgx as the database driver for better performance and features.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jokefactory/src/infra/config"
)

// Postgres wraps a pgx connection pool with helper methods.
type Postgres struct {
	Pool *pgxpool.Pool
	log  *slog.Logger
}

// New creates a new PostgreSQL connection pool.
// It validates the connection by pinging the database.
func New(ctx context.Context, cfg config.DatabaseConfig, log *slog.Logger) (*Postgres, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Apply connection pool settings
	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("database connection established",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Name,
	)

	return &Postgres{
		Pool: pool,
		log:  log,
	}, nil
}

// Close closes the connection pool.
// Call this during graceful shutdown.
func (p *Postgres) Close() {
	if p.Pool != nil {
		p.Pool.Close()
		p.log.Info("database connection closed")
	}
}

// Health checks if the database is reachable.
// Returns nil if healthy, error otherwise.
func (p *Postgres) Health(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}

// WithTx runs fn inside a single database transaction (unit of work).
//
// It acquires a connection from the pool, begins a transaction, and invokes fn
// with the transaction handle. If fn returns an error (or panics), the
// transaction is rolled back and the error (or panic) is propagated; otherwise
// it is committed. Use this to make multi-table operations atomic (e.g. publish,
// buy, swap, round start).
func (p *Postgres) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure a panic in fn still rolls back the transaction before unwinding.
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return errors.Join(err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// TODO: Add query logging middleware for development
// TODO: Add connection pool metrics
