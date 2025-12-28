package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// RoundService handles round-level queries.
type RoundService struct {
	repo ports.GameRepository
	log  *slog.Logger
}

func NewRoundService(repo ports.GameRepository, log *slog.Logger) *RoundService {
	return &RoundService{repo: repo, log: log}
}

// Active returns the active round, if any.
func (s *RoundService) Active(ctx context.Context) (*domain.Round, error) {
	return s.repo.GetActiveRound(ctx)
}

// TeamSummary returns stats for a team in a round.
func (s *RoundService) TeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	return s.repo.GetTeamSummary(ctx, roundID, teamID)
}

