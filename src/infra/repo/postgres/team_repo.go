package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
)

func (r *Repositories) EnsureTeamCount(ctx context.Context, teamCount int) ([]domain.Team, error) {
	var teams []domain.Team
	err := r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		var current int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM teams`).Scan(&current); err != nil {
			return err
		}
		for i := current; i < teamCount; i++ {
			if _, err := tx.Exec(ctx, `INSERT INTO teams (name) VALUES ($1)`, fmt.Sprintf("Team %d", i+1)); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `SELECT id, name, created_at FROM teams ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t domain.Team
			if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
				return err
			}
			teams = append(teams, t)
		}
		return rows.Err()
	})
	return teams, err
}

func (r *Repositories) GetTeam(ctx context.Context, teamID int64) (*domain.Team, error) {
	var t domain.Team
	err := r.pg.Pool.QueryRow(ctx, `SELECT id, name, created_at FROM teams WHERE id = $1`, teamID).
		Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("team")
		}
		return nil, err
	}
	return &t, nil
}
