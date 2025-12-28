package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// InstructorService handles instructor endpoints.
type InstructorService struct {
	repo ports.GameRepository
	log  *slog.Logger
}

func NewInstructorService(repo ports.GameRepository, log *slog.Logger) *InstructorService {
	return &InstructorService{repo: repo, log: log}
}

func (s *InstructorService) Lobby(ctx context.Context, roundID int64) (*ports.LobbySnapshot, error) {
	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) InsertConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error) {
	return s.repo.InsertRoundConfig(ctx, roundID, customerBudget, batchSize)
}

// Assign auto-assigns waiting participants into JM/QC/Customer roles.
func (s *InstructorService) Assign(ctx context.Context, roundID int64, customerCount, teamCount int) (*ports.LobbySnapshot, error) {
	teams, err := s.repo.EnsureTeamCount(ctx, teamCount)
	if err != nil {
		return nil, err
	}

	waiting, err := s.repo.ListParticipantsByStatus(ctx, roundID, domain.ParticipantWaiting)
	if err != nil {
		return nil, err
	}

	assignIdx := 0
	assign := func(role domain.Role, teamID *int64) error {
		if assignIdx >= len(waiting) {
			return nil
		}
		u := waiting[assignIdx]
		assignIdx++
		if err := s.repo.UpdateUserAssignment(ctx, u.ID, &role, teamID); err != nil {
			return err
		}
		if err := s.repo.MarkParticipantAssigned(ctx, roundID, u.ID); err != nil {
			return err
		}
		return nil
	}

	// One JM + one QC per team (best-effort with available waiting users)
	for _, team := range teams {
		tid := team.ID
		if err := assign(domain.RoleJM, &tid); err != nil {
			return nil, err
		}
		if err := assign(domain.RoleQC, &tid); err != nil {
			return nil, err
		}
		if err := s.repo.EnsureTeamRoundState(ctx, roundID, tid); err != nil {
			return nil, err
		}
	}

	for i := 0; i < customerCount; i++ {
		if err := assign(domain.RoleCustomer, nil); err != nil {
			return nil, err
		}
	}

	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) PatchUser(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) (*ports.LobbySnapshot, error) {
	if status == domain.ParticipantWaiting {
		role = nil
		teamID = nil
	}
	if err := s.repo.UpdateUserAssignment(ctx, userID, role, teamID); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateParticipantStatus(ctx, roundID, userID, status); err != nil {
		return nil, err
	}
	if status == domain.ParticipantAssigned {
		if err := s.repo.MarkParticipantAssigned(ctx, roundID, userID); err != nil {
			return nil, err
		}
	}
	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) StartRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	// Start without updating budget/batch is no longer used; see StartRoundWithConfig.
	return s.repo.StartRound(ctx, roundID, 0, 1)
}

func (s *InstructorService) EndRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	return s.repo.EndRound(ctx, roundID)
}

// StartRoundWithConfig activates a round with provided configuration.
func (s *InstructorService) StartRoundWithConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error) {
	return s.repo.StartRound(ctx, roundID, customerBudget, batchSize)
}

func (s *InstructorService) Stats(ctx context.Context, roundID int64) ([]ports.TeamStats, error) {
	return s.repo.GetRoundStats(ctx, roundID)
}

