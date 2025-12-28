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

	return &AdminLoginResult{
		User:  user,
		Round: round,
	}, nil
}

