package repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
	"jokefactory/src/infra/db"
)

// PostgresRepository implements GameRepository using pgx.
type PostgresRepository struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewPostgresRepository constructs a repository backed by Postgres.
func NewPostgresRepository(pg *db.Postgres, log *slog.Logger) *PostgresRepository {
	return &PostgresRepository{
		pool: pg.Pool,
		log:  log,
	}
}

func (r *PostgresRepository) Health(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// Users & participants

func (r *PostgresRepository) CreateUser(ctx context.Context, displayName string) (*domain.User, error) {
	const q = `
		INSERT INTO users (display_name)
		VALUES ($1)
		RETURNING user_id, display_name, role, team_id, created_at
	`
	var u domain.User
	err := r.pool.QueryRow(ctx, q, displayName).Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.NewConflictError("display name already taken")
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepository) GetUserByDisplayName(ctx context.Context, displayName string) (*domain.User, error) {
	const q = `
		SELECT user_id, display_name, role, team_id, created_at
		FROM users
		WHERE display_name = $1
	`
	var u domain.User
	if err := r.pool.QueryRow(ctx, q, displayName).Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("user")
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	const q = `
		SELECT user_id, display_name, role, team_id, created_at
		FROM users
		WHERE user_id = $1
	`
	var u domain.User
	if err := r.pool.QueryRow(ctx, q, userID).Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("user")
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepository) EnsureParticipant(ctx context.Context, roundID, userID int64) (*domain.RoundParticipant, error) {
	const q = `
		INSERT INTO round_participants (round_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (round_id, user_id) DO NOTHING
	`
	if _, err := r.pool.Exec(ctx, q, roundID, userID); err != nil {
		return nil, err
	}
	return r.GetParticipant(ctx, roundID, userID)
}

func (r *PostgresRepository) GetParticipant(ctx context.Context, roundID, userID int64) (*domain.RoundParticipant, error) {
	const q = `
		SELECT round_id, user_id, status, joined_at, assigned_at
		FROM round_participants
		WHERE round_id = $1 AND user_id = $2
	`
	var p domain.RoundParticipant
	if err := r.pool.QueryRow(ctx, q, roundID, userID).Scan(
		&p.RoundID, &p.UserID, &p.Status, &p.JoinedAt, &p.AssignedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("participant")
		}
		return nil, err
	}
	return &p, nil
}

func (r *PostgresRepository) UpdateUserAssignment(ctx context.Context, userID int64, role *domain.Role, teamID *int64) error {
	const q = `
		UPDATE users
		SET role = $2, team_id = $3
		WHERE user_id = $1
	`
	res, err := r.pool.Exec(ctx, q, userID, role, teamID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}
	return nil
}

func (r *PostgresRepository) UpdateParticipantStatus(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus) error {
	const q = `
		UPDATE round_participants
		SET status = $3
		WHERE round_id = $1 AND user_id = $2
	`
	res, err := r.pool.Exec(ctx, q, roundID, userID, status)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("participant")
	}
	return nil
}

func (r *PostgresRepository) MarkParticipantAssigned(ctx context.Context, roundID, userID int64) error {
	const q = `
		UPDATE round_participants
		SET status = 'ASSIGNED', assigned_at = now()
		WHERE round_id = $1 AND user_id = $2
	`
	res, err := r.pool.Exec(ctx, q, roundID, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("participant")
	}
	return nil
}

