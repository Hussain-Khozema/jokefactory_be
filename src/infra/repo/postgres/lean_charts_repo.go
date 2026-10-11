package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// leanChartFirstSaleCTE is each joke's earliest +1 purchase event in round $1
// (the same rule as first_sale in leanTeamCTE: a return does not undo a sale).
const leanChartFirstSaleCTE = `
WITH first_sale AS (
	SELECT joke_id, MIN(created_at) AS first_sold_at
	FROM purchase_events
	WHERE round_id = $1 AND delta > 0
	GROUP BY joke_id
)`

// leanChartTeamsSQL is one row per team_rounds_state row of round $1 with the
// totals the lean charts need, from the same CTE as the leaderboard.
const leanChartTeamsSQL = leanTeamCTE + `
SELECT team_id, team_name, batches_submitted, discarded_jokes, shipping_cost,
       wip_joke_minutes, inventory_joke_minutes
FROM profits
ORDER BY team_id`

// leanChartBatchesSQL lists every submitted batch of round $1 with its joke
// rows, published jokes, jokes ever sold and first sale. first_sale has one
// row per joke, so the LEFT JOINs never fan a joke out.
const leanChartBatchesSQL = leanChartFirstSaleCTE + `
SELECT b.batch_id, b.team_id, b.submitted_at, b.processed_at,
       COUNT(j.joke_id)::int                                         AS joke_rows,
       (COUNT(*) FILTER (WHERE j.publish_status = 'PUBLISHED'))::int AS published,
       COUNT(fs.joke_id)::int                                        AS sold,
       MIN(fs.first_sold_at)                                         AS first_sold_at
FROM batches b
LEFT JOIN jokes j       ON j.batch_id = b.batch_id
LEFT JOIN first_sale fs ON fs.joke_id = j.joke_id
WHERE b.round_id = $1 AND b.submitted_at IS NOT NULL
GROUP BY b.batch_id
ORDER BY b.team_id, b.submitted_at, b.batch_id`

// leanChartJokesSQL lists every PUBLISHED joke of round $1 with its first sale
// (the inventory CTE's filter: discarded jokes never sit on the shelf).
const leanChartJokesSQL = leanChartFirstSaleCTE + `
SELECT b.team_id, j.published_at, fs.first_sold_at
FROM jokes j
JOIN batches b          ON b.batch_id = j.batch_id
LEFT JOIN first_sale fs ON fs.joke_id = j.joke_id
WHERE b.round_id = $1 AND j.publish_status = 'PUBLISHED' AND j.published_at IS NOT NULL
ORDER BY b.team_id, j.published_at, j.joke_id`

// GetLeanChartRows reads the lean charts' rows in one read-only REPEATABLE
// READ transaction: every query sees the same rows and the same now(), so a
// running round's clock end, idle samples and storage and hosting minutes stop
// at one instant. Nothing is written.
func (r *Repositories) GetLeanChartRows(ctx context.Context, roundID int64) (*ports.LeanChartRows, error) {
	tx, err := r.pg.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out := &ports.LeanChartRows{}
	err = tx.QueryRow(ctx, `SELECT COALESCE(ended_at, now()) FROM rounds WHERE round_id = $1`, roundID).Scan(&out.ClockEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NewNotFoundError("round")
	}
	if err != nil {
		return nil, err
	}
	out.ClockEnd = out.ClockEnd.UTC()
	if out.Teams, err = leanChartTeams(ctx, tx, roundID); err != nil {
		return nil, err
	}
	if out.Batches, err = leanChartBatches(ctx, tx, roundID); err != nil {
		return nil, err
	}
	if out.Jokes, err = leanChartJokes(ctx, tx, roundID); err != nil {
		return nil, err
	}
	return out, nil
}

func leanChartTeams(ctx context.Context, tx pgx.Tx, roundID int64) ([]ports.LeanChartTeamRow, error) {
	rows, err := tx.Query(ctx, leanChartTeamsSQL, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ports.LeanChartTeamRow{}
	for rows.Next() {
		var t ports.LeanChartTeamRow
		if err := rows.Scan(&t.TeamID, &t.TeamName, &t.BatchesSubmitted, &t.DiscardedJokes, &t.ShippingCost,
			&t.WIPJokeMinutes, &t.InventoryJokeMinutes); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func leanChartBatches(ctx context.Context, tx pgx.Tx, roundID int64) ([]ports.LeanChartBatchRow, error) {
	rows, err := tx.Query(ctx, leanChartBatchesSQL, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ports.LeanChartBatchRow{}
	for rows.Next() {
		var b ports.LeanChartBatchRow
		if err := rows.Scan(&b.BatchID, &b.TeamID, &b.SubmittedAt, &b.ProcessedAt,
			&b.JokeRows, &b.Published, &b.Sold, &b.FirstSoldAt); err != nil {
			return nil, err
		}
		b.SubmittedAt = b.SubmittedAt.UTC()
		if b.ProcessedAt != nil {
			p := b.ProcessedAt.UTC()
			b.ProcessedAt = &p
		}
		if b.FirstSoldAt != nil {
			f := b.FirstSoldAt.UTC()
			b.FirstSoldAt = &f
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func leanChartJokes(ctx context.Context, tx pgx.Tx, roundID int64) ([]ports.LeanChartJokeRow, error) {
	rows, err := tx.Query(ctx, leanChartJokesSQL, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ports.LeanChartJokeRow{}
	for rows.Next() {
		var j ports.LeanChartJokeRow
		if err := rows.Scan(&j.TeamID, &j.PublishedAt, &j.FirstSoldAt); err != nil {
			return nil, err
		}
		j.PublishedAt = j.PublishedAt.UTC()
		if j.FirstSoldAt != nil {
			f := j.FirstSoldAt.UTC()
			j.FirstSoldAt = &f
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
