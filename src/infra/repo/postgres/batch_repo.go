package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

const batchColumns = `batch_id, round_id, team_id, status, submitted_at, processed_at, locked_at, locked_by, created_at`

func scanBatch(row scannable) (*domain.Batch, error) {
	var b domain.Batch
	err := row.Scan(
		&b.ID, &b.RoundID, &b.TeamID, &b.Status, &b.SubmittedAt,
		&b.ProcessedAt, &b.LockedAt, &b.LockedBy, &b.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repositories) CreateBatch(ctx context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error) {
	var batch *domain.Batch
	err := r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		batch, err = scanBatch(tx.QueryRow(ctx, `
			INSERT INTO batches (round_id, team_id, status, submitted_at)
			VALUES ($1, $2, 'SUBMITTED', now())
			RETURNING `+batchColumns, roundID, teamID))
		if err != nil {
			return err
		}
		for _, text := range jokes {
			if _, err := tx.Exec(ctx, `INSERT INTO jokes (batch_id, joke_text) VALUES ($1, $2)`, batch.ID, text); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO batch_submission_events (round_id, team_id, batch_id, jokes_count, delta)
			VALUES ($1, $2, $3, $4, 1)`, roundID, teamID, batch.ID, len(jokes)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE team_rounds_state
			SET batches_created = batches_created + 1, updated_at = now()
			WHERE round_id = $1 AND team_id = $2`, roundID, teamID)
		return err
	})
	return batch, err
}

func (r *Repositories) ListBatchesByTeam(ctx context.Context, roundID, teamID int64) ([]domain.Batch, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT `+batchColumns+`
		FROM batches
		WHERE round_id = $1 AND team_id = $2
		ORDER BY submitted_at DESC, batch_id DESC`, roundID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []domain.Batch
	var batchIDs []int64
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		batches = append(batches, *b)
		batchIDs = append(batchIDs, b.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(batchIDs) == 0 {
		return batches, nil
	}

	jokeRows, err := r.pg.Pool.Query(ctx, `
		SELECT joke_id, batch_id, joke_text, joke_title, publish_status, published_at, created_at
		FROM jokes
		WHERE batch_id = ANY($1)
		ORDER BY batch_id, joke_id`, batchIDs)
	if err != nil {
		return nil, err
	}
	defer jokeRows.Close()

	jokeMap := make(map[int64][]domain.Joke)
	for jokeRows.Next() {
		var j domain.Joke
		if err := jokeRows.Scan(&j.ID, &j.BatchID, &j.Text, &j.Title, &j.PublishStatus, &j.PublishedAt, &j.CreatedAt); err != nil {
			return nil, err
		}
		jokeMap[j.BatchID] = append(jokeMap[j.BatchID], j)
	}
	if err := jokeRows.Err(); err != nil {
		return nil, err
	}
	for i := range batches {
		batches[i].Jokes = jokeMap[batches[i].ID]
	}
	return batches, nil
}

func (r *Repositories) GetBatchWithJokes(ctx context.Context, batchID int64) (*ports.BatchWithJokes, error) {
	b, err := scanBatch(r.pg.Pool.QueryRow(ctx, `
		SELECT `+batchColumns+` FROM batches WHERE batch_id = $1`, batchID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("batch")
		}
		return nil, err
	}

	rows, err := r.pg.Pool.Query(ctx, `
		SELECT joke_id, batch_id, joke_text, joke_title, publish_status, published_at, created_at
		FROM jokes WHERE batch_id = $1 ORDER BY joke_id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jokes []domain.Joke
	for rows.Next() {
		var j domain.Joke
		if err := rows.Scan(&j.ID, &j.BatchID, &j.Text, &j.Title, &j.PublishStatus, &j.PublishedAt, &j.CreatedAt); err != nil {
			return nil, err
		}
		jokes = append(jokes, j)
	}
	return &ports.BatchWithJokes{Batch: *b, Jokes: jokes}, rows.Err()
}

func (r *Repositories) CountSubmittedBatches(ctx context.Context, roundID int64) (int, error) {
	var count int
	err := r.pg.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM batches WHERE round_id = $1 AND status = 'SUBMITTED'`, roundID).Scan(&count)
	return count, err
}
