package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

func (r *Repositories) ClaimNextBatch(ctx context.Context, roundID, teamID, marketerID int64) (*ports.BatchWithJokes, error) {
	var out *ports.BatchWithJokes
	err := r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		b, err := loadHeldOrClaimBatch(ctx, tx, roundID, teamID, marketerID)
		if err != nil {
			return err
		}
		if b == nil {
			return nil
		}
		jokes, err := listJokesTx(ctx, tx, b.ID)
		if err != nil {
			return err
		}
		out = &ports.BatchWithJokes{Batch: *b, Jokes: jokes}
		return nil
	})
	return out, err
}

func loadHeldOrClaimBatch(ctx context.Context, tx pgx.Tx, roundID, teamID, marketerID int64) (*domain.Batch, error) {
	b, err := scanBatch(tx.QueryRow(ctx, `
		SELECT `+batchColumns+`
		FROM batches
		WHERE round_id = $1 AND team_id = $2 AND status = 'SUBMITTED' AND locked_by = $3
		ORDER BY submitted_at ASC NULLS LAST, batch_id ASC
		LIMIT 1
		FOR UPDATE`, roundID, teamID, marketerID))
	if err == nil {
		return b, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	b, err = scanBatch(tx.QueryRow(ctx, `
		SELECT `+batchColumns+`
		FROM batches
		WHERE round_id = $1 AND team_id = $2 AND status = 'SUBMITTED'
		  AND (locked_by IS NULL OR locked_at < now() - interval '15 minutes')
		ORDER BY submitted_at ASC NULLS LAST, batch_id ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`, roundID, teamID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // empty queue is a valid outcome
		}
		return nil, err
	}
	return scanBatch(tx.QueryRow(ctx, `
		UPDATE batches
		SET locked_by = $2, locked_at = now()
		WHERE batch_id = $1
		RETURNING `+batchColumns, b.ID, marketerID))
}

func (r *Repositories) CountSubmittedBatchesForTeam(ctx context.Context, roundID, teamID int64) (int, error) {
	var count int
	err := r.pg.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM batches
		WHERE round_id = $1 AND team_id = $2 AND status = 'SUBMITTED'`, roundID, teamID).Scan(&count)
	return count, err
}

func (r *Repositories) PublishBatch(
	ctx context.Context,
	batchID, marketerID, teamID int64,
	decisions []ports.JokePublishDecision,
) (*ports.PublishResult, error) {
	var result *ports.PublishResult
	err := r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := lockBatchForPublish(ctx, tx, batchID, marketerID, teamID); err != nil {
			return err
		}
		jokes, err := listJokesTx(ctx, tx, batchID)
		if err != nil {
			return err
		}
		published, discarded, err := applyPublishDecisions(ctx, tx, jokes, decisions)
		if err != nil {
			return err
		}
		processed, err := markBatchProcessed(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if err := bumpTeamPublishCounters(ctx, tx, processed, len(published), len(discarded), len(jokes)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO classification_jobs (batch_id, round_id, status)
			VALUES ($1, $2, 'PENDING')
			ON CONFLICT (batch_id) DO NOTHING`, processed.ID, processed.RoundID); err != nil {
			return err
		}
		result = &ports.PublishResult{
			Batch:        *processed,
			PublishedIDs: published,
			DiscardedIDs: discarded,
		}
		return nil
	})
	return result, err
}

