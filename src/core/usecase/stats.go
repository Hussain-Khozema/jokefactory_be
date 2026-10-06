package usecase

import (
	"context"
	"log/slog"
	"time"

	"jokefactory/src/core/ports"
)

// ArrivalServer serves Lean customers whose arrival time has passed. It is a
// no-op for CLASSIC rounds and for rounds that are not ACTIVE.
type ArrivalServer interface {
	ServeArrivals(ctx context.Context, roundID int64, now time.Time) error
}

// StatsService owns round/team aggregate reads used by instructor stats and
// team summary. Response shapes are unchanged from the live FE contract.
type StatsService struct {
	repo ports.Store
	// arrivals, when set, serves due Lean customers before each read so the
	// polled numbers include them. Nil means reads never serve.
	arrivals ArrivalServer
	log      *slog.Logger
}

func NewStatsService(repo ports.Store, log *slog.Logger) *StatsService {
	return &StatsService{repo: repo, log: log}
}

// NewStatsServiceWithArrivals is NewStatsService plus lazy Lean arrivals on
// every read. arrivals may be nil.
func NewStatsServiceWithArrivals(repo ports.Store, arrivals ArrivalServer, log *slog.Logger) *StatsService {
	return &StatsService{repo: repo, arrivals: arrivals, log: log}
}

// GetRoundStats returns the instructor leaderboard payload for a round. A
// LEAN round reads the Lean query (setup and carrying costs in profit, plus a
// lean block); every other round reads the Classic query unchanged.
func (s *StatsService) GetRoundStats(ctx context.Context, roundID int64) (*ports.RoundStats, error) {
	s.serveArrivals(ctx, roundID)
	if s.isLean(ctx, roundID) {
		return s.repo.GetRoundStatsLean(ctx, roundID)
	}
	return s.repo.GetRoundStatsV2(ctx, roundID)
}

// GetTeamSummary returns the team card for a round, Lean-costed for a LEAN round.
func (s *StatsService) GetTeamSummary(ctx context.Context, roundID, teamID int64) (*ports.TeamSummary, error) {
	s.serveArrivals(ctx, roundID)
	if s.isLean(ctx, roundID) {
		return s.repo.GetTeamSummaryLean(ctx, roundID, teamID)
	}
	return s.repo.GetTeamSummary(ctx, roundID, teamID)
}

// isLean reports whether the round is LEAN. A round that cannot be loaded is
// treated as CLASSIC, so the Classic query keeps its existing behaviour (and
// its existing errors) for unknown rounds.
func (s *StatsService) isLean(ctx context.Context, roundID int64) bool {
	round, err := s.repo.GetRoundByID(ctx, roundID)
	return err == nil && round != nil && round.MarketMode.IsLean()
}

// serveArrivals serves due Lean customers. ServeArrivals itself returns early
// unless the round is LEAN and ACTIVE, and a failure is logged, never
// returned: a read must not fail because serving did.
func (s *StatsService) serveArrivals(ctx context.Context, roundID int64) {
	if s.arrivals == nil {
		return
	}
	if err := s.arrivals.ServeArrivals(ctx, roundID, time.Now().UTC()); err != nil {
		s.log.Error("serve arrivals on stats read", "round_id", roundID, "error", err)
	}
}

// arrivalServer converts a possibly-nil *AICustomerService into an
// ArrivalServer without producing a non-nil interface around a nil pointer.
func arrivalServer(ai *AICustomerService) ArrivalServer {
	if ai == nil {
		return nil
	}
	return ai
}