func (r *PostgresRepository) ListParticipantsByStatus(ctx context.Context, roundID int64, status domain.ParticipantStatus) ([]domain.User, error) {
	const q = `
		SELECT u.user_id, u.display_name, u.role, u.team_id, u.created_at
		FROM round_participants rp
		JOIN users u ON u.user_id = rp.user_id
		WHERE rp.round_id = $1 AND rp.status = $2
		ORDER BY rp.joined_at ASC
	`
	rows, err := r.pool.Query(ctx, q, roundID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *PostgresRepository) ListTeamMembers(ctx context.Context, teamID int64) ([]ports.TeamMember, error) {
	const q = `
		SELECT user_id, display_name, role
		FROM users
		WHERE team_id = $1 AND role IS NOT NULL
		ORDER BY user_id
	`
	rows, err := r.pool.Query(ctx, q, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ports.TeamMember
	for rows.Next() {
		var m ports.TeamMember
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Role); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

func (r *PostgresRepository) ListCustomers(ctx context.Context, roundID int64) ([]ports.LobbyCustomer, error) {
	const q = `
		SELECT u.user_id, u.display_name, u.role
		FROM round_participants rp
		JOIN users u ON u.user_id = rp.user_id
		WHERE rp.round_id = $1 AND u.role = 'CUSTOMER'
		ORDER BY u.user_id
	`
	rows, err := r.pool.Query(ctx, q, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var customers []ports.LobbyCustomer
	for rows.Next() {
		var c ports.LobbyCustomer
		if err := rows.Scan(&c.UserID, &c.DisplayName, &c.Role); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, nil
}

// Teams

func (r *PostgresRepository) EnsureTeamCount(ctx context.Context, teamCount int) ([]domain.Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	const countQ = `SELECT COUNT(*) FROM teams`
	var current int
	if err := tx.QueryRow(ctx, countQ).Scan(&current); err != nil {
		return nil, err
	}
	for i := current; i < teamCount; i++ {
		name := fmt.Sprintf("Team %d", i+1)
		if _, err := tx.Exec(ctx, `INSERT INTO teams (name) VALUES ($1)`, name); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT id, name, created_at FROM teams ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var teams []domain.Team
	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return teams, nil
}

func (r *PostgresRepository) GetTeam(ctx context.Context, teamID int64) (*domain.Team, error) {
	const q = `SELECT id, name, created_at FROM teams WHERE id = $1`
	var t domain.Team
	if err := r.pool.QueryRow(ctx, q, teamID).Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("team")
		}
		return nil, err
	}
	return &t, nil
}

// Rounds

func (r *PostgresRepository) GetActiveRound(ctx context.Context) (*domain.Round, error) {
	const q = `
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
		FROM rounds
		WHERE status = 'ACTIVE'
		LIMIT 1
	`
	var rd domain.Round
	err := r.pool.QueryRow(ctx, q).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) GetRoundByID(ctx context.Context, roundID int64) (*domain.Round, error) {
	const q = `
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
		FROM rounds WHERE round_id = $1
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) GetLatestRound(ctx context.Context) (*domain.Round, error) {
	const q = `
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
		FROM rounds
		ORDER BY round_id DESC
		LIMIT 1
	`
	var rd domain.Round
	err := r.pool.QueryRow(ctx, q).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) UpdateRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error) {
	const q = `
		UPDATE rounds
		SET customer_budget = $2, batch_size = $3
		WHERE round_id = $1
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID, customerBudget, batchSize).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		r.log.Error("UpdateRoundConfig failed", "round_id", roundID, "err", err)
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) InsertRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error) {
	rd, err := r.UpdateRoundConfig(ctx, roundID, customerBudget, batchSize)
	if err == nil {
		return rd, nil
	}
	if !domain.IsNotFound(err) {
		r.log.Error("InsertRoundConfig update branch failed", "round_id", roundID, "err", err)
		return nil, err
	}

	var inserted domain.Round
	roundNumber := int(roundID)
	const q = `
		INSERT INTO rounds (round_id, round_number, status, customer_budget, batch_size)
		VALUES ($1, $2, 'CONFIGURED', $3, $4)
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
	`
	if err := r.pool.QueryRow(ctx, q, roundID, roundNumber, customerBudget, batchSize).Scan(
		&inserted.ID, &inserted.RoundNumber, &inserted.Status, &inserted.CustomerBudget, &inserted.BatchSize,
		&inserted.StartedAt, &inserted.EndedAt, &inserted.CreatedAt,
	); err != nil {
		r.log.Error("InsertRoundConfig insert failed", "round_id", roundID, "err", err)
		return nil, err
	}
	r.log.Info("InsertRoundConfig inserted round", "round_id", inserted.ID, "round_number", inserted.RoundNumber)
	return &inserted, nil
}

func (r *PostgresRepository) StartRound(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error) {
	const q = `
		UPDATE rounds
		SET status = 'ACTIVE',
		    customer_budget = $2,
		    batch_size = $3,
		    started_at = COALESCE(started_at, now()),
		    ended_at = NULL
		WHERE round_id = $1
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID, customerBudget, batchSize).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) EndRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	const q = `
		UPDATE rounds
		SET status = 'ENDED', ended_at = now()
		WHERE round_id = $1
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return &rd, nil
}

// Team round state

func (r *PostgresRepository) EnsureTeamRoundState(ctx context.Context, roundID, teamID int64) error {
	const q = `
		INSERT INTO team_rounds_state (round_id, team_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := r.pool.Exec(ctx, q, roundID, teamID)
	return err
}

func (r *PostgresRepository) IncrementBatchCreated(ctx context.Context, roundID, teamID int64) error {
	const q = `
		UPDATE team_rounds_state
		SET batches_created = batches_created + 1, updated_at = now()
		WHERE round_id = $1 AND team_id = $2
	`
	_, err := r.pool.Exec(ctx, q, roundID, teamID)
	return err
}

func (r *PostgresRepository) IncrementRatedStats(ctx context.Context, roundID, teamID int64, passesCount, pointsDelta int) error {
	const q = `
		UPDATE team_rounds_state
		SET batches_rated = batches_rated + 1,
			accepted_jokes = accepted_jokes + $3,
			points_earned = points_earned + $4,
			updated_at = now()
		WHERE round_id = $1 AND team_id = $2
	`
	_, err := r.pool.Exec(ctx, q, roundID, teamID, passesCount, pointsDelta)
	return err
}

// Batches and jokes

func (r *PostgresRepository) CreateBatch(ctx context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var batch domain.Batch
	const insertBatch = `
		INSERT INTO batches (round_id, team_id, status, submitted_at)
		VALUES ($1, $2, 'SUBMITTED', now())
		RETURNING batch_id, round_id, team_id, status, submitted_at, rated_at, avg_score, passes_count, locked_at, created_at
	`
	if err := tx.QueryRow(ctx, insertBatch, roundID, teamID).Scan(
		&batch.ID, &batch.RoundID, &batch.TeamID, &batch.Status, &batch.SubmittedAt,
		&batch.RatedAt, &batch.AvgScore, &batch.PassesCount, &batch.LockedAt, &batch.CreatedAt,
	); err != nil {
		return nil, err
	}

	const insertJoke = `
		INSERT INTO jokes (batch_id, joke_text)
		VALUES ($1, $2)
	`
	for _, text := range jokes {
		if _, err := tx.Exec(ctx, insertJoke, batch.ID, text); err != nil {
			return nil, err
		}
	}

	if err := r.IncrementBatchCreated(ctx, roundID, teamID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *PostgresRepository) ListBatchesByTeam(ctx context.Context, roundID, teamID int64) ([]domain.Batch, error) {
	const q = `
		SELECT batch_id, round_id, team_id, status, submitted_at, rated_at, avg_score, passes_count, feedback, locked_at, created_at
		FROM batches
		WHERE round_id = $1 AND team_id = $2
		ORDER BY submitted_at DESC, batch_id DESC
	`
	rows, err := r.pool.Query(ctx, q, roundID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []domain.Batch
	var batchIDs []int64
	for rows.Next() {
		var b domain.Batch
		if err := rows.Scan(&b.ID, &b.RoundID, &b.TeamID, &b.Status, &b.SubmittedAt, &b.RatedAt, &b.AvgScore, &b.PassesCount, &b.Feedback, &b.LockedAt, &b.CreatedAt); err != nil {
			return nil, err
		}
		batches = append(batches, b)
		batchIDs = append(batchIDs, b.ID)
	}

	// Aggregate tag counts per batch to serve JM view.
	if len(batchIDs) > 0 {
		const tagsQ = `
			SELECT j.batch_id, jr.tag, COUNT(*) AS cnt
			FROM joke_ratings jr
			JOIN jokes j ON j.joke_id = jr.joke_id
			WHERE j.batch_id = ANY($1)
			GROUP BY j.batch_id, jr.tag
		`
		rowsTags, err := r.pool.Query(ctx, tagsQ, batchIDs)
		if err != nil {
			return nil, err
		}
		defer rowsTags.Close()

		tagMap := make(map[int64][]domain.TagCount)
		for rowsTags.Next() {
			var batchID int64
			var tag domain.QCTag
			var cnt int
			if err := rowsTags.Scan(&batchID, &tag, &cnt); err != nil {
				return nil, err
			}
			tagMap[batchID] = append(tagMap[batchID], domain.TagCount{Tag: tag, Count: cnt})
		}

		for i := range batches {
			if ts, ok := tagMap[batches[i].ID]; ok {
				batches[i].TagSummary = ts
			}
		}

		// Fetch jokes for all batches in one query.
		const jokesQ = `
			SELECT joke_id, batch_id, joke_text, created_at
			FROM jokes
			WHERE batch_id = ANY($1)
			ORDER BY batch_id, joke_id
		`
		rowsJokes, err := r.pool.Query(ctx, jokesQ, batchIDs)
		if err != nil {
			return nil, err
		}
		defer rowsJokes.Close()

		jokeMap := make(map[int64][]domain.Joke)
		for rowsJokes.Next() {
			var j domain.Joke
			if err := rowsJokes.Scan(&j.ID, &j.BatchID, &j.Text, &j.CreatedAt); err != nil {
				return nil, err
			}
			jokeMap[j.BatchID] = append(jokeMap[j.BatchID], j)
		}

		for i := range batches {
			if js, ok := jokeMap[batches[i].ID]; ok {
				batches[i].Jokes = js
			}
		}
	}

	return batches, nil
}

func (r *PostgresRepository) GetBatchWithJokes(ctx context.Context, batchID int64) (*ports.BatchWithJokes, error) {
	const batchQ = `
		SELECT batch_id, round_id, team_id, status, submitted_at, rated_at, avg_score, passes_count, locked_at, created_at, locked_by_qc
		FROM batches
		WHERE batch_id = $1
	`
	var b domain.Batch
	var lockedBy *int64
	if err := r.pool.QueryRow(ctx, batchQ, batchID).Scan(
		&b.ID, &b.RoundID, &b.TeamID, &b.Status, &b.SubmittedAt, &b.RatedAt, &b.AvgScore, &b.PassesCount, &b.LockedAt, &b.CreatedAt, &lockedBy,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("batch")
		}
		return nil, err
	}
	const jokesQ = `SELECT joke_id, batch_id, joke_text, created_at FROM jokes WHERE batch_id = $1 ORDER BY joke_id`
	rows, err := r.pool.Query(ctx, jokesQ, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jokes []domain.Joke
	for rows.Next() {
		var j domain.Joke
		if err := rows.Scan(&j.ID, &j.BatchID, &j.Text, &j.CreatedAt); err != nil {
			return nil, err
		}
		jokes = append(jokes, j)
	}
	return &ports.BatchWithJokes{Batch: b, Jokes: jokes}, nil
}

func (r *PostgresRepository) GetNextBatchForQC(ctx context.Context, roundID, qcUserID int64) (*ports.BatchWithJokes, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	const nextQ = `
		SELECT batch_id
		FROM batches
		WHERE round_id = $1 AND status = 'SUBMITTED' AND (locked_by_qc IS NULL OR locked_by_qc = $2)
		ORDER BY submitted_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`
	var batchID int64
	if err := tx.QueryRow(ctx, nextQ, roundID, qcUserID).Scan(&batchID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, domain.NewNotFoundError("batch")
		}
		return nil, 0, err
	}

	if _, err := tx.Exec(ctx, `UPDATE batches SET locked_at = now(), locked_by_qc = $2 WHERE batch_id = $1`, batchID, qcUserID); err != nil {
		return nil, 0, err
	}

	const jokesQ = `SELECT joke_id, batch_id, joke_text, created_at FROM jokes WHERE batch_id = $1 ORDER BY joke_id`
	rows, err := tx.Query(ctx, jokesQ, batchID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var jokes []domain.Joke
	for rows.Next() {
		var j domain.Joke
		if err := rows.Scan(&j.ID, &j.BatchID, &j.Text, &j.CreatedAt); err != nil {
			return nil, 0, err
		}
		jokes = append(jokes, j)
	}

	const batchQ = `
		SELECT batch_id, round_id, team_id, status, submitted_at, rated_at, avg_score, passes_count, locked_at, created_at
		FROM batches WHERE batch_id = $1
	`
	var b domain.Batch
	if err := tx.QueryRow(ctx, batchQ, batchID).Scan(
		&b.ID, &b.RoundID, &b.TeamID, &b.Status, &b.SubmittedAt, &b.RatedAt, &b.AvgScore, &b.PassesCount, &b.LockedAt, &b.CreatedAt,
	); err != nil {
		return nil, 0, err
	}

	var queueSize int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM batches WHERE round_id = $1 AND status = 'SUBMITTED'`, roundID).Scan(&queueSize); err != nil {
		return nil, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return &ports.BatchWithJokes{Batch: b, Jokes: jokes}, queueSize, nil
}

func (r *PostgresRepository) RateBatch(ctx context.Context, batchID int64, qcUserID int64, ratings []domain.JokeRating, feedback *string) (*domain.Batch, []int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	const selectQ = `
		SELECT batch_id, round_id, team_id, status, locked_by_qc
		FROM batches
		WHERE batch_id = $1
		FOR UPDATE
	`
	var batch domain.Batch
	var lockedBy *int64
	if err := tx.QueryRow(ctx, selectQ, batchID).Scan(&batch.ID, &batch.RoundID, &batch.TeamID, &batch.Status, &lockedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, domain.NewNotFoundError("batch")
		}
		return nil, nil, err
	}
	if batch.Status == domain.BatchRated {
		return nil, nil, domain.NewConflictError("batch already rated")
	}
	if lockedBy != nil && *lockedBy != qcUserID {
		return nil, nil, domain.NewConflictError("not assigned to this qc")
	}

	// Insert ratings
	const insertRating = `
		INSERT INTO joke_ratings (joke_id, qc_user_id, rating, tag)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (joke_id)
		DO UPDATE SET rating = EXCLUDED.rating, tag = EXCLUDED.tag, qc_user_id = EXCLUDED.qc_user_id, rated_at = now()
	`
	var passes int
	var total int
	for _, rgt := range ratings {
		if _, err := tx.Exec(ctx, insertRating, rgt.JokeID, qcUserID, rgt.Rating, rgt.Tag); err != nil {
			return nil, nil, err
		}
		total += rgt.Rating
		if rgt.Rating == 5 {
			passes++
		}
	}

	avg := float64(total) / float64(len(ratings))
	now := time.Now()
	const updateBatch = `
		UPDATE batches
		SET status = 'RATED',
			rated_at = $2,
			avg_score = $3,
			passes_count = $4,
			feedback = $5,
			locked_at = NULL,
			locked_by_qc = NULL
		WHERE batch_id = $1
		RETURNING batch_id, round_id, team_id, status, submitted_at, rated_at, avg_score, passes_count, feedback, locked_at, created_at
	`
	var updated domain.Batch
	if err := tx.QueryRow(ctx, updateBatch, batchID, now, avg, passes, feedback).Scan(
		&updated.ID, &updated.RoundID, &updated.TeamID, &updated.Status, &updated.SubmittedAt, &updated.RatedAt, &updated.AvgScore, &updated.PassesCount, &updated.Feedback, &updated.LockedAt, &updated.CreatedAt,
	); err != nil {
		return nil, nil, err
	}

	// Publish jokes with rating 5
	const publishQ = `
		INSERT INTO published_jokes (joke_id, round_id, team_id)
		SELECT joke_id, $2, $3 FROM jokes WHERE batch_id = $1 AND joke_id IN (
			SELECT joke_id FROM joke_ratings WHERE joke_id = jokes.joke_id AND rating = 5
		)
		ON CONFLICT (joke_id) DO NOTHING
		RETURNING joke_id
	`
	pubRows, err := tx.Query(ctx, publishQ, batchID, updated.RoundID, updated.TeamID)
	if err != nil {
		return nil, nil, err
	}
	defer pubRows.Close()
	var published []int64
	for pubRows.Next() {
		var id int64
		if err := pubRows.Scan(&id); err != nil {
			return nil, nil, err
		}
		published = append(published, id)
	}

	if err := r.IncrementRatedStats(ctx, updated.RoundID, updated.TeamID, passes, 0); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return &updated, published, nil
}

func (r *PostgresRepository) CountSubmittedBatches(ctx context.Context, roundID int64) (int, error) {
	const q = `SELECT COUNT(*) FROM batches WHERE round_id = $1 AND status = 'SUBMITTED'`
	var count int
	if err := r.pool.QueryRow(ctx, q, roundID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Market and budget

func (r *PostgresRepository) EnsureCustomerBudget(ctx context.Context, roundID, customerID int64, starting int) (*domain.CustomerRoundBudget, error) {
	const q = `
		INSERT INTO customer_round_budget (round_id, customer_user_id, starting_budget, remaining_budget)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (round_id, customer_user_id)
		DO NOTHING
	`
	if _, err := r.pool.Exec(ctx, q, roundID, customerID, starting); err != nil {
		return nil, err
	}
	return r.getCustomerBudget(ctx, roundID, customerID)
}

func (r *PostgresRepository) getCustomerBudget(ctx context.Context, roundID, customerID int64) (*domain.CustomerRoundBudget, error) {
	const q = `
		SELECT round_id, customer_user_id, starting_budget, remaining_budget, created_at, updated_at
		FROM customer_round_budget
		WHERE round_id = $1 AND customer_user_id = $2
	`
	var b domain.CustomerRoundBudget
	if err := r.pool.QueryRow(ctx, q, roundID, customerID).Scan(
		&b.RoundID, &b.CustomerUserID, &b.StartingBudget, &b.RemainingBudget, &b.CreatedAt, &b.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("budget")
		}
		return nil, err
	}
	return &b, nil
}

func (r *PostgresRepository) ListMarket(ctx context.Context, roundID, customerID int64) ([]ports.MarketItem, error) {
	const q = `
		SELECT pj.joke_id, j.joke_text, pj.team_id, t.name,
			CASE WHEN p.purchase_id IS NOT NULL THEN TRUE ELSE FALSE END AS is_bought
		FROM published_jokes pj
		JOIN jokes j ON j.joke_id = pj.joke_id
		JOIN teams t ON t.id = pj.team_id
		LEFT JOIN purchases p ON p.round_id = pj.round_id AND p.joke_id = pj.joke_id AND p.customer_user_id = $2
		WHERE pj.round_id = $1
		ORDER BY pj.joke_id
	`
	rows, err := r.pool.Query(ctx, q, roundID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ports.MarketItem
	for rows.Next() {
		var item ports.MarketItem
		if err := rows.Scan(&item.JokeID, &item.JokeText, &item.TeamID, &item.TeamName, &item.IsBoughtByMe); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgresRepository) BuyJoke(ctx context.Context, roundID, customerID, jokeID int64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	defer tx.Rollback(ctx)

	budget, err := r.getCustomerBudgetTx(ctx, tx, roundID, customerID)
	if err != nil {
		return nil, nil, 0, err
	}
	if budget.RemainingBudget <= 0 {
		return nil, nil, 0, domain.NewConflictError("insufficient budget")
	}

	const purchaseQ = `
		INSERT INTO purchases (round_id, customer_user_id, joke_id)
		VALUES ($1, $2, $3)
		RETURNING purchase_id, created_at
	`
	var p domain.Purchase
	if err := tx.QueryRow(ctx, purchaseQ, roundID, customerID, jokeID).Scan(&p.ID, &p.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, nil, 0, domain.NewConflictError("already bought")
		}
		return nil, nil, 0, err
	}
	p.RoundID = roundID
	p.CustomerUserID = customerID
	p.JokeID = jokeID

	if _, err := tx.Exec(ctx, `UPDATE customer_round_budget SET remaining_budget = remaining_budget - 1, updated_at = now() WHERE round_id = $1 AND customer_user_id = $2`, roundID, customerID); err != nil {
		return nil, nil, 0, err
	}

	var teamID int64
	if err := tx.QueryRow(ctx, `SELECT team_id FROM published_jokes WHERE joke_id = $1`, jokeID).Scan(&teamID); err != nil {
		return nil, nil, 0, err
	}

	if err := r.IncrementRatedStats(ctx, roundID, teamID, 0, 1); err != nil {
		return nil, nil, 0, err
	}

	budget.RemainingBudget--
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, 0, err
	}
	return &p, budget, teamID, nil
}

func (r *PostgresRepository) ReturnJoke(ctx context.Context, roundID, customerID, jokeID int64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	defer tx.Rollback(ctx)

	const findQ = `
		SELECT purchase_id, created_at
		FROM purchases
		WHERE round_id = $1 AND customer_user_id = $2 AND joke_id = $3
		FOR UPDATE
	`
	var p domain.Purchase
	if err := tx.QueryRow(ctx, findQ, roundID, customerID, jokeID).Scan(&p.ID, &p.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, 0, domain.NewConflictError("not bought yet")
		}
		return nil, nil, 0, err
	}
	p.RoundID = roundID
	p.CustomerUserID = customerID
	p.JokeID = jokeID

	if _, err := tx.Exec(ctx, `DELETE FROM purchases WHERE purchase_id = $1`, p.ID); err != nil {
		return nil, nil, 0, err
	}

	if _, err := tx.Exec(ctx, `UPDATE customer_round_budget SET remaining_budget = remaining_budget + 1, updated_at = now() WHERE round_id = $1 AND customer_user_id = $2`, roundID, customerID); err != nil {
		return nil, nil, 0, err
	}

	var teamID int64
	if err := tx.QueryRow(ctx, `SELECT team_id FROM published_jokes WHERE joke_id = $1`, jokeID).Scan(&teamID); err != nil {
		return nil, nil, 0, err
	}

	if err := r.IncrementRatedStats(ctx, roundID, teamID, 0, -1); err != nil {
		return nil, nil, 0, err
	}

	budget, err := r.getCustomerBudgetTx(ctx, tx, roundID, customerID)
	if err != nil {
		return nil, nil, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, 0, err
	}
	return &p, budget, teamID, nil
}

func (r *PostgresRepository) getCustomerBudgetTx(ctx context.Context, tx pgx.Tx, roundID, customerID int64) (*domain.CustomerRoundBudget, error) {
	const q = `
		SELECT round_id, customer_user_id, starting_budget, remaining_budget, created_at, updated_at
		FROM customer_round_budget
		WHERE round_id = $1 AND customer_user_id = $2
	`
	var b domain.CustomerRoundBudget
	if err := tx.QueryRow(ctx, q, roundID, customerID).Scan(
		&b.RoundID, &b.CustomerUserID, &b.StartingBudget, &b.RemainingBudget, &b.CreatedAt, &b.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("budget")
		}
		return nil, err
	}
	return &b, nil
}

