package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
)

func (r *Repositories) GetActiveRound(ctx context.Context) (*domain.Round, error) {
	rd, err := scanRound(r.pg.Pool.QueryRow(ctx, `
		SELECT `+roundColumns+` FROM rounds WHERE status = 'ACTIVE' LIMIT 1`))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rd, nil
}

func (r *Repositories) GetRoundByID(ctx context.Context, roundID int64) (*domain.Round, error) {
	rd, err := scanRound(r.pg.Pool.QueryRow(ctx, `
		SELECT `+roundColumns+` FROM rounds WHERE round_id = $1`, roundID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return rd, nil
}

func (r *Repositories) GetLatestRound(ctx context.Context) (*domain.Round, error) {
	rd, err := scanRound(r.pg.Pool.QueryRow(ctx, `
		SELECT `+roundColumns+` FROM rounds ORDER BY round_id DESC LIMIT 1`))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rd, nil
}

func (r *Repositories) ListRounds(ctx context.Context) ([]domain.Round, error) {
	rows, err := r.pg.Pool.Query(ctx, `SELECT `+roundColumns+` FROM rounds ORDER BY round_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []domain.Round
	for rows.Next() {
		rd, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		rounds = append(rounds, *rd)
	}
	return rounds, rows.Err()
}

func (r *Repositories) UpdateRoundConfig(ctx context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error) {
	rd, err := scanRound(r.pg.Pool.QueryRow(ctx, `
		UPDATE rounds
		SET customer_budget = $2,
		    batch_size = $3,
		    market_price = $4,
		    cost_of_publishing = $5,
		    cost_of_discard = $6,
		    customer_count = $7,
		    buy_threshold = $8,
		    jitter = $9,
		    swap_margin = $10,
		    feedback_joke_count = $11,
		    feedback_pass_threshold = $12
		WHERE round_id = $1
		RETURNING `+roundColumns, roundID,
		cfg.CustomerBudget, cfg.BatchSize, cfg.MarketPrice, cfg.CostOfPublishing,
		cfg.CostOfDiscard, cfg.CustomerCount, cfg.BuyThreshold, cfg.Jitter,
		cfg.SwapMargin, cfg.FeedbackJokeCount, cfg.FeedbackPassThreshold))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return rd, nil
}

func (r *Repositories) InsertRoundConfig(ctx context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error) {
	rd, err := r.UpdateRoundConfig(ctx, roundID, cfg)
	if err == nil {
		return rd, nil
	}
	if !domain.IsNotFound(err) {
		return nil, err
	}

	return scanRound(r.pg.Pool.QueryRow(ctx, `
		INSERT INTO rounds (
			round_id, round_number, status,
			customer_budget, batch_size, market_price, cost_of_publishing,
			cost_of_discard, customer_count, buy_threshold, jitter, swap_margin,
			feedback_joke_count, feedback_pass_threshold
		)
		VALUES ($1, $2, 'CONFIGURED', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+roundColumns,
		roundID, int(roundID),
		cfg.CustomerBudget, cfg.BatchSize, cfg.MarketPrice, cfg.CostOfPublishing,
		cfg.CostOfDiscard, cfg.CustomerCount, cfg.BuyThreshold, cfg.Jitter,
		cfg.SwapMargin, cfg.FeedbackJokeCount, cfg.FeedbackPassThreshold))
}

func (r *Repositories) UpsertIdealProfile(ctx context.Context, roundID int64, profile domain.IdealProfile) error {
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM round_ideal_profile WHERE round_id = $1`, roundID); err != nil {
			return err
		}
		for dim, cat := range profile {
			if _, err := tx.Exec(ctx, `
				INSERT INTO round_ideal_profile (round_id, dimension, ideal_category)
				VALUES ($1, $2, $3)`, roundID, string(dim), cat); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repositories) GetIdealProfile(ctx context.Context, roundID int64) (domain.IdealProfile, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT dimension, ideal_category
		FROM round_ideal_profile
		WHERE round_id = $1`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profile := make(domain.IdealProfile)
	for rows.Next() {
		var dim, cat string
		if err := rows.Scan(&dim, &cat); err != nil {
			return nil, err
		}
		profile[domain.Dimension(dim)] = cat
	}
	return profile, rows.Err()
}

func (r *Repositories) StartRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	var rd *domain.Round
	err := r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		rd, err = scanRound(tx.QueryRow(ctx, `
			UPDATE rounds
			SET status = 'ACTIVE',
			    started_at = COALESCE(started_at, now()),
			    ended_at = NULL
			WHERE round_id = $1
			RETURNING `+roundColumns, roundID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NewNotFoundError("round")
			}
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO team_rounds_state (round_id, team_id)
			SELECT $1, t.id FROM teams t
			ON CONFLICT DO NOTHING`, roundID)
		return err
	})
	return rd, err
}

func (r *Repositories) EndRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	rd, err := scanRound(r.pg.Pool.QueryRow(ctx, `
		UPDATE rounds
		SET status = 'ENDED', ended_at = now()
		WHERE round_id = $1
		RETURNING `+roundColumns, roundID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return rd, nil
}

func (r *Repositories) SetRoundPopupState(ctx context.Context, roundID int64, isActive bool) (*domain.Round, error) {
	rd, err := scanRound(r.pg.Pool.QueryRow(ctx, `
		UPDATE rounds
		SET is_popped_active = $2
		WHERE round_id = $1
		RETURNING `+roundColumns, roundID, isActive))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return rd, nil
}

func (r *Repositories) EnsureTeamRoundState(ctx context.Context, roundID, teamID int64) error {
	_, err := r.pg.Pool.Exec(ctx, `
		INSERT INTO team_rounds_state (round_id, team_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, roundID, teamID)
	return err
}

func (r *Repositories) ResetGame(ctx context.Context) error {
	cfg := domain.DefaultRoundConfig()
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			TRUNCATE TABLE
				purchase_events,
				purchases,
				ai_customers,
				classification_jobs,
				joke_fit,
				joke_dim_fit,
				joke_dimension_values,
				batch_submission_events,
				round_ideal_profile,
				jokes,
				batches,
				team_rounds_state
			RESTART IDENTITY CASCADE`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rounds
			SET status = 'CONFIGURED',
			    customer_budget = $1,
			    batch_size = $2,
			    market_price = $3,
			    cost_of_publishing = $4,
			    cost_of_discard = $5,
			    customer_count = $6,
			    buy_threshold = $7,
			    jitter = $8,
			    swap_margin = $9,
			    feedback_joke_count = $10,
			    feedback_pass_threshold = $11,
			    started_at = NULL,
			    ended_at = NULL,
			    is_popped_active = FALSE`,
			cfg.CustomerBudget,
			cfg.BatchSize,
			cfg.MarketPrice,
			cfg.CostOfPublishing,
			cfg.CostOfDiscard,
			cfg.CustomerCount,
			cfg.BuyThreshold,
			cfg.Jitter,
			cfg.SwapMargin,
			cfg.FeedbackJokeCount,
			cfg.FeedbackPassThreshold,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM users WHERE role IS DISTINCT FROM 'INSTRUCTOR'`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM teams`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `ALTER SEQUENCE teams_id_seq RESTART WITH 1`)
		return err
	})
}
