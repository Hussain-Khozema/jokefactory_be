package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

func (r *Repositories) GetLobby(ctx context.Context, roundID int64) (*ports.LobbySnapshot, error) {
	snapshot := &ports.LobbySnapshot{RoundID: roundID}

	if err := r.pg.Pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users u
				WHERE u.status = 'WAITING' AND (u.role IS NULL OR u.role <> 'INSTRUCTOR')),
			(SELECT COUNT(*) FROM users u
				WHERE u.status = 'ASSIGNED' AND (u.role IS NULL OR u.role <> 'INSTRUCTOR'))
	`).Scan(&snapshot.Summary.Waiting, &snapshot.Summary.Assigned); err != nil {
		return nil, err
	}

	teamRows, err := r.pg.Pool.Query(ctx, `SELECT id, name, created_at FROM teams ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer teamRows.Close()

	for teamRows.Next() {
		var t domain.Team
		if err := teamRows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		members, err := r.ListTeamMembers(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		if len(members) > 0 {
			snapshot.Teams = append(snapshot.Teams, ports.LobbyTeam{Team: t, Members: members})
		}
	}
	if err := teamRows.Err(); err != nil {
		return nil, err
	}
	snapshot.Summary.TeamCount = len(snapshot.Teams)

	waiting, err := r.ListUsersByStatus(ctx, domain.ParticipantWaiting)
	if err != nil {
		return nil, err
	}
	for _, u := range waiting {
		snapshot.Unassigned = append(snapshot.Unassigned, ports.LobbyUnassigned{
			UserID:      u.ID,
			DisplayName: u.DisplayName,
			Status:      domain.ParticipantWaiting,
		})
	}
	return snapshot, nil
}

func (r *Repositories) GetTeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	const q = `
		WITH cfg AS (
			SELECT market_price, cost_of_publishing, cost_of_discard
			FROM rounds WHERE round_id = $1
		),
		stats AS (
			SELECT points_earned, batches_created, batches_processed, published_jokes, discarded_jokes
			FROM team_rounds_state
			WHERE round_id = $1 AND team_id = $2
		),
		unprocessed AS (
			SELECT COUNT(*) AS cnt
			FROM batches
			WHERE round_id = $1 AND team_id = $2 AND status = 'SUBMITTED'
		),
		profits AS (
			SELECT
				trs.team_id,
				(SELECT market_price FROM cfg) * trs.points_earned::float8
				  - (SELECT cost_of_publishing FROM cfg) * trs.published_jokes::float8
				  - (SELECT cost_of_discard FROM cfg) * trs.discarded_jokes::float8 AS profit
			FROM team_rounds_state trs
			WHERE trs.round_id = $1
		),
		ranks AS (
			SELECT team_id, profit, DENSE_RANK() OVER (ORDER BY profit DESC) AS rnk
			FROM profits
		)
		SELECT t.id, t.name, $1,
		       COALESCE(r.rnk, 1),
		       COALESCE(s.points_earned, 0),
		       COALESCE(r.profit, 0),
		       COALESCE(s.points_earned, 0),
		       COALESCE(s.batches_created, 0),
		       COALESCE(s.batches_processed, 0),
		       COALESCE(s.published_jokes, 0),
		       COALESCE(s.discarded_jokes, 0),
		       GREATEST(COALESCE(s.published_jokes, 0) - COALESCE(s.points_earned, 0), 0),
		       LEAST(COALESCE(s.published_jokes, 0), COALESCE(s.points_earned, 0)),
		       COALESCE(u.cnt, 0)
		FROM teams t
		LEFT JOIN stats s ON true
		LEFT JOIN ranks r ON r.team_id = t.id
		LEFT JOIN unprocessed u ON true
		WHERE t.id = $2
	`
	var summary ports.TeamSummary
	err := r.pg.Pool.QueryRow(ctx, q, roundID, teamID).Scan(
		&summary.Team.ID, &summary.Team.Name, &summary.RoundID,
		&summary.Rank, &summary.Points, &summary.Profit, &summary.TotalSales,
		&summary.BatchesCreated, &summary.BatchesProcessed, &summary.PublishedJokes, &summary.DiscardedJokes,
		&summary.UnsoldJokes, &summary.SoldJokesCount, &summary.UnprocessedBatches,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("team summary")
		}
		return nil, err
	}
	summary.Performance = "AVERAGE PERFORMING"
	return &summary, nil
}