// Stats and lobby

func (r *PostgresRepository) GetTeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	const q = `
		WITH stats AS (
			SELECT trs.points_earned,
			       trs.batches_created,
			       trs.batches_rated,
			       trs.accepted_jokes,
			       COALESCE(SUM(b.passes_count)::INT, 0) AS passes,
			       COALESCE(AVG(b.avg_score), 0) AS avg_score
			FROM team_rounds_state trs
			LEFT JOIN batches b ON b.round_id = trs.round_id AND b.team_id = trs.team_id AND b.status = 'RATED'
			WHERE trs.round_id = $1 AND trs.team_id = $2
			GROUP BY trs.points_earned, trs.batches_created, trs.batches_rated, trs.accepted_jokes
		),
		ranks AS (
			SELECT team_id, DENSE_RANK() OVER (ORDER BY points_earned DESC) AS rnk
			FROM team_rounds_state
			WHERE round_id = $1
		),
		unrated AS (
			SELECT COUNT(*) AS cnt
			FROM batches
			WHERE round_id = $1 AND team_id = $2 AND status = 'SUBMITTED'
		),
		sales AS (
			SELECT COUNT(*) AS total_sales
			FROM purchases
			WHERE round_id = $1 AND joke_id IN (SELECT joke_id FROM published_jokes WHERE team_id = $2 AND round_id = $1)
		)
		SELECT t.id, t.name, $1 as round_id, r.rnk, s.points_earned, sa.total_sales,
		       s.batches_created, s.batches_rated, s.accepted_jokes,
		       COALESCE(s.avg_score, 0), u.cnt
		FROM teams t
		JOIN stats s ON true
		JOIN ranks r ON r.team_id = t.id
		JOIN unrated u ON true
		JOIN sales sa ON true
		WHERE t.id = $2
	`
	var summary ports.TeamSummary
	if err := r.pool.QueryRow(ctx, q, roundID, teamID).Scan(
		&summary.Team.ID, &summary.Team.Name, &summary.RoundID, &summary.Rank, &summary.Points, &summary.TotalSales,
		&summary.BatchesCreated, &summary.BatchesRated, &summary.AcceptedJokes,
		&summary.AvgScoreOverall, &summary.UnratedBatches,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("team summary")
		}
		return nil, err
	}
	return &summary, nil
}

