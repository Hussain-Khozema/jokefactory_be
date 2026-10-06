package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// leanTeamCTE computes, per team of round $1, everything Lean mode adds, and
// ends in `profits`: one row per team_rounds_state row, with the Classic
// counts, the Lean profit and the five Lean columns. The Classic queries in
// stats_repo.go are not touched.
//
//	cfg         round settings; clock_end = ended_at, or now() while running
//	submitted   batches_submitted = +1 batch_submission_events (a submit);
//	            the -1 events record processing, so they are not counted
//	batch_jokes one row per submitted batch with its joke-row count
//	wip         Σ jokes × minutes(submitted_at → processed_at or clock_end),
//	            an unsplit batch (0 joke rows) counting batch_size jokes;
//	            plus the mean submit → processed seconds of processed batches
//	first_sale  each joke's earliest +1 purchase event
//	inventory   Σ over PUBLISHED jokes of minutes(published_at → first sale
//	            or clock_end); discarded jokes never carry; plus the mean
//	            publish → first sale seconds of sold jokes
//	lean        setup_cost = batches_submitted × setup_cost_per_batch,
//	            carrying_cost = carrying_cost_per_joke_minute × (wip + inventory)
//	profits     Classic profit − setup_cost − carrying_cost
const leanTeamCTE = `
WITH cfg AS (
	SELECT market_price, cost_of_publishing, cost_of_discard, batch_size,
	       setup_cost_per_batch::float8          AS setup_cost_per_batch,
	       carrying_cost_per_joke_minute::float8 AS carrying_cost_per_joke_minute,
	       COALESCE(ended_at, now())             AS clock_end
	FROM rounds WHERE round_id = $1
),
submitted AS (
	SELECT team_id, COUNT(*)::int AS batches_submitted
	FROM batch_submission_events
	WHERE round_id = $1 AND delta > 0
	GROUP BY team_id
),
batch_jokes AS (
	SELECT b.batch_id, b.team_id, b.submitted_at, b.processed_at,
	       COUNT(j.joke_id)::int AS joke_rows
	FROM batches b
	LEFT JOIN jokes j ON j.batch_id = b.batch_id
	WHERE b.round_id = $1 AND b.submitted_at IS NOT NULL
	GROUP BY b.batch_id
),
wip AS (
	SELECT bj.team_id,
	       SUM(
	         (CASE WHEN bj.joke_rows > 0 THEN bj.joke_rows ELSE c.batch_size END)
	         * GREATEST(EXTRACT(EPOCH FROM (COALESCE(bj.processed_at, c.clock_end) - bj.submitted_at))::float8, 0)
	         / 60.0
	       ) AS wip_joke_minutes,
	       AVG(EXTRACT(EPOCH FROM (bj.processed_at - bj.submitted_at))::float8)
	         FILTER (WHERE bj.processed_at IS NOT NULL) AS avg_submit_to_publish_s
	FROM batch_jokes bj CROSS JOIN cfg c
	GROUP BY bj.team_id
),
first_sale AS (
	SELECT joke_id, MIN(created_at) AS first_sold_at
	FROM purchase_events
	WHERE round_id = $1 AND delta > 0
	GROUP BY joke_id
),
inventory AS (
	SELECT b.team_id,
	       SUM(
	         GREATEST(EXTRACT(EPOCH FROM (COALESCE(fs.first_sold_at, c.clock_end) - j.published_at))::float8, 0)
	         / 60.0
	       ) AS inventory_joke_minutes,
	       AVG(EXTRACT(EPOCH FROM (fs.first_sold_at - j.published_at))::float8)
	         FILTER (WHERE fs.first_sold_at IS NOT NULL) AS avg_publish_to_first_sale_s
	FROM jokes j
	JOIN batches b ON b.batch_id = j.batch_id
	LEFT JOIN first_sale fs ON fs.joke_id = j.joke_id
	CROSS JOIN cfg c
	WHERE b.round_id = $1 AND j.publish_status = 'PUBLISHED' AND j.published_at IS NOT NULL
	GROUP BY b.team_id
),
lean AS (
	SELECT trs.team_id,
	       COALESCE(s.batches_submitted, 0) AS batches_submitted,
	       COALESCE(s.batches_submitted, 0) * c.setup_cost_per_batch AS setup_cost,
	       c.carrying_cost_per_joke_minute
	         * (COALESCE(w.wip_joke_minutes, 0) + COALESCE(i.inventory_joke_minutes, 0)) AS carrying_cost,
	       COALESCE(w.avg_submit_to_publish_s, 0)     AS avg_submit_to_publish_s,
	       COALESCE(i.avg_publish_to_first_sale_s, 0) AS avg_publish_to_first_sale_s
	FROM team_rounds_state trs
	CROSS JOIN cfg c
	LEFT JOIN submitted s ON s.team_id = trs.team_id
	LEFT JOIN wip w       ON w.team_id = trs.team_id
	LEFT JOIN inventory i ON i.team_id = trs.team_id
	WHERE trs.round_id = $1
),
profits AS (
	SELECT trs.team_id,
	       t.name AS team_name,
	       trs.batches_created,
	       trs.batches_processed,
	       trs.points_earned AS total_sales,
	       trs.published_jokes,
	       trs.discarded_jokes,
	       trs.published_jokes + trs.discarded_jokes AS total_jokes,
	       GREATEST(trs.published_jokes - trs.points_earned, 0) AS unsold_jokes,
	       (SELECT market_price FROM cfg) * trs.points_earned::float8
	         - (SELECT cost_of_publishing FROM cfg) * trs.published_jokes::float8
	         - (SELECT cost_of_discard FROM cfg) * trs.discarded_jokes::float8
	         - l.setup_cost - l.carrying_cost AS profit,
	       l.batches_submitted, l.setup_cost, l.carrying_cost,
	       l.avg_submit_to_publish_s, l.avg_publish_to_first_sale_s
	FROM team_rounds_state trs
	JOIN teams t ON t.id = trs.team_id
	JOIN lean l  ON l.team_id = trs.team_id
	WHERE trs.round_id = $1
)`

