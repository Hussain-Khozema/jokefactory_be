package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// CustomerService handles market flows.
type CustomerService struct {
	repo ports.GameRepository
	log  *slog.Logger
}

func NewCustomerService(repo ports.GameRepository, log *slog.Logger) *CustomerService {
	return &CustomerService{repo: repo, log: log}
}

func (s *CustomerService) Market(ctx context.Context, userID, roundID int64) ([]ports.MarketItem, error) {
	if err := s.ensureCustomerOrInstructor(ctx, userID); err != nil {
		return nil, err
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, domain.NewConflictError("round not active")
	}
	if _, err := s.repo.EnsureCustomerBudget(ctx, roundID, userID, round.CustomerBudget); err != nil {
		return nil, err
	}
	return s.repo.ListMarket(ctx, roundID, userID)
}

func (s *CustomerService) Budget(ctx context.Context, userID, roundID int64) (*domain.CustomerRoundBudget, error) {
	if err := s.ensureCustomer(ctx, userID); err != nil {
		return nil, err
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	return s.repo.EnsureCustomerBudget(ctx, roundID, userID, round.CustomerBudget)
}

func (s *CustomerService) Buy(ctx context.Context, userID, roundID, jokeID int64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error) {
	if err := s.ensureCustomer(ctx, userID); err != nil {
		return nil, nil, 0, err
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, nil, 0, err
	}
	if round.Status != domain.RoundActive {
		return nil, nil, 0, domain.NewConflictError("round not active")
	}
	if _, err := s.repo.EnsureCustomerBudget(ctx, roundID, userID, round.CustomerBudget); err != nil {
		return nil, nil, 0, err
	}
	return s.repo.BuyJoke(ctx, roundID, userID, jokeID)
}

func (s *CustomerService) Return(ctx context.Context, userID, roundID, jokeID int64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error) {
	if err := s.ensureCustomer(ctx, userID); err != nil {
		return nil, nil, 0, err
	}
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, nil, 0, err
	}
	if round.Status != domain.RoundActive {
		return nil, nil, 0, domain.NewConflictError("round not active")
	}
	if _, err := s.repo.EnsureCustomerBudget(ctx, roundID, userID, round.CustomerBudget); err != nil {
		return nil, nil, 0, err
	}
	return s.repo.ReturnJoke(ctx, roundID, userID, jokeID)
}

func (s *CustomerService) ensureCustomer(ctx context.Context, userID int64) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Role == nil || *user.Role != domain.RoleCustomer {
		return domain.NewForbiddenError("user must be customer")
	}
	return nil
}

// ensureCustomerOrInstructor allows customers (normal path) and instructors (for viewing/monitoring).
func (s *CustomerService) ensureCustomerOrInstructor(ctx context.Context, userID int64) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Role == nil || (*user.Role != domain.RoleCustomer && *user.Role != domain.RoleInstructor) {
		return domain.NewForbiddenError("user must be customer or instructor")
	}
	return nil
}

