package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// SessionService handles join/me flows.
type SessionService struct {
	repo ports.GameRepository
	log  *slog.Logger
}

func NewSessionService(repo ports.GameRepository, log *slog.Logger) *SessionService {
	return &SessionService{repo: repo, log: log}
}

type SessionJoinResult struct {
	User *domain.User
}

// Join registers a user into the latest round as waiting.
func (s *SessionService) Join(ctx context.Context, displayName string) (*SessionJoinResult, error) {
	// Idempotent "login": reuse existing user if the display name already exists.
	var user *domain.User
	user, err := s.repo.GetUserByDisplayName(ctx, displayName)
	if err != nil {
		if domain.IsNotFound(err) {
			user, err = s.repo.CreateUser(ctx, displayName)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else if user.Role != nil && *user.Role == domain.RoleInstructor {
		// Avoid logging into instructor accounts from the student join flow.
		// Create a fresh user with the same display name but no role.
		user, err = s.repo.CreateUser(ctx, displayName)
		if err != nil {
			return nil, err
		}
	}

	// Preserve existing status; only set to WAITING for brand-new users lacking a status.
	if user.Status == "" {
		if err := s.repo.UpdateUserStatus(ctx, user.ID, domain.ParticipantWaiting); err != nil {
			s.log.Error("join: update user status failed", "user_id", user.ID, "error", err)
			return nil, err
		}
		// Refresh status in the returned struct
		user.Status = domain.ParticipantWaiting
	}

	return &SessionJoinResult{
		User: user,
	}, nil
}

type SessionMeResult struct {
	User *domain.User
}

// Me returns session info for a user.
func (s *SessionService) Me(ctx context.Context, userID int64) (*SessionMeResult, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &SessionMeResult{
		User: user,
	}, nil
}