// batchSeriesSQL lists every submitted batch of round $1 (of team $2, or of
// every team when $2 is 0) in submission order, with its joke rows, published
// jokes and jokes ever sold (any +1 purchase event).
const batchSeriesSQL = `
SELECT b.team_id, b.submitted_at,
       (SELECT COUNT(*) FROM jokes j WHERE j.batch_id = b.batch_id)::int AS jokes,
       (SELECT COUNT(*) FROM jokes j
         WHERE j.batch_id = b.batch_id AND j.publish_status = 'PUBLISHED')::int AS published,
       (SELECT COUNT(DISTINCT pe.joke_id) FROM purchase_events pe
         JOIN jokes j ON j.joke_id = pe.joke_id
         WHERE j.batch_id = b.batch_id AND pe.round_id = $1 AND pe.delta > 0)::int AS sold
FROM batches b
WHERE b.round_id = $1 AND b.submitted_at IS NOT NULL
  AND ($2::bigint = 0 OR b.team_id = $2::bigint)
ORDER BY b.team_id, b.submitted_at, b.batch_id`

func (r *Repositories) GetRoundStatsLean(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	rows, err := r.pg.Pool.Query(ctx, leanTeamCTE+`
		SELECT
			DENSE_RANK() OVER (ORDER BY profit DESC) AS rnk,
			team_id, team_name, batches_processed, total_sales,
			published_jokes, discarded_jokes, total_jokes, unsold_jokes, profit,
			batches_submitted, setup_cost, carrying_cost,
			avg_submit_to_publish_s, avg_publish_to_first_sale_s
		FROM profits
		ORDER BY profit DESC, team_id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &ports.RoundStats{RoundID: roundID, Leaderboard: []ports.TeamStats{}}
	for rows.Next() {
		ts := ports.TeamStats{Lean: &ports.LeanTeamStats{BatchSeries: []ports.BatchPoint{}}}
		if err := rows.Scan(
			&ts.Rank, &ts.Team.ID, &ts.Team.Name, &ts.BatchesProcessed, &ts.TotalSales,
			&ts.PublishedJokes, &ts.DiscardedJokes, &ts.TotalJokes, &ts.UnsoldJokes, &ts.Profit,
			&ts.Lean.BatchesSubmitted, &ts.Lean.SetupCost, &ts.Lean.CarryingCost,
			&ts.Lean.AvgSubmitToPublishS, &ts.Lean.AvgPublishToFirstSaleS,
		); err != nil {
			return nil, err
		}
		stats.Leaderboard = append(stats.Leaderboard, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	series, err := r.leanBatchSeries(ctx, roundID, 0)
	if err != nil {
		return nil, err
	}
	for i := range stats.Leaderboard {
		if s, ok := series[stats.Leaderboard[i].Team.ID]; ok {
			stats.Leaderboard[i].Lean.BatchSeries = s
		}
	}
	return stats, nil
}

// GetTeamSummaryLean mirrors GetTeamSummary's output columns and scan order,
// with Lean profit and rank, and appends the five Lean columns. Like the
// Classic query it returns zeros for a team with no team_rounds_state row.
func (r *Repositories) GetTeamSummaryLean(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	q := leanTeamCTE + `,
		unprocessed AS (
			SELECT COUNT(*) AS cnt
			FROM batches
			WHERE round_id = $1 AND team_id = $2 AND status = 'SUBMITTED'
		),
		ranks AS (
			SELECT team_id, DENSE_RANK() OVER (ORDER BY profit DESC) AS rnk
			FROM profits
		)
		SELECT t.id, t.name, $1,
		       COALESCE(r.rnk, 1),
		       COALESCE(p.total_sales, 0),
		       COALESCE(p.profit, 0),
		       COALESCE(p.total_sales, 0),
		       COALESCE(p.batches_created, 0),
		       COALESCE(p.batches_processed, 0),
		       COALESCE(p.published_jokes, 0),
		       COALESCE(p.discarded_jokes, 0),
		       COALESCE(p.unsold_jokes, 0),
		       LEAST(COALESCE(p.published_jokes, 0), COALESCE(p.total_sales, 0)),
		       COALESCE(u.cnt, 0),
		       COALESCE(p.total_jokes, 0),
		       COALESCE(p.published_jokes, 0),
		       COALESCE(p.batches_submitted, 0),
		       COALESCE(p.setup_cost, 0),
		       COALESCE(p.carrying_cost, 0),
		       COALESCE(p.avg_submit_to_publish_s, 0),
		       COALESCE(p.avg_publish_to_first_sale_s, 0)
		FROM teams t
		LEFT JOIN profits p ON p.team_id = t.id
		LEFT JOIN ranks r   ON r.team_id = t.id
		LEFT JOIN unprocessed u ON true
		WHERE t.id = $2`
	summary := ports.TeamSummary{Lean: &ports.LeanTeamStats{BatchSeries: []ports.BatchPoint{}}}
	err := r.pg.Pool.QueryRow(ctx, q, roundID, teamID).Scan(
		&summary.Team.ID, &summary.Team.Name, &summary.RoundID,
		&summary.Rank, &summary.Points, &summary.Profit, &summary.TotalSales,
		&summary.BatchesCreated, &summary.BatchesProcessed, &summary.PublishedJokes, &summary.DiscardedJokes,
		&summary.UnsoldJokes, &summary.SoldJokesCount, &summary.UnprocessedBatches,
		&summary.JokesCreated, &summary.JokesPublished,
		&summary.Lean.BatchesSubmitted, &summary.Lean.SetupCost, &summary.Lean.CarryingCost,
		&summary.Lean.AvgSubmitToPublishS, &summary.Lean.AvgPublishToFirstSaleS,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("team summary")
		}
		return nil, err
	}
	summary.Performance = "AVERAGE PERFORMING"

	series, err := r.leanBatchSeries(ctx, roundID, teamID)
	if err != nil {
		return nil, err
	}
	if s, ok := series[teamID]; ok {
		summary.Lean.BatchSeries = s
	}
	return &summary, nil
}

// leanBatchSeries runs batchSeriesSQL and groups it by team, numbering each
// team's batches from 1. teamID 0 means every team.
func (r *Repositories) leanBatchSeries(ctx context.Context, roundID, teamID int64) (map[int64][]ports.BatchPoint, error) {
	rows, err := r.pg.Pool.Query(ctx, batchSeriesSQL, roundID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64][]ports.BatchPoint)
	for rows.Next() {
		var (
			team int64
			p    ports.BatchPoint
			at   time.Time
		)
		if err := rows.Scan(&team, &at, &p.Jokes, &p.Published, &p.Sold); err != nil {
			return nil, err
		}
		p.SubmittedAt = at
		p.Seq = len(out[team]) + 1
		out[team] = append(out[team], p)
	}
	return out, rows.Err()
}
