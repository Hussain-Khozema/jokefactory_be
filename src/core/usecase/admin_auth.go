package usecase

import (
	"context"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// AdminAuthService handles instructor login via admin password.
type AdminAuthService struct {
	repo          ports.GameRepository
	adminPassword string
}

const (
	defaultInstructorCustomerBudget = 0
	defaultInstructorBatchSize      = 1
)

func NewAdminAuthService(repo ports.GameRepository, adminPassword string) *AdminAuthService {
	return &AdminAuthService{repo: repo, adminPassword: adminPassword}
}

type AdminLoginResult struct {
	User  *domain.User
	Round *domain.Round
}

func (s *AdminAuthService) Login(ctx context.Context, displayName, password string) (*AdminLoginResult, error) {
	if s.adminPassword == "" {
		return nil, domain.NewUnauthorizedError("admin password not configured")
	}
	if password != s.adminPassword {
		return nil, domain.NewUnauthorizedError("invalid admin password")
	}

	// Get or create user
	user, err := s.repo.GetUserByDisplayName(ctx, displayName)
	if err != nil {
		if !domain.IsNotFound(err) {
			return nil, err
		}
		user, err = s.repo.CreateUser(ctx, displayName)
		if err != nil {
			return nil, err
		}
	}

	role := domain.RoleInstructor
	if err := s.repo.UpdateUserAssignment(ctx, user.ID, &role, nil); err != nil {
		return nil, err
	}
	// refresh user
	user, _ = s.repo.GetUserByID(ctx, user.ID)

	round, err := s.repo.GetLatestRound(ctx)
	if err != nil {
		return nil, err
	}
	if round == nil || round.RoundNumber < 2 {
		// Ensure round 1 exists
		if round == nil {
			roundID := int64(1)
			r1, err := s.repo.InsertRoundConfig(ctx, roundID, defaultInstructorCustomerBudget, defaultInstructorBatchSize)
			if err != nil {
				return nil, err
			}
			round = r1 // keep round 1 in the response for compatibility
		}
		// Ensure round 2 exists
		if _, err := s.repo.InsertRoundConfig(ctx, int64(2), defaultInstructorCustomerBudget, defaultInstructorBatchSize); err != nil {
			return nil, err
		}
	}

	return &AdminLoginResult{
		User:  user,
		Round: round,
	}, nil
}

// ResetGame clears all game data (guarded by upstream instructor auth).
func (s *AdminAuthService) ResetGame(ctx context.Context) error {
	if err := s.repo.ResetGame(ctx); err != nil {
		return err
	}
	return nil
}
