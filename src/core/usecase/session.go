package usecase

import (
	"context"
	"fmt"
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

const (
	placeholderCustomerBudget = 0
	placeholderBatchSize      = 1
)

type SessionJoinResult struct {
	User        *domain.User
	Round       *domain.Round
	Participant *domain.RoundParticipant
}

// Join registers a user into the latest round as waiting.
func (s *SessionService) Join(ctx context.Context, displayName string) (*SessionJoinResult, error) {
	round, err := s.repo.GetLatestRound(ctx)
	if err != nil {
		return nil, err
	}
	if round == nil {
		// Auto-create a new CONFIGURED round if none exist yet.
		roundID := int64(1)
		round, err = s.repo.InsertRoundConfig(ctx, roundID, placeholderCustomerBudget, placeholderBatchSize)
		if err != nil {
			return nil, err
		}
	}

	// Idempotent "login": reuse existing user if the display name already exists.
	var user *domain.User
	user, err = s.repo.GetUserByDisplayName(ctx, displayName)
	if err != nil {
		if domain.IsNotFound(err) {
			user, err = s.repo.CreateUser(ctx, displayName)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	participant, err := s.repo.EnsureParticipant(ctx, round.ID, user.ID)
	if err != nil {
		return nil, err
	}

	return &SessionJoinResult{
		User:        user,
		Round:       round,
		Participant: participant,
	}, nil
}

type SessionMeResult struct {
	User        *domain.User
	Round       *domain.Round
	Participant *domain.RoundParticipant
}

// Me returns session info for a user.
func (s *SessionService) Me(ctx context.Context, userID int64) (*SessionMeResult, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	round, err := s.repo.GetLatestRound(ctx)
	if err != nil {
		return nil, err
	}
	if round == nil {
		return nil, domain.NewNotFoundError("round")
	}

	var participant *domain.RoundParticipant
	// Instructors are not in round_participants; allow them through.
	if user.Role != nil && *user.Role == domain.RoleInstructor {
		participant = nil
	} else {
		participant, err = s.repo.GetParticipant(ctx, round.ID, userID)
		if err != nil {
			return nil, domain.NewNotFoundError(fmt.Sprintf("participant in round %d", round.ID))
		}
	}

	return &SessionMeResult{
		User:        user,
		Round:       round,
		Participant: participant,
	}, nil
}