func (r *PostgresRepository) GetLobby(ctx context.Context, roundID int64) (*ports.LobbySnapshot, error) {
	var snapshot ports.LobbySnapshot
	snapshot.RoundID = roundID

	const summaryQ = `
		SELECT
			(SELECT COUNT(*) FROM round_participants WHERE round_id = $1 AND status = 'WAITING') AS waiting,
			(SELECT COUNT(*) FROM round_participants WHERE round_id = $1 AND status = 'ASSIGNED') AS assigned
	`
	if err := r.pool.QueryRow(ctx, summaryQ, roundID).Scan(&snapshot.Summary.Waiting, &snapshot.Summary.Assigned); err != nil {
		return nil, err
	}
	snapshot.Summary.Dropped = 0

	// teams with members
	teamRows, err := r.pool.Query(ctx, `SELECT id, name, created_at FROM teams ORDER BY id`)
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
	snapshot.Summary.TeamCount = len(snapshot.Teams)

	customers, err := r.ListCustomers(ctx, roundID)
	if err != nil {
		return nil, err
	}
	snapshot.Customers = customers
	snapshot.Summary.CustomerCount = len(customers)

	// unassigned (waiting)
	waiting, err := r.ListParticipantsByStatus(ctx, roundID, domain.ParticipantWaiting)
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

	return &snapshot, nil
}

