package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

func (r *Repositories) ReplaceAICustomers(ctx context.Context, roundID int64, customers []domain.AICustomer) error {
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM ai_customers WHERE round_id = $1`, roundID); err != nil {
			return err
		}
		for _, c := range customers {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ai_customers (round_id, personal_threshold, starting_budget, remaining_budget)
				VALUES ($1, $2, $3, $4)`,
				roundID, c.PersonalThreshold, c.StartingBudget, c.RemainingBudget); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repositories) ListAICustomers(ctx context.Context, roundID int64) ([]domain.AICustomer, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT ai_customer_id, round_id, personal_threshold, starting_budget, remaining_budget, created_at
		FROM ai_customers WHERE round_id = $1 ORDER BY ai_customer_id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.AICustomer
	for rows.Next() {
		var c domain.AICustomer
		if err := rows.Scan(
			&c.ID, &c.RoundID, &c.PersonalThreshold, &c.StartingBudget, &c.RemainingBudget, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repositories) ListCandidateJokes(ctx context.Context, jokeIDs []int64) ([]ports.CandidateJoke, error) {
	if len(jokeIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT j.joke_id, b.team_id, b.round_id, jf.true_fit
		FROM jokes j
		JOIN batches b ON b.batch_id = j.batch_id
		JOIN joke_fit jf ON jf.joke_id = j.joke_id
		WHERE j.joke_id = ANY($1)
		  AND j.publish_status = 'PUBLISHED'`, jokeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ports.CandidateJoke
	for rows.Next() {
		var c ports.CandidateJoke
		if err := rows.Scan(&c.JokeID, &c.TeamID, &c.RoundID, &c.TrueFit); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repositories) ListHoldings(ctx context.Context, roundID, aiCustomerID int64) ([]ports.HeldJoke, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT p.joke_id, p.team_id, COALESCE(jf.true_fit, 0), p.price
		FROM purchases p
		LEFT JOIN joke_fit jf ON jf.joke_id = p.joke_id
		WHERE p.round_id = $1 AND p.ai_customer_id = $2
		ORDER BY p.joke_id`, roundID, aiCustomerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ports.HeldJoke
	for rows.Next() {
		var h ports.HeldJoke
		if err := rows.Scan(&h.JokeID, &h.TeamID, &h.TrueFit, &h.Price); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *Repositories) BuyJoke(
	ctx context.Context,
	roundID, aiCustomerID, jokeID, teamID int64,
	price float64,
) error {
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		var remaining float64
		err := tx.QueryRow(ctx, `
			SELECT remaining_budget FROM ai_customers
			WHERE ai_customer_id = $1 AND round_id = $2
			FOR UPDATE`, aiCustomerID, roundID).Scan(&remaining)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NewNotFoundError("ai_customer")
			}
			return err
		}
		if remaining < price {
			return domain.NewConflictError("insufficient budget")
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO purchases (round_id, ai_customer_id, joke_id, team_id, price)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (round_id, ai_customer_id, joke_id) DO NOTHING`,
			roundID, aiCustomerID, jokeID, teamID, price)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already held
		}

		if _, err := tx.Exec(ctx, `
			UPDATE ai_customers SET remaining_budget = remaining_budget - $2
			WHERE ai_customer_id = $1`, aiCustomerID, price); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_events (round_id, ai_customer_id, joke_id, team_id, delta, price)
			VALUES ($1, $2, $3, $4, 1, $5)`,
			roundID, aiCustomerID, jokeID, teamID, price); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE team_rounds_state
			SET points_earned = points_earned + 1, updated_at = now()
			WHERE round_id = $1 AND team_id = $2`, roundID, teamID)
		return err
	})
}

func (r *Repositories) SwapJoke(
	ctx context.Context,
	roundID, aiCustomerID, buyJokeID, buyTeamID, returnJokeID, returnTeamID int64,
	price float64,
) error {
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			SELECT ai_customer_id FROM ai_customers
			WHERE ai_customer_id = $1 AND round_id = $2
			FOR UPDATE`, aiCustomerID, roundID); err != nil {
			return err
		}

		tag, err := tx.Exec(ctx, `
			DELETE FROM purchases
			WHERE round_id = $1 AND ai_customer_id = $2 AND joke_id = $3`,
			roundID, aiCustomerID, returnJokeID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.NewConflictError("held joke not found for swap")
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_events (round_id, ai_customer_id, joke_id, team_id, delta, price)
			VALUES ($1, $2, $3, $4, -1, $5)`,
			roundID, aiCustomerID, returnJokeID, returnTeamID, price); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE team_rounds_state
			SET points_earned = GREATEST(points_earned - 1, 0), updated_at = now()
			WHERE round_id = $1 AND team_id = $2`, roundID, returnTeamID); err != nil {
			return err
		}

		// Budget unchanged on swap (same market_price in/out).
		tag, err = tx.Exec(ctx, `
			INSERT INTO purchases (round_id, ai_customer_id, joke_id, team_id, price)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (round_id, ai_customer_id, joke_id) DO NOTHING`,
			roundID, aiCustomerID, buyJokeID, buyTeamID, price)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.NewConflictError("already holds buy target")
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_events (round_id, ai_customer_id, joke_id, team_id, delta, price)
			VALUES ($1, $2, $3, $4, 1, $5)`,
			roundID, aiCustomerID, buyJokeID, buyTeamID, price); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE team_rounds_state
			SET points_earned = points_earned + 1, updated_at = now()
			WHERE round_id = $1 AND team_id = $2`, roundID, buyTeamID)
		return err
	})
}

func (r *Repositories) ListMarket(ctx context.Context, roundID int64) ([]ports.MarketJoke, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT j.joke_id, j.joke_text, j.joke_title, b.team_id, t.name,
		       COALESCE(sc.sold_count, 0), j.published_at
		FROM jokes j
		JOIN batches b ON b.batch_id = j.batch_id
		JOIN teams t ON t.id = b.team_id
		LEFT JOIN (
		  SELECT joke_id, COUNT(*)::int AS sold_count
		  FROM purchases
		  WHERE round_id = $1
		  GROUP BY joke_id
		) sc ON sc.joke_id = j.joke_id
		WHERE b.round_id = $1 AND j.publish_status = 'PUBLISHED'
		ORDER BY j.published_at ASC NULLS LAST, j.joke_id ASC`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ports.MarketJoke
	for rows.Next() {
		var m ports.MarketJoke
		if err := rows.Scan(
			&m.JokeID, &m.JokeText, &m.JokeTitle, &m.TeamID, &m.TeamName,
			&m.SoldCount, &m.PublishedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
