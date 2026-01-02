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
		RETURNING user_id, display_name, role, team_id, status, assigned_at, joined_at, created_at
	`
	var u domain.User
	err := r.pool.QueryRow(ctx, q, displayName).Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.Status, &u.AssignedAt, &u.JoinedAt, &u.CreatedAt)
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
		SELECT user_id, display_name, role, team_id, status, assigned_at, joined_at, created_at
		FROM users
		WHERE display_name = $1
	`
	var u domain.User
	if err := r.pool.QueryRow(ctx, q, displayName).Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.Status, &u.AssignedAt, &u.JoinedAt, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("user")
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	const q = `
		SELECT user_id, display_name, role, team_id, status, assigned_at, joined_at, created_at
		FROM users
		WHERE user_id = $1
	`
	var u domain.User
	if err := r.pool.QueryRow(ctx, q, userID).Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.Status, &u.AssignedAt, &u.JoinedAt, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("user")
		}
		return nil, err
	}
	return &u, nil
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

func (r *PostgresRepository) UpdateUserStatus(ctx context.Context, userID int64, status domain.ParticipantStatus) error {
	const q = `
		UPDATE users
		SET status = $2::participant_status,
		    assigned_at = CASE WHEN $2::participant_status = 'ASSIGNED' THEN assigned_at ELSE NULL END
		WHERE user_id = $1
	`
	res, err := r.pool.Exec(ctx, q, userID, status)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}
	return nil
}

func (r *PostgresRepository) MarkUserAssigned(ctx context.Context, userID int64) error {
	const q = `
		UPDATE users
		SET status = 'ASSIGNED', assigned_at = now()
		WHERE user_id = $1
	`
	res, err := r.pool.Exec(ctx, q, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}
	return nil
}

func (r *PostgresRepository) ListUsersByStatus(ctx context.Context, status domain.ParticipantStatus) ([]domain.User, error) {
	const q = `
		SELECT user_id, display_name, role, team_id, status, assigned_at, joined_at, created_at
		FROM users
		WHERE status = $1::participant_status AND (role IS NULL OR role <> 'INSTRUCTOR')
		ORDER BY joined_at ASC
	`
	rows, err := r.pool.Query(ctx, q, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Role, &u.TeamID, &u.Status, &u.AssignedAt, &u.JoinedAt, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *PostgresRepository) ListCustomers(ctx context.Context) ([]ports.LobbyCustomer, error) {
	const q = `
		SELECT user_id, display_name, role
		FROM users
		WHERE role = 'CUSTOMER'
		ORDER BY user_id
	`
	rows, err := r.pool.Query(ctx, q)
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

func (r *PostgresRepository) ListTeamMembers(ctx context.Context, teamID int64) ([]ports.TeamMember, error) {
	const q = `
		SELECT user_id, display_name, role
		FROM users
		WHERE team_id = $1 AND role IS NOT NULL AND role <> 'INSTRUCTOR'
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

// DeleteUser removes a non-instructor user from the database.
func (r *PostgresRepository) DeleteUser(ctx context.Context, userID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var role *string
	if err := tx.QueryRow(ctx, `SELECT role FROM users WHERE user_id = $1`, userID).Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NewNotFoundError("user")
		}
		return err
	}
	if role != nil && *role == string(domain.RoleInstructor) {
		return domain.NewConflictError("cannot delete instructor user")
	}

	res, err := tx.Exec(ctx, `DELETE FROM users WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}

	return tx.Commit(ctx)
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
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
		FROM rounds
		WHERE status = 'ACTIVE'
		LIMIT 1
	`
	var rd domain.Round
	err := r.pool.QueryRow(ctx, q).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
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
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
		FROM rounds WHERE round_id = $1
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
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
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
		FROM rounds
		ORDER BY round_id DESC
		LIMIT 1
	`
	var rd domain.Round
	err := r.pool.QueryRow(ctx, q).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) ListRounds(ctx context.Context) ([]domain.Round, error) {
	const q = `
		SELECT round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
		FROM rounds
		ORDER BY round_id ASC
	`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []domain.Round
	for rows.Next() {
		var rd domain.Round
		if err := rows.Scan(
			&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
			&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
		); err != nil {
			return nil, err
		}
		rounds = append(rounds, rd)
	}
	return rounds, nil
}

func (r *PostgresRepository) UpdateRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error) {
	const q = `
		UPDATE rounds
		SET customer_budget = $2, batch_size = $3
		WHERE round_id = $1
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID, customerBudget, batchSize).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
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
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
	`
	if err := r.pool.QueryRow(ctx, q, roundID, roundNumber, customerBudget, batchSize).Scan(
		&inserted.ID, &inserted.RoundNumber, &inserted.Status, &inserted.CustomerBudget, &inserted.BatchSize,
		&inserted.StartedAt, &inserted.EndedAt, &inserted.CreatedAt, &inserted.IsPoppedActive,
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
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID, customerBudget, batchSize).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
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
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("round")
		}
		return nil, err
	}
	return &rd, nil
}