func (r *PostgresRepository) GetRoundStats(ctx context.Context, roundID int64) ([]ports.TeamStats, error) {
	const q = `
		SELECT
			DENSE_RANK() OVER (ORDER BY trs.points_earned DESC) as rank,
			t.id, t.name,
			trs.points_earned,
			COALESCE(sales.total_sales, 0),
			trs.batches_rated,
			COALESCE(avg(b.avg_score), 0),
			trs.accepted_jokes
		FROM team_rounds_state trs
		JOIN teams t ON t.id = trs.team_id
		LEFT JOIN batches b ON b.round_id = trs.round_id AND b.team_id = trs.team_id AND b.status = 'RATED'
		LEFT JOIN (
			SELECT pj.team_id, COUNT(*) AS total_sales
			FROM purchases p
			JOIN published_jokes pj ON pj.joke_id = p.joke_id
			WHERE p.round_id = $1
			GROUP BY pj.team_id
		) sales ON sales.team_id = trs.team_id
		WHERE trs.round_id = $1
		GROUP BY rank, t.id, t.name, trs.points_earned, sales.total_sales, trs.batches_rated, trs.accepted_jokes
		ORDER BY rank, t.id
	`
	rows, err := r.pool.Query(ctx, q, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ports.TeamStats
	for rows.Next() {
		var s ports.TeamStats
		if err := rows.Scan(&s.Rank, &s.Team.ID, &s.Team.Name, &s.Points, &s.TotalSales, &s.BatchesRated, &s.AvgScoreOverall, &s.AcceptedJokes); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

