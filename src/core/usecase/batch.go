package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// BatchService handles JM batch workflows.
type BatchService struct {
	repo ports.Store
	log  *slog.Logger
}

func NewBatchService(repo ports.Store, log *slog.Logger) *BatchService {
	return &BatchService{repo: repo, log: log}
}

// Submit allows a JM to submit a batch of jokes.
func (s *BatchService) Submit(ctx context.Context, userID, roundID, teamID int64, jokes []string) (*domain.Batch, error) {
	if len(jokes) == 0 {
		return nil, domain.NewValidationError("jokes", "at least one joke required")
	}
	for i, j := range jokes {
		if j == "" {
			return nil, domain.NewValidationError("jokes", fmt.Sprintf("joke %d is empty", i))
		}
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Role == nil || *user.Role != domain.RoleJM {
		return nil, domain.NewForbiddenError("user must be JM")
	}
	if user.TeamID == nil || *user.TeamID != teamID {
		return nil, domain.NewForbiddenError("user not on this team")
	}

	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, domain.NewConflictError("round not active")
	}

	// R1: exact batch_size. R2+: up to batch_size (cap).
	if round.RoundNumber == 1 {
		if len(jokes) != round.BatchSize {
			return nil, domain.NewValidationError("jokes", fmt.Sprintf("expected %d jokes", round.BatchSize))
		}
	} else if len(jokes) > round.BatchSize {
		return nil, domain.NewValidationError("jokes", fmt.Sprintf("expected up to %d jokes", round.BatchSize))
	}

	if err := s.repo.EnsureTeamRoundState(ctx, roundID, teamID); err != nil {
		return nil, err
	}
	return s.repo.CreateBatch(ctx, roundID, teamID, jokes)
}

// List returns batches submitted by a team (JM or Marketing on that team).
func (s *BatchService) List(ctx context.Context, roundID, teamID, userID int64) ([]domain.Batch, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.TeamID == nil || *user.TeamID != teamID {
		return nil, domain.NewForbiddenError("user not on this team")
	}
	if user.Role == nil || (*user.Role != domain.RoleJM && *user.Role != domain.RoleMarketing) {
		return nil, domain.NewForbiddenError("user must be JM or MARKETING")
	}
	return s.repo.ListBatchesByTeam(ctx, roundID, teamID)
}
