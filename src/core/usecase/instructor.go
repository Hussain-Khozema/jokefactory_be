package usecase

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

type InstructorService struct {
	repo        ports.Store
	aiCustomers *AICustomerService
	stats       *StatsService
	log         *slog.Logger
}

func NewInstructorService(repo ports.Store, aiCustomers *AICustomerService, log *slog.Logger) *InstructorService {
	return &InstructorService{
		repo:        repo,
		aiCustomers: aiCustomers,
		stats:       NewStatsService(repo, log),
		log:         log,
	}
}

type ConfigResult struct {
	Round        *domain.Round
	IdealProfile domain.IdealProfile
}

func (s *InstructorService) Lobby(ctx context.Context, roundID int64) (*ports.LobbySnapshot, error) {
	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) GetRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	return s.repo.GetRoundByID(ctx, roundID)
}

func (s *InstructorService) GetIdealProfile(ctx context.Context, roundID int64) (domain.IdealProfile, error) {
	return s.repo.GetIdealProfile(ctx, roundID)
}

// Config persists round knobs and optional ideal profile. Missing numeric
// fields fall back to the existing round (or defaults for a new round).
func (s *InstructorService) Config(ctx context.Context, roundID int64, cfg *domain.RoundConfig, profile domain.IdealProfile) (*ConfigResult, error) {
	if err := validateRoundConfig(cfg); err != nil {
		return nil, err
	}
	if profile != nil {
		if err := scoring.ValidateIdealProfile(profile); err != nil {
			return nil, err
		}
	}

	round, err := s.repo.InsertRoundConfig(ctx, roundID, cfg)
	if err != nil {
		return nil, err
	}

	storedProfile := domain.IdealProfile(nil)
	if profile != nil {
		if err := s.repo.UpsertIdealProfile(ctx, roundID, profile); err != nil {
			return nil, err
		}
		storedProfile = profile
	} else {
		storedProfile, err = s.repo.GetIdealProfile(ctx, roundID)
		if err != nil {
			return nil, err
		}
	}

	return &ConfigResult{Round: round, IdealProfile: storedProfile}, nil
}

func validateRoundConfig(cfg *domain.RoundConfig) error {
	if cfg.BatchSize < 1 {
		return domain.NewValidationError("batch_size", "must be >= 1")
	}
	if cfg.CustomerBudget < 0 || cfg.MarketPrice < 0 || cfg.CostOfPublishing < 0 || cfg.CostOfDiscard < 0 {
		return domain.NewValidationError("config", "costs and budget must be >= 0")
	}
	if cfg.CustomerCount < 1 {
		return domain.NewValidationError("customer_count", "must be >= 1")
	}
	if cfg.FeedbackJokeCount < 1 {
		return domain.NewValidationError("feedback_joke_count", "must be >= 1")
	}
	if cfg.FeedbackPassThreshold < 0 || cfg.FeedbackPassThreshold > 1 {
		return domain.NewValidationError("feedback_pass_threshold", "must be between 0 and 1")
	}
	if cfg.Jitter < 0 {
		return domain.NewValidationError("jitter", "must be >= 0")
	}
	if cfg.SwapMargin < 0 {
		return domain.NewValidationError("swap_margin", "must be >= 0")
	}
	return nil
}

// Assign creates N teams and assigns one JM + one MARKETING per team.
func (s *InstructorService) Assign(ctx context.Context, roundID int64, teamCount int) (*ports.LobbySnapshot, error) {
	if teamCount < 1 {
		return nil, domain.NewValidationError("team_count", "must be >= 1")
	}
	if _, err := s.repo.GetRoundByID(ctx, roundID); err != nil {
		return nil, err
	}

	teams, err := s.repo.EnsureTeamCount(ctx, teamCount)
	if err != nil {
		return nil, err
	}
	participants, err := s.loadAssignableParticipants(ctx)
	if err != nil {
		return nil, err
	}

	assignIdx := 0
	for _, team := range teams {
		tid := team.ID
		if err := s.assignNext(ctx, participants, &assignIdx, domain.RoleJM, &tid); err != nil {
			return nil, err
		}
		if err := s.assignNext(ctx, participants, &assignIdx, domain.RoleMarketing, &tid); err != nil {
			return nil, err
		}
		if err := s.repo.EnsureTeamRoundState(ctx, roundID, tid); err != nil {
			return nil, err
		}
	}

	if err := s.unassignRemainder(ctx, participants, assignIdx); err != nil {
		return nil, err
	}
	return s.repo.GetLobby(ctx, roundID)
}

func (s *InstructorService) loadAssignableParticipants(ctx context.Context) ([]domain.User, error) {
	waiting, err := s.repo.ListUsersByStatus(ctx, domain.ParticipantWaiting)
	if err != nil {
		return nil, err
	}
	assigned, err := s.repo.ListUsersByStatus(ctx, domain.ParticipantAssigned)
	if err != nil {
		return nil, err
	}
	participants := waiting
	participants = append(participants, assigned...)
	if len(participants) > 1 {
		r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // shuffle only; not security-sensitive
		r.Shuffle(len(participants), func(i, j int) {
			participants[i], participants[j] = participants[j], participants[i]
		})
	}
	return participants, nil
}

