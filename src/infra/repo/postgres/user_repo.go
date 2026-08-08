package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

const userColumns = `user_id, display_name, role, team_id, status, assigned_at, joined_at, created_at`

func (r *Repositories) CreateUser(ctx context.Context, displayName string) (*domain.User, error) {
	u, err := scanUser(r.pg.Pool.QueryRow(ctx, `
		INSERT INTO users (display_name)
		VALUES ($1)
		RETURNING `+userColumns, displayName))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.NewConflictError("display name already taken")
		}
		return nil, err
	}
	return u, nil
}

func (r *Repositories) GetUserByDisplayName(ctx context.Context, displayName string) (*domain.User, error) {
	u, err := scanUser(r.pg.Pool.QueryRow(ctx, `
		SELECT `+userColumns+` FROM users WHERE display_name = $1`, displayName))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("user")
		}
		return nil, err
	}
	return u, nil
}

func (r *Repositories) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	u, err := scanUser(r.pg.Pool.QueryRow(ctx, `
		SELECT `+userColumns+` FROM users WHERE user_id = $1`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("user")
		}
		return nil, err
	}
	return u, nil
}

func (r *Repositories) UpdateUserAssignment(ctx context.Context, userID int64, role *domain.Role, teamID *int64) error {
	res, err := r.pg.Pool.Exec(ctx, `
		UPDATE users SET role = $2, team_id = $3 WHERE user_id = $1`, userID, role, teamID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}
	return nil
}

func (r *Repositories) PatchUserInRound(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) error {
	_ = roundID
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
		var exists int64
		if err := tx.QueryRow(ctx, `SELECT user_id FROM users WHERE user_id = $1 FOR UPDATE`, userID).Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NewNotFoundError("user")
			}
			return err
		}
		res, err := tx.Exec(ctx, `
			UPDATE users
			SET role = $2,
			    team_id = $3,
			    status = $4::participant_status,
			    assigned_at = CASE
					WHEN $4::participant_status = 'ASSIGNED' THEN now()
					ELSE NULL
				END
			WHERE user_id = $1`, userID, role, teamID, status)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return domain.NewNotFoundError("user")
		}
		return nil
	})
}

func (r *Repositories) UpdateUserStatus(ctx context.Context, userID int64, status domain.ParticipantStatus) error {
	res, err := r.pg.Pool.Exec(ctx, `
		UPDATE users
		SET status = $2::participant_status,
		    assigned_at = CASE WHEN $2::participant_status = 'ASSIGNED' THEN assigned_at ELSE NULL END
		WHERE user_id = $1`, userID, status)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}
	return nil
}

func (r *Repositories) MarkUserAssigned(ctx context.Context, userID int64) error {
	res, err := r.pg.Pool.Exec(ctx, `
		UPDATE users SET status = 'ASSIGNED', assigned_at = now() WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return domain.NewNotFoundError("user")
	}
	return nil
}

func (r *Repositories) ListUsersByStatus(ctx context.Context, status domain.ParticipantStatus) ([]domain.User, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE status = $1::participant_status AND (role IS NULL OR role <> 'INSTRUCTOR')
		ORDER BY joined_at ASC`, status)
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
	return users, rows.Err()
}

func (r *Repositories) ListTeamMembers(ctx context.Context, teamID int64) ([]ports.TeamMember, error) {
	rows, err := r.pg.Pool.Query(ctx, `
		SELECT user_id, display_name, role
		FROM users
		WHERE team_id = $1 AND role IS NOT NULL AND role <> 'INSTRUCTOR'
		ORDER BY user_id`, teamID)
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
	return members, rows.Err()
}

func (r *Repositories) DeleteUser(ctx context.Context, userID int64) error {
	return r.pg.WithTx(ctx, func(tx pgx.Tx) error {
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
		return nil
	})
}
