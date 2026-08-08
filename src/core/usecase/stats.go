package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/ports"
)

// StatsService owns round/team aggregate reads used by instructor stats and
// team summary. Response shapes are unchanged from the live FE contract.
type StatsService struct {
	repo ports.Store
	log  *slog.Logger
}

func NewStatsService(repo ports.Store, log *slog.Logger) *StatsService {
	return &StatsService{repo: repo, log: log}
}

// GetRoundStats returns the instructor leaderboard payload for a round.
func (s *StatsService) GetRoundStats(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	return s.repo.GetRoundStatsV2(ctx, roundID)
}

// GetTeamSummary returns the team card for a round.
func (s *StatsService) GetTeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	return s.repo.GetTeamSummary(ctx, roundID, teamID)
}
