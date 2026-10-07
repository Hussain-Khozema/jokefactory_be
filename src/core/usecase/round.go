package usecase

import (
	"context"
	"log/slog"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

// RoundService handles round-level queries.
type RoundService struct {
	repo  ports.Store
	stats *StatsService
	log   *slog.Logger
}

func NewRoundService(repo ports.Store, log *slog.Logger) *RoundService {
	return &RoundService{repo: repo, stats: NewStatsService(repo, log), log: log}
}

// Active returns the active round, if any.
func (s *RoundService) Active(ctx context.Context) (*domain.Round, error) {
	return s.repo.GetActiveRound(ctx)
}

// List returns all rounds.
func (s *RoundService) List(ctx context.Context) ([]domain.Round, error) {
	return s.repo.ListRounds(ctx)
}

// TeamSummary returns stats for a team in a round.
func (s *RoundService) TeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	return s.stats.GetTeamSummary(ctx, roundID, teamID)
}

// TeamLeanCharts returns one team's lean charts plus the class average.
func (s *RoundService) TeamLeanCharts(ctx context.Context, roundID, teamID int64) (*LeanCharts, error) {
	return s.stats.GetTeamLeanCharts(ctx, roundID, teamID)
}

// NewRoundServiceWithArrivals is NewRoundService whose team summary also
// serves due Lean customers. aiCustomers may be nil.
func NewRoundServiceWithArrivals(repo ports.Store, aiCustomers *AICustomerService, log *slog.Logger) *RoundService {
	return &RoundService{
		repo:  repo,
		stats: NewStatsServiceWithArrivals(repo, arrivalServer(aiCustomers), log),
		log:   log,
	}
}