func (s *InstructorService) assignNext(ctx context.Context, participants []domain.User, idx *int, role domain.Role, teamID *int64) error {
	if *idx >= len(participants) {
		return nil
	}
	u := participants[*idx]
	*idx++
	if err := s.repo.UpdateUserAssignment(ctx, u.ID, &role, teamID); err != nil {
		return err
	}
	return s.repo.MarkUserAssigned(ctx, u.ID)
}

func (s *InstructorService) unassignRemainder(ctx context.Context, participants []domain.User, from int) error {
	for i := from; i < len(participants); i++ {
		u := participants[i]
		if err := s.repo.UpdateUserAssignment(ctx, u.ID, nil, nil); err != nil {
			return err
		}
		if err := s.repo.UpdateUserStatus(ctx, u.ID, domain.ParticipantWaiting); err != nil {
			return err
		}
	}
	return nil
}

func (s *InstructorService) PatchUser(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) (*ports.LobbySnapshot, error) {
	existing, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	desiredRole := existing.Role
	desiredTeamID := existing.TeamID
	if role != nil {
		desiredRole = role
	}
	if teamID != nil {
		desiredTeamID = teamID
	}
	if status == domain.ParticipantWaiting {
		desiredRole = nil
		desiredTeamID = nil
	}

	if desiredRole != nil {
		switch *desiredRole {
		case domain.RoleInstructor:
			desiredTeamID = nil
		case domain.RoleJM, domain.RoleMarketing:
			if desiredTeamID == nil {
				return nil, domain.NewValidationError("team_id", "team_id is required for JM/MARKETING roles")
			}
		default:
			return nil, domain.NewValidationError("role", "unsupported role")
		}
	}

	if err := s.repo.PatchUserInRound(ctx, roundID, userID, status, desiredRole, desiredTeamID); err != nil {
		return nil, err
	}
	return s.repo.GetLobby(ctx, roundID)
}

// StartRound activates a configured round. Ideal profile must already be set.
func (s *InstructorService) StartRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status == domain.RoundActive {
		return nil, domain.NewConflictError("round already active")
	}
	if round.Status == domain.RoundEnded {
		return nil, domain.NewConflictError("round already ended")
	}

	profile, err := s.repo.GetIdealProfile(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if err := scoring.ValidateIdealProfile(profile); err != nil {
		return nil, domain.NewConflictError("ideal_profile must be configured before start")
	}

	round, err = s.repo.StartRound(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if s.aiCustomers != nil {
		if err := s.aiCustomers.GenerateCustomers(ctx, round); err != nil {
			return nil, err
		}
	}
	return round, nil
}

func (s *InstructorService) EndRound(ctx context.Context, roundID int64) (*domain.Round, error) {
	round, err := s.repo.GetRoundByID(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round.Status != domain.RoundActive {
		return nil, domain.NewConflictError("round not active")
	}
	return s.repo.EndRound(ctx, roundID)
}

func (s *InstructorService) SetPopupState(ctx context.Context, roundID int64, isActive bool) (*domain.Round, error) {
	return s.repo.SetRoundPopupState(ctx, roundID, isActive)
}

func (s *InstructorService) Stats(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	return s.stats.GetRoundStats(ctx, roundID)
}

func (s *InstructorService) DeleteUser(ctx context.Context, roundID, userID int64) error {
	_ = roundID
	return s.repo.DeleteUser(ctx, userID)
}

// MergeConfig overlays non-nil DTO fields onto a base RoundConfig.
func MergeConfig(base *domain.RoundConfig, overlay *PartialRoundConfig) domain.RoundConfig {
	cfg := *base
	if overlay.CustomerBudget != nil {
		cfg.CustomerBudget = *overlay.CustomerBudget
	}
	if overlay.BatchSize != nil {
		cfg.BatchSize = *overlay.BatchSize
	}
	if overlay.MarketPrice != nil {
		cfg.MarketPrice = *overlay.MarketPrice
	}
	if overlay.CostOfPublishing != nil {
		cfg.CostOfPublishing = *overlay.CostOfPublishing
	}
	if overlay.CostOfDiscard != nil {
		cfg.CostOfDiscard = *overlay.CostOfDiscard
	}
	if overlay.CustomerCount != nil {
		cfg.CustomerCount = *overlay.CustomerCount
	}
	if overlay.BuyThreshold != nil {
		cfg.BuyThreshold = *overlay.BuyThreshold
	}
	if overlay.Jitter != nil {
		cfg.Jitter = *overlay.Jitter
	}
	if overlay.SwapMargin != nil {
		cfg.SwapMargin = *overlay.SwapMargin
	}
	if overlay.FeedbackJokeCount != nil {
		cfg.FeedbackJokeCount = *overlay.FeedbackJokeCount
	}
	if overlay.FeedbackPassThreshold != nil {
		cfg.FeedbackPassThreshold = *overlay.FeedbackPassThreshold
	}
	return cfg
}

// PartialRoundConfig carries optional config fields from the wire.
type PartialRoundConfig struct {
	CustomerBudget        *float64
	BatchSize             *int
	MarketPrice           *float64
	CostOfPublishing      *float64
	CostOfDiscard         *float64
	CustomerCount         *int
	BuyThreshold          *float64
	Jitter                *float64
	SwapMargin            *float64
	FeedbackJokeCount     *int
	FeedbackPassThreshold *float64
}
