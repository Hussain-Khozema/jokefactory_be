package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

func (r *Repositories) EnsureClassificationJob(ctx context.Context, batchID, roundID int64) error {
	_, err := r.pg.Pool.Exec(ctx, `
		INSERT INTO classification_jobs (batch_id, round_id, status)
		VALUES ($1, $2, 'PENDING')
		ON CONFLICT (batch_id) DO NOTHING`, batchID, roundID)
	return err
}

func (r *Repositories) ClaimClassificationJob(ctx context.Context, batchID int64) (*domain.ClassificationJob, error) {
	job, err := scanClassificationJob(r.pg.Pool.QueryRow(ctx, `
		UPDATE classification_jobs
		SET status = 'PROCESSING',
		    attempts = attempts + 1,
		    updated_at = now(),
		    last_error = NULL
		WHERE batch_id = $1
		  AND attempts < $2
		  AND (
		    status IN ('PENDING', 'FAILED')
		    OR (status = 'PROCESSING' AND updated_at < now() - make_interval(secs => $3))
		  )
		RETURNING batch_id, round_id, status, attempts, last_error, model,
		          created_at, updated_at, classified_at`,
		batchID, ports.MaxClassificationAttempts, int(ports.StaleClassificationAfter.Seconds())))
	if err == nil {
		return job, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // not claimable is a valid outcome
	}
	return nil, err
}

func (r *Repositories) MarkClassificationDone(ctx context.Context, batchID int64, model string) error {
	_, err := r.pg.Pool.Exec(ctx, `
		UPDATE classification_jobs
		SET status = 'DONE',
		    model = $2,
		    last_error = NULL,
		    classified_at = now(),
		    updated_at = now()
		WHERE batch_id = $1`, batchID, model)
	return err
}

func (r *Repositories) MarkClassificationFailed(ctx context.Context, batchID int64, errMsg string) error {
	_, err := r.pg.Pool.Exec(ctx, `
		UPDATE classification_jobs
		SET status = CASE
		      WHEN attempts >= $3 THEN 'FAILED'::classification_status
		      ELSE 'PENDING'::classification_status
		    END,
		    last_error = $2,
		    updated_at = now()
		WHERE batch_id = $1`, batchID, errMsg, ports.MaxClassificationAttempts)
	return err
}

func (r *Repositories) PersistJokeFits(ctx context.Context, fits []ports.JokeFitMaterialization) error {
	if len(fits) == 0 {
		return nil
	}
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		for _, fit := range fits {
			if err := persistOneJokeFit(ctx, tx, fit); err != nil {
				return err
			}
		}
		return nil
	})
}

func persistOneJokeFit(ctx context.Context, tx pgx.Tx, fit ports.JokeFitMaterialization) error {
	for dim, cat := range fit.Categories {
		if _, err := tx.Exec(ctx, `
			INSERT INTO joke_dimension_values (joke_id, dimension, category)
			VALUES ($1, $2, $3)
			ON CONFLICT (joke_id, dimension) DO UPDATE SET category = EXCLUDED.category`,
			fit.JokeID, string(dim), cat); err != nil {
			return err
		}
	}
	for dim, score := range fit.DimFits {
		if _, err := tx.Exec(ctx, `
			INSERT INTO joke_dim_fit (joke_id, dimension, dim_fit)
			VALUES ($1, $2, $3)
			ON CONFLICT (joke_id, dimension) DO UPDATE SET dim_fit = EXCLUDED.dim_fit`,
			fit.JokeID, string(dim), score); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO joke_fit (joke_id, round_id, true_fit, computed_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (joke_id) DO UPDATE
		SET round_id = EXCLUDED.round_id,
		    true_fit = EXCLUDED.true_fit,
		    computed_at = now()`,
		fit.JokeID, fit.RoundID, fit.TrueFit)
	return err
}

func (r *Repositories) GetJokeFit(ctx context.Context, jokeID int64) (*domain.JokeFit, error) {
	var f domain.JokeFit
	err := r.pg.Pool.QueryRow(ctx, `
		SELECT joke_id, round_id, true_fit, computed_at
		FROM joke_fit WHERE joke_id = $1`, jokeID).Scan(
		&f.JokeID, &f.RoundID, &f.TrueFit, &f.ComputedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("joke_fit")
		}
		return nil, err
	}
	return &f, nil
}

func (r *Repositories) ListJokeDimFits(ctx context.Context, jokeID int64) ([]domain.JokeDimFit, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT joke_id, dimension, dim_fit
		FROM joke_dim_fit WHERE joke_id = $1`, jokeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.JokeDimFit
	for rows.Next() {
		var d domain.JokeDimFit
		var dim string
		if err := rows.Scan(&d.JokeID, &dim, &d.DimFit); err != nil {
			return nil, err
		}
		d.Dimension = domain.Dimension(dim)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repositories) ListJokeDimensionValues(ctx context.Context, jokeID int64) ([]domain.JokeDimensionValue, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT joke_id, dimension, category
		FROM joke_dimension_values WHERE joke_id = $1`, jokeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.JokeDimensionValue
	for rows.Next() {
		var d domain.JokeDimensionValue
		var dim string
		if err := rows.Scan(&d.JokeID, &dim, &d.Category); err != nil {
			return nil, err
		}
		d.Dimension = domain.Dimension(dim)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repositories) ListOrphanClassificationBatchIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT b.batch_id
		FROM batches b
		WHERE b.status = 'PROCESSED'
		  AND EXISTS (
		    SELECT 1 FROM jokes j
		    WHERE j.batch_id = b.batch_id AND j.publish_status = 'PUBLISHED'
		  )
		  AND (
		    NOT EXISTS (
		      SELECT 1 FROM classification_jobs cj WHERE cj.batch_id = b.batch_id
		    )
		    OR EXISTS (
		      SELECT 1 FROM classification_jobs cj
		      WHERE cj.batch_id = b.batch_id
		        AND cj.attempts < $1
		        AND (
		          cj.status IN ('PENDING', 'FAILED')
		          OR (cj.status = 'PROCESSING' AND cj.updated_at < now() - make_interval(secs => $2))
		        )
		    )
		  )
		ORDER BY b.batch_id
		LIMIT $3`,
		ports.MaxClassificationAttempts, int(ports.StaleClassificationAfter.Seconds()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanClassificationJob(row scannable) (*domain.ClassificationJob, error) {
	var j domain.ClassificationJob
	var status string
	err := row.Scan(
		&j.BatchID, &j.RoundID, &status, &j.Attempts, &j.LastError, &j.Model,
		&j.CreatedAt, &j.UpdatedAt, &j.ClassifiedAt,
	)
	if err != nil {
		return nil, err
	}
	j.Status = domain.ClassificationStatus(status)
	return &j, nil
}