func (r *PostgresRepository) SetRoundPopupState(ctx context.Context, roundID int64, isActive bool) (*domain.Round, error) {
	const q = `
		UPDATE rounds
		SET is_popped_active = $2
		WHERE round_id = $1
		RETURNING round_id, round_number, status, customer_budget, batch_size, started_at, ended_at, created_at, is_popped_active
	`
	var rd domain.Round
	if err := r.pool.QueryRow(ctx, q, roundID, isActive).Scan(
		&rd.ID, &rd.RoundNumber, &rd.Status, &rd.CustomerBudget, &rd.BatchSize,
		&rd.StartedAt, &rd.EndedAt, &rd.CreatedAt, &rd.IsPoppedActive,
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
			(SELECT COUNT(*) FROM users u
				WHERE u.status = 'WAITING' AND (u.role IS NULL OR u.role <> 'INSTRUCTOR')) AS waiting,
			(SELECT COUNT(*) FROM users u
				WHERE u.status = 'ASSIGNED' AND (u.role IS NULL OR u.role <> 'INSTRUCTOR')) AS assigned
	`
	if err := r.pool.QueryRow(ctx, summaryQ).Scan(&snapshot.Summary.Waiting, &snapshot.Summary.Assigned); err != nil {
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

	customers, err := r.ListCustomers(ctx)
	if err != nil {
		return nil, err
	}
	snapshot.Customers = customers
	snapshot.Summary.CustomerCount = len(customers)

	// unassigned (waiting)
	waiting, err := r.ListUsersByStatus(ctx, domain.ParticipantWaiting)
	if err != nil {
		return nil, err
	}
	for _, u := range waiting {
		if u.Role != nil && *u.Role == domain.RoleInstructor {
			// Skip instructors in lobby snapshot
			continue
		}
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
		WITH joke_counts AS (
			SELECT b.team_id, COUNT(j.joke_id) AS total_jokes
			FROM batches b
			LEFT JOIN jokes j ON j.batch_id = b.batch_id
			WHERE b.round_id = $1
			GROUP BY b.team_id
		)
		SELECT rank, team_id, team_name, points_earned, batches_rated, total_sales, accepted_jokes, total_jokes
		FROM (
			SELECT
				DENSE_RANK() OVER (ORDER BY trs.points_earned DESC) as rank,
				t.id AS team_id,
				t.name AS team_name,
				trs.points_earned,
				trs.batches_rated,
				COALESCE(sales.total_sales, 0) AS total_sales,
				trs.accepted_jokes,
				COALESCE(jc.total_jokes, 0) AS total_jokes
			FROM team_rounds_state trs
			JOIN teams t ON t.id = trs.team_id
			LEFT JOIN (
				SELECT pj.team_id, COUNT(*) AS total_sales
				FROM purchases p
				JOIN published_jokes pj ON pj.joke_id = p.joke_id
				WHERE p.round_id = $1
				GROUP BY pj.team_id
			) sales ON sales.team_id = trs.team_id
			LEFT JOIN joke_counts jc ON jc.team_id = trs.team_id
			WHERE trs.round_id = $1
			GROUP BY t.id, t.name, trs.points_earned, trs.batches_rated, sales.total_sales, trs.accepted_jokes, jc.total_jokes
		) ranked
		ORDER BY rank, team_id
	`
	rows, err := r.pool.Query(ctx, q, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ports.TeamStats
	for rows.Next() {
		var s ports.TeamStats
		if err := rows.Scan(&s.Rank, &s.Team.ID, &s.Team.Name, &s.Points, &s.BatchesRated, &s.TotalSales, &s.AcceptedJokes, &s.TotalJokes); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

// GetRoundStatsV2 returns leaderboard plus chart-friendly aggregates.
func (r *PostgresRepository) GetRoundStatsV2(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	leaderboard, err := r.GetRoundStats(ctx, roundID)
	if err != nil {
		r.log.Error("GetRoundStatsV2: leaderboard query failed", "round_id", roundID, "error", err)
		return nil, err
	}

	result := &ports.RoundStats{
		RoundID:     roundID,
		Leaderboard: leaderboard,
	}

	// Sales over time (cumulative points) per team
	const salesQ = `
		WITH purchases_cte AS (
			SELECT p.purchase_id,
			       p.created_at,
			       pj.team_id,
			       t.name AS team_name,
			       ROW_NUMBER() OVER (ORDER BY p.created_at, p.purchase_id) AS event_idx,
			       ROW_NUMBER() OVER (PARTITION BY pj.team_id ORDER BY p.created_at, p.purchase_id) AS team_idx
			FROM purchases p
			JOIN published_jokes pj ON pj.joke_id = p.joke_id AND pj.round_id = p.round_id
			JOIN teams t ON t.id = pj.team_id
			WHERE p.round_id = $1
		)
		SELECT event_idx,
		       team_idx,
		       created_at,
		       team_id,
		       team_name,
		       team_idx AS cumulative_points
		FROM purchases_cte
		ORDER BY event_idx
	`
	rows, err := r.pool.Query(ctx, salesQ, roundID)
	if err != nil {
		r.log.Error("GetRoundStatsV2: sales query failed", "round_id", roundID, "error", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pnt ports.SalesPoint
		if err := rows.Scan(&pnt.EventIndex, &pnt.TeamEventIndex, &pnt.Timestamp, &pnt.TeamID, &pnt.TeamName, &pnt.CumulativePoints); err != nil {
			return nil, err
		}
		result.SalesOverTime = append(result.SalesOverTime, pnt)
	}

	// Batch sequence vs quality (learning over time) for the given round
	const sequenceQ = `
		SELECT b.round_id,
		       r.round_number,
		       b.team_id,
		       t.name,
		       ROW_NUMBER() OVER (PARTITION BY b.team_id ORDER BY b.submitted_at, b.batch_id) AS batch_order,
		       COALESCE(b.avg_score, 0) AS avg_score
		FROM batches b
		JOIN rounds r ON r.round_id = b.round_id
		JOIN teams t ON t.id = b.team_id
		WHERE b.round_id = $1 AND b.status = 'RATED'
		ORDER BY b.submitted_at, b.batch_id
	`
	batchRows, err := r.pool.Query(ctx, sequenceQ, roundID)
	if err != nil {
		r.log.Error("GetRoundStatsV2: sequence query failed", "round_id", roundID, "error", err)
		return nil, err
	}
	defer batchRows.Close()
	for batchRows.Next() {
		var (
			rid        int64
			roundNum   int
			teamID     int64
			teamName   string
			batchOrder int
			avgScore   float64
		)
		if err := batchRows.Scan(&rid, &roundNum, &teamID, &teamName, &batchOrder, &avgScore); err != nil {
			return nil, err
		}
		result.BatchSequenceQuality = append(result.BatchSequenceQuality, ports.BatchSequencePoint{
			RoundID:     rid,
			RoundNumber: roundNum,
			TeamID:      teamID,
			TeamName:    teamName,
			BatchOrder:  batchOrder,
			AvgScore:    avgScore,
		})
	}

	// Batch size vs average quality across rounds (for R1 vs R2 comparison on FE)
	const sizeQ = `
		WITH batch_sizes AS (
			SELECT b.batch_id, COUNT(j.joke_id) AS batch_size
			FROM batches b
			LEFT JOIN jokes j ON j.batch_id = b.batch_id
			WHERE b.status = 'RATED'
			GROUP BY b.batch_id
		)
		SELECT b.round_id,
		       r.round_number,
		       b.team_id,
		       t.name,
		       COALESCE(bs.batch_size, 0) AS batch_size,
		       COALESCE(b.avg_score, 0) AS avg_score
		FROM batches b
		JOIN rounds r ON r.round_id = b.round_id
		JOIN teams t ON t.id = b.team_id
		LEFT JOIN batch_sizes bs ON bs.batch_id = b.batch_id
		WHERE b.status = 'RATED'
		ORDER BY r.round_number, b.batch_id
	`
	sizeRows, err := r.pool.Query(ctx, sizeQ)
	if err != nil {
		r.log.Error("GetRoundStatsV2: batch size query failed", "round_id", roundID, "error", err)
		return nil, err
	}
	defer sizeRows.Close()
	for sizeRows.Next() {
		var pnt ports.BatchSizeQualityPoint
		if err := sizeRows.Scan(&pnt.RoundID, &pnt.RoundNumber, &pnt.TeamID, &pnt.TeamName, &pnt.BatchSize, &pnt.AvgScore); err != nil {
			return nil, err
		}
		result.BatchSizeQuality = append(result.BatchSizeQuality, pnt)
	}

	return result, nil
}

// ResetGame removes all game data from the database. Intended for admin use only.
func (r *PostgresRepository) ResetGame(ctx context.Context) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		r.log.Error("ResetGame: begin tx failed", "error", err)
		return err
	}
	defer tx.Rollback(ctx)

	// Clear gameplay data but keep instructor accounts.
	const truncateQ = `
		TRUNCATE TABLE
			purchases,
			customer_round_budget,
			published_jokes,
			joke_ratings,
			jokes,
			batches,
			team_rounds_state
		RESTART IDENTITY CASCADE
	`
	if _, err := tx.Exec(ctx, truncateQ); err != nil {
		r.log.Error("ResetGame: truncate failed", "error", err)
		return err
	}

	// Reset rounds to defaults instead of deleting them so seeded round ids remain.
	const resetRoundsQ = `
		UPDATE rounds
		SET status = 'CONFIGURED',
		    customer_budget = 0,
		    batch_size = 1,
		    started_at = NULL,
		    ended_at = NULL,
		    is_popped_active = FALSE
	`
	if _, err := tx.Exec(ctx, resetRoundsQ); err != nil {
		r.log.Error("ResetGame: reset rounds failed", "error", err)
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE role IS DISTINCT FROM 'INSTRUCTOR'`); err != nil {
		r.log.Error("ResetGame: delete users failed", "error", err)
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM teams`); err != nil {
		r.log.Error("ResetGame: delete teams failed", "error", err)
		return err
	}
	if _, err := tx.Exec(ctx, `ALTER SEQUENCE teams_id_seq RESTART WITH 1`); err != nil {
		r.log.Error("ResetGame: reset team sequence failed", "error", err)
		return err
	}

	return tx.Commit(ctx)
}