func (r *Repositories) ListTeamFeedbackJokes(ctx context.Context, roundID, teamID int64, limit int) ([]ports.FeedbackJokeRow, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.pg.Pool.Query(ctx, `
		WITH latest AS (
			SELECT j.joke_id, j.joke_title, j.published_at,
			       EXISTS (
			         SELECT 1 FROM purchases p
			         WHERE p.joke_id = j.joke_id AND p.round_id = $1
			       ) AS was_bought
			FROM jokes j
			JOIN batches b ON b.batch_id = j.batch_id
			WHERE b.round_id = $1
			  AND b.team_id = $2
			  AND j.publish_status = 'PUBLISHED'
			ORDER BY j.published_at DESC NULLS LAST, j.joke_id DESC
			LIMIT $3
		)
		SELECT l.joke_id, COALESCE(l.joke_title, ''), l.was_bought, l.published_at,
		       d.dimension, d.dim_fit
		FROM latest l
		LEFT JOIN joke_dim_fit d ON d.joke_id = l.joke_id
		ORDER BY l.published_at DESC NULLS LAST, l.joke_id DESC`,
		roundID, teamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[int64]*ports.FeedbackJokeRow)
	var order []int64
	for rows.Next() {
		var (
			jokeID      int64
			title       string
			wasBought   bool
			publishedAt *time.Time
			dimension   *string
			dimFit      *float64
		)
		if err := rows.Scan(&jokeID, &title, &wasBought, &publishedAt, &dimension, &dimFit); err != nil {
			return nil, err
		}
		row, ok := byID[jokeID]
		if !ok {
			row = &ports.FeedbackJokeRow{
				JokeID:    jokeID,
				JokeTitle: title,
				WasBought: wasBought,
				DimFits:   make(map[domain.Dimension]float64),
			}
			byID[jokeID] = row
			order = append(order, jokeID)
		}
		if dimension != nil && dimFit != nil {
			row.DimFits[domain.Dimension(*dimension)] = *dimFit
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ports.FeedbackJokeRow, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

func (r *Repositories) GetRoundStatsV2(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		WITH cfg AS (
			SELECT market_price, cost_of_publishing, cost_of_discard
			FROM rounds WHERE round_id = $1
		),
		profits AS (
			SELECT
				trs.team_id,
				t.name AS team_name,
				trs.batches_processed,
				trs.points_earned AS total_sales,
				trs.published_jokes,
				trs.discarded_jokes,
				trs.published_jokes + trs.discarded_jokes AS total_jokes,
				GREATEST(trs.published_jokes - trs.points_earned, 0) AS unsold_jokes,
				(SELECT market_price FROM cfg) * trs.points_earned::float8
				  - (SELECT cost_of_publishing FROM cfg) * trs.published_jokes::float8
				  - (SELECT cost_of_discard FROM cfg) * trs.discarded_jokes::float8 AS profit
			FROM team_rounds_state trs
			JOIN teams t ON t.id = trs.team_id
			WHERE trs.round_id = $1
		)
		SELECT
			DENSE_RANK() OVER (ORDER BY profit DESC) AS rnk,
			team_id, team_name, batches_processed, total_sales,
			published_jokes, discarded_jokes, total_jokes, unsold_jokes, profit
		FROM profits
		ORDER BY profit DESC, team_id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &ports.RoundStats{RoundID: roundID, Leaderboard: []ports.TeamStats{}}
	for rows.Next() {
		var ts ports.TeamStats
		if err := rows.Scan(
			&ts.Rank, &ts.Team.ID, &ts.Team.Name, &ts.BatchesProcessed, &ts.TotalSales,
			&ts.PublishedJokes, &ts.DiscardedJokes, &ts.TotalJokes, &ts.UnsoldJokes, &ts.Profit,
		); err != nil {
			return nil, err
		}
		stats.Leaderboard = append(stats.Leaderboard, ts)
	}
	return stats, rows.Err()
}
