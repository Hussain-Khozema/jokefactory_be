package usecase

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase/testutil"
)

// seedLeanStatsFixture builds the scenario TestLeanStatsComputeSetupAndCarrying
// describes, with T = round.StartedAt: one team, one batch of 5 submitted at T,
// processed at T+6min with 3 published and 2 discarded, and one published joke
// bought at T+10min.
func seedLeanStatsFixture(t *testing.T, st *testutil.Store) (*domain.Round, int64) {
	t.Helper()
	ctx := context.Background()
	cfg := domain.DefaultRoundConfig()
	cfg.MarketMode = domain.MarketLean
	cfg.BatchSize = 5
	cfg.MarketPrice = 1
	cfg.CostOfPublishing = 0.10
	cfg.CostOfDiscard = 0.01
	// Flat $3 shipping and one $0.02 rate for storage and hosting keep the
	// old setup $3 / carrying $0.02 arithmetic of the tests built on this.
	cfg.ShippingFeeSmall, cfg.ShippingFeeStandard, cfg.ShippingFeeLarge = 3, 3, 3
	cfg.StorageFeePerJokeMinute = 0.02
	cfg.HostingFeePerJokeMinute = 0.02
	if _, err := st.InsertRoundConfig(ctx, 1, &cfg); err != nil {
		t.Fatal(err)
	}
	const teamID int64 = 1
	st.Teams = []domain.Team{{ID: teamID, Name: "Team 1"}}
	if _, err := st.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}
	round, err := st.GetRoundByID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	at := func(min int) *time.Time {
		v := round.StartedAt.Add(time.Duration(min) * time.Minute)
		return &v
	}
	title := "t"
	jokes := make([]domain.Joke, 0, 5)
	for i := int64(1); i <= 5; i++ {
		j := domain.Joke{ID: 100 + i, BatchID: 1, Text: "j", PublishStatus: domain.PublishDiscarded}
		if i <= 3 {
			j.Title = &title
			j.PublishStatus = domain.PublishPublished
			j.PublishedAt = at(6)
		}
		jokes = append(jokes, j)
	}
	st.Batches[1] = &domain.Batch{
		ID: 1, RoundID: 1, TeamID: teamID, Status: domain.BatchProcessed,
		SubmittedAt: at(0), ProcessedAt: at(6), CreatedAt: *at(0), Jokes: jokes,
	}
	st.NextBatch, st.NextJoke = 2, 106

	state := st.TeamState[[2]int64{1, teamID}]
	state.BatchesCreated, state.BatchesProcessed = 1, 1
	state.PublishedJokes, state.DiscardedJokes = 3, 2

	st.AICustomers[1] = &domain.AICustomer{ID: 1, RoundID: 1, StartingBudget: 3, RemainingBudget: 3}
	st.NextAICust = 2
	if err := st.BuyJoke(ctx, 1, 1, 101, teamID, 1); err != nil {
		t.Fatal(err)
	}
	// Stamp the sale at T+10min: BuyJoke uses the wall clock.
	st.PurchaseEvents[len(st.PurchaseEvents)-1].CreatedAt = *at(10)
	for _, p := range st.Purchases {
		p.CreatedAt = *at(10)
	}
	return round, teamID
}

// One team, one batch of 5 submitted at T, processed at T+6min with 3
// published and 2 discarded; one published joke sold at T+10min; now = T+16min;
// shipping $3, storage and hosting $0.02/joke-min, price $1, publishing $0.10,
// discard $0.01.
//
//	WIP:        5 jokes × 6 min                     = 30 joke-min
//	Inventory:  sold joke 4 min (6→10) + 2 unsold × 10 min (6→16) = 24 joke-min
//	storage = 0.02 × 30 = 0.60 ; hosting = 0.02 × 24 = 0.48 ; shipping = 3
//	carrying = 1.08 ; setup = 3
//	profit   = 1×1 − 3×0.10 − 2×0.01 − 3 − 1.08 = −3.40
func TestLeanStatsComputeSetupAndCarrying(t *testing.T) {
	st := testutil.NewStore()
	round, teamID := seedLeanStatsFixture(t, st) // builds exactly the scenario above
	svc := NewStatsService(st, slog.Default())
	now := round.StartedAt.Add(16 * time.Minute)
	st.Now = func() time.Time { return now }

	stats, err := svc.GetRoundStats(context.Background(), round.ID)
	if err != nil {
		t.Fatal(err)
	}
	row := stats.Leaderboard[0]
	if row.Team.ID != teamID || row.Lean == nil {
		t.Fatalf("no lean block: %+v", row)
	}
	approx := func(got, want float64) bool { return math.Abs(got-want) < 0.005 }
	if !approx(row.Lean.SetupCost, 3) || !approx(row.Lean.CarryingCost, 1.08) || !approx(row.Profit, -3.40) {
		t.Fatalf("setup=%.2f carrying=%.2f profit=%.2f", row.Lean.SetupCost, row.Lean.CarryingCost, row.Profit)
	}
	if row.Lean.BatchesSubmitted != 1 || !approx(row.Lean.AvgSubmitToPublishS, 360) || !approx(row.Lean.AvgPublishToFirstSaleS, 240) {
		t.Fatalf("lean block: %+v", *row.Lean)
	}
	if len(row.Lean.BatchSeries) != 1 || row.Lean.BatchSeries[0].Published != 3 || row.Lean.BatchSeries[0].Sold != 1 {
		t.Fatalf("series: %+v", row.Lean.BatchSeries)
	}

	sum, err := svc.GetTeamSummary(context.Background(), round.ID, teamID)
	if err != nil || sum.Lean == nil || !approx(sum.Lean.CarryingCost, 1.08) || !approx(sum.Profit, -3.40) {
		t.Fatalf("summary: %+v err=%v", sum, err)
	}
}

func TestClassicStatsHaveNoLeanBlockAndUnchangedProfit(t *testing.T) {
	st := testutil.NewStore()
	round, _ := seedLeanStatsFixture(t, st)
	round.MarketMode = domain.MarketClassic
	st.Rounds[round.ID] = round
	svc := NewStatsService(st, slog.Default())
	stats, _ := svc.GetRoundStats(context.Background(), round.ID)
	row := stats.Leaderboard[0]
	if row.Lean != nil || math.Abs(row.Profit-(1-0.30-0.02)) > 0.005 {
		t.Fatalf("classic row changed: %+v", row)
	}
}
