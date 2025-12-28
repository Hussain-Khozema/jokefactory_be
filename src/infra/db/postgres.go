// Package db provides database connection management for PostgreSQL.
// It uses pgx as the database driver for better performance and features.
package db

import (
	"context"
	"fmt"
	"log/slog"

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

// TODO: Add transaction helper methods
// TODO: Add query logging middleware for development
// TODO: Add connection pool metrics

