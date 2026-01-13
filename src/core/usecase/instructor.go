package usecase

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

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

// GetRound returns a round by id.
func (s *InstructorService) GetRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	return s.repo.GetRoundByID(ctx, roundID)
}

func (s *InstructorService) InsertConfig(ctx context.Context, roundID int64, customerBudget, batchSize int, marketPrice, costOfPublishing float64) (*domain.Round, error) {
	return s.repo.InsertRoundConfig(ctx, roundID, customerBudget, batchSize, marketPrice, costOfPublishing)
}

// Assign auto-assigns waiting participants into JM/QC/Customer roles.
func (s *InstructorService) Assign(ctx context.Context, roundID int64, customerCount, teamCount int) (*ports.LobbySnapshot, error) {
	teams, err := s.repo.EnsureTeamCount(ctx, teamCount)
	if err != nil {
		return nil, err
	}

	waiting, err := s.repo.ListUsersByStatus(ctx, domain.ParticipantWaiting)
	if err != nil {
		return nil, err
	}

	assigned, err := s.repo.ListUsersByStatus(ctx, domain.ParticipantAssigned)
	if err != nil {
		return nil, err
	}

	participants := append(waiting, assigned...)

	if len(participants) > 1 {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		r.Shuffle(len(participants), func(i, j int) {
			participants[i], participants[j] = participants[j], participants[i]
		})
	}

	assignIdx := 0
	assign := func(role domain.Role, teamID *int64) error {
		if assignIdx >= len(participants) {
			return nil
		}
		u := participants[assignIdx]
		assignIdx++
		if err := s.repo.UpdateUserAssignment(ctx, u.ID, &role, teamID); err != nil {
			return err
		}
		if err := s.repo.MarkUserAssigned(ctx, u.ID); err != nil {
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

	// Any remaining participants should return to waiting with no assignment.
	for ; assignIdx < len(participants); assignIdx++ {
		u := participants[assignIdx]
		if err := s.repo.UpdateUserAssignment(ctx, u.ID, nil, nil); err != nil {
			return nil, err
		}
		if err := s.repo.UpdateUserStatus(ctx, u.ID, domain.ParticipantWaiting); err != nil {
			return nil, err
		}
	}

	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) PatchUser(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) (*ports.LobbySnapshot, error) {
	// Fetch existing user so PATCH can support partial updates:
	// - role omitted => keep existing role
	// - team_id omitted => keep existing team
	existing, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	desiredRole := existing.Role
	desiredTeamID := existing.TeamID

	// Apply explicit changes from request (if provided)
	if role != nil {
		desiredRole = role
	}
	if teamID != nil {
		desiredTeamID = teamID
	}

	// Waiting participants must have no assignment.
	if status == domain.ParticipantWaiting {
		desiredRole = nil
		desiredTeamID = nil
	}

	// Enforce role/team constraints
	if desiredRole != nil {
		switch *desiredRole {
		case domain.RoleCustomer, domain.RoleInstructor:
			// These roles must not have a team.
			desiredTeamID = nil
		case domain.RoleJM, domain.RoleQC:
			// JM/QC must have a team (either provided or already assigned).
			if desiredTeamID == nil {
				return nil, domain.NewValidationError("team_id", "team_id is required for JM/QC roles")
			}
		default:
			// Future roles: leave team untouched.
		}
	}

	// Atomic update in repo (also syncs customer_round_budget when role changes to/from CUSTOMER)
	if err := s.repo.PatchUserInRound(ctx, roundID, userID, status, desiredRole, desiredTeamID); err != nil {
		return nil, err
	}
	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) StartRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	// Start without updating budget/batch is no longer used; see StartRoundWithConfig.
	return s.repo.StartRound(ctx, roundID, 0, 1, 1, 0.1)
}

func (s *InstructorService) EndRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	return s.repo.EndRound(ctx, roundID)
}

// SetPopupState toggles whether popups are active for a round.
func (s *InstructorService) SetPopupState(ctx context.Context, roundID int64, isActive bool) (*domain.Round, error) {
	return s.repo.SetRoundPopupState(ctx, roundID, isActive)
}

// StartRoundWithConfig activates a round with provided configuration.
func (s *InstructorService) StartRoundWithConfig(ctx context.Context, roundID int64, customerBudget, batchSize int, marketPrice, costOfPublishing float64) (*domain.Round, error) {
	return s.repo.StartRound(ctx, roundID, customerBudget, batchSize, marketPrice, costOfPublishing)
}

func (s *InstructorService) Stats(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	stats, err := s.repo.GetRoundStatsV2(ctx, roundID)
	if err != nil {
		s.log.Error("instructor stats failed", "round_id", roundID, "error", err)
		return nil, err
	}
	return stats, nil
}

// DeleteUser removes a non-instructor user from the database.
func (s *InstructorService) DeleteUser(ctx context.Context, roundID, userID int64) error {
	return s.repo.DeleteUser(ctx, userID)
}
