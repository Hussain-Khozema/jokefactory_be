package postgres

import (
	"context"
	"log/slog"

	"jokefactory/src/infra/db"
)

type Repositories struct {
	pg  *db.Postgres
	log *slog.Logger
}

func New(pg *db.Postgres, log *slog.Logger) *Repositories {
	return &Repositories{pg: pg, log: log}
}

func (r *Repositories) Health(ctx context.Context) error {
	return r.pg.Health(ctx)
}