func lockBatchForPublish(ctx context.Context, tx pgx.Tx, batchID, marketerID, teamID int64) (*domain.Batch, error) {
	b, err := scanBatch(tx.QueryRow(ctx, `
		SELECT `+batchColumns+` FROM batches WHERE batch_id = $1 FOR UPDATE`, batchID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("batch")
		}
		return nil, err
	}
	if b.TeamID != teamID {
		return nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	if b.Status == domain.BatchProcessed {
		return nil, domain.NewConflictError("BATCH_ALREADY_PROCESSED")
	}
	if b.Status != domain.BatchSubmitted {
		return nil, domain.NewConflictError("batch not submitted")
	}
	if b.LockedBy == nil || *b.LockedBy != marketerID {
		return nil, domain.NewForbiddenError("NOT_ASSIGNED_TO_THIS_MARKETER")
	}
	return b, nil
}

func applyPublishDecisions(
	ctx context.Context,
	tx pgx.Tx,
	jokes []domain.Joke,
	decisions []ports.JokePublishDecision,
) (published, discarded []int64, err error) {
	byID := make(map[int64]domain.Joke, len(jokes))
	for _, j := range jokes {
		byID[j.ID] = j
	}
	if len(decisions) != len(jokes) {
		return nil, nil, domain.NewValidationError("jokes", fmt.Sprintf("expected %d joke decisions", len(jokes)))
	}

	published = make([]int64, 0, len(decisions))
	discarded = make([]int64, 0, len(decisions))
	seen := make(map[int64]struct{}, len(decisions))

	for _, d := range decisions {
		if _, ok := byID[d.JokeID]; !ok {
			return nil, nil, domain.NewValidationError("jokes", fmt.Sprintf("joke %d not in batch", d.JokeID))
		}
		if _, dup := seen[d.JokeID]; dup {
			return nil, nil, domain.NewValidationError("jokes", fmt.Sprintf("duplicate joke_id %d", d.JokeID))
		}
		seen[d.JokeID] = struct{}{}

		if d.IsPublished {
			if d.Title == "" {
				return nil, nil, domain.NewValidationError("joke_title", "title required for published jokes")
			}
			if _, err := tx.Exec(ctx, `
				UPDATE jokes
				SET joke_title = $2, publish_status = 'PUBLISHED', published_at = now()
				WHERE joke_id = $1`, d.JokeID, d.Title); err != nil {
				return nil, nil, err
			}
			published = append(published, d.JokeID)
			continue
		}

		var titleArg any
		if d.Title != "" {
			titleArg = d.Title
		}
		if _, err := tx.Exec(ctx, `
			UPDATE jokes
			SET joke_title = COALESCE($2, joke_title),
			    publish_status = 'DISCARDED',
			    published_at = NULL
			WHERE joke_id = $1`, d.JokeID, titleArg); err != nil {
			return nil, nil, err
		}
		discarded = append(discarded, d.JokeID)
	}
	if len(published) == 0 {
		return nil, nil, domain.NewValidationError("jokes", "NO_JOKE_PUBLISHED")
	}
	return published, discarded, nil
}

func markBatchProcessed(ctx context.Context, tx pgx.Tx, batchID int64) (*domain.Batch, error) {
	return scanBatch(tx.QueryRow(ctx, `
		UPDATE batches
		SET status = 'PROCESSED',
		    processed_at = now(),
		    locked_by = NULL,
		    locked_at = NULL
		WHERE batch_id = $1
		RETURNING `+batchColumns, batchID))
}

func bumpTeamPublishCounters(ctx context.Context, tx pgx.Tx, batch *domain.Batch, published, discarded, jokeCount int) error {
	if _, err := tx.Exec(ctx, `
		UPDATE team_rounds_state
		SET batches_processed = batches_processed + 1,
		    published_jokes = published_jokes + $3,
		    discarded_jokes = discarded_jokes + $4,
		    updated_at = now()
		WHERE round_id = $1 AND team_id = $2`,
		batch.RoundID, batch.TeamID, published, discarded); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO batch_submission_events (round_id, team_id, batch_id, jokes_count, delta)
		VALUES ($1, $2, $3, $4, -1)`,
		batch.RoundID, batch.TeamID, batch.ID, jokeCount)
	return err
}

func listJokesTx(ctx context.Context, tx pgx.Tx, batchID int64) ([]domain.Joke, error) {
	rows, err := tx.Query(ctx, `
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
	return jokes, rows.Err()
}
