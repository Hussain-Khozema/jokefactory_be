package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

// feesFixture is a running Round 2 (batch_size 10) at the default fees, with
// storage pinned at the old default $0.10 so the arithmetic below holds
// ($1 / $2 / $3 shipping, storage $0.10 and hosting $0.05 per joke-minute,
// price $1, listing $0.10, discard $0.01), read at T+30min:
//
//	Team 1: batch of 5 sent T, released T+2 (WIP 5×2 = 10): 1 published at
//	        T+2 and sold at T+6 (hosting 4 min), 4 discarded. Batch of 3 and
//	        batch of 8 sent and released at T+3 / T+4, all discarded. An
//	        unsplit batch sent T+29, still on the desk (WIP 5×1 = 5).
//	        shipping 2+1+3+2 = 8.00; storage 0.10×15 = 1.50; hosting
//	        0.05×4 = 0.20; profit 1 − 0.10 − 0.15 − 8 − 1.50 − 0.20 = −8.95
//	Team 2: batches of exactly 4, 5 and 6, each released the moment it was
//	        sent, all discarded: shipping 1+2+3 = 6.00, no storage, no
//	        hosting; profit −0.15 − 6 = −6.15
func feesFixture(t *testing.T) (*testutil.Store, *domain.Round) {
	t.Helper()
	ctx := context.Background()
	st := testutil.NewStore()
	st.Teams = []domain.Team{{ID: 1, Name: "Team 1"}, {ID: 2, Name: "Team 2"}}
	cfg := testutil.FiveFees(domain.DefaultRoundConfigFor(2))
	cfg.StorageFeePerJokeMinute = 0.10
	if _, err := st.InsertRoundConfig(ctx, 2, &cfg); err != nil {
		t.Fatal(err)
	}
	round, err := st.StartRound(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	at := func(min int) *time.Time {
		v := round.StartedAt.Add(time.Duration(min) * time.Minute)
		return &v
	}
	now := *at(30)
	st.Now = func() time.Time { return now }

	title := "t"
	nextJoke := int64(1)
	jokes := func(n, published int, publishedAt *time.Time) []domain.Joke {
		out := make([]domain.Joke, 0, n)
		for i := 0; i < n; i++ {
			j := domain.Joke{ID: nextJoke, Text: "j", PublishStatus: domain.PublishDiscarded}
			if i < published {
				j.Title, j.PublishStatus, j.PublishedAt = &title, domain.PublishPublished, publishedAt
			}
			nextJoke++
			out = append(out, j)
		}
		return out
	}
	nextBatch := int64(1)
	batch := func(team int64, sent, processed *time.Time, js []domain.Joke) {
		status := domain.BatchSubmitted
		if processed != nil {
			status = domain.BatchProcessed
		}
		b := &domain.Batch{
			ID: nextBatch, RoundID: 2, TeamID: team, Status: status,
			SubmittedAt: sent, ProcessedAt: processed, CreatedAt: *sent, Jokes: js,
		}
		for i := range b.Jokes {
			b.Jokes[i].BatchID = b.ID
		}
		if len(js) == 0 {
			raw := "an unsplit blob of several jokes"
			b.RawText, b.RawTextOriginal = &raw, &raw
		}
		st.Batches[b.ID] = b
		nextBatch++
	}
	batch(1, at(0), at(2), jokes(5, 1, at(2)))
	batch(1, at(3), at(3), jokes(3, 0, nil))
	batch(1, at(4), at(4), jokes(8, 0, nil))
	batch(1, at(29), nil, nil)
	batch(2, at(1), at(1), jokes(4, 0, nil))
	batch(2, at(2), at(2), jokes(5, 0, nil))
	batch(2, at(3), at(3), jokes(6, 0, nil))
	st.NextBatch, st.NextJoke = nextBatch, nextJoke

	team1 := st.TeamState[[2]int64{2, 1}]
	team1.BatchesCreated, team1.BatchesProcessed, team1.PublishedJokes, team1.DiscardedJokes = 4, 3, 1, 15
	team2 := st.TeamState[[2]int64{2, 2}]
	team2.BatchesCreated, team2.BatchesProcessed, team2.DiscardedJokes = 3, 3, 15

	st.AICustomers[1] = &domain.AICustomer{ID: 1, RoundID: 2, StartingBudget: 3, RemainingBudget: 3}
	st.NextAICust = 2
	if err := st.BuyJoke(ctx, 2, 1, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	// Stamp the sale at T+6: BuyJoke uses the wall clock.
	st.PurchaseEvents[len(st.PurchaseEvents)-1].CreatedAt = *at(6)
	for _, p := range st.Purchases {
		p.CreatedAt = *at(6)
	}
	return st, round
}

func TestLeanFeesInStatsAndCharts(t *testing.T) {
	ctx := context.Background()
	st, round := feesFixture(t)
	svc := usecase.NewStatsService(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	want := map[int64]struct{ shipping, storage, hosting, profit float64 }{
		1: {8, 1.50, 0.20, -8.95},
		2: {6, 0, 0, -6.15},
	}

	stats, err := svc.GetRoundStats(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Leaderboard) != 2 {
		t.Fatalf("leaderboard %+v", stats.Leaderboard)
	}
	for _, row := range stats.Leaderboard {
		w, l := want[row.Team.ID], row.Lean
		if l == nil {
			t.Fatalf("team %d: no lean block", row.Team.ID)
		}
		if !near(l.ShippingCost, w.shipping) || !near(l.StorageCost, w.storage) || !near(l.HostingCost, w.hosting) ||
			!near(row.Profit, w.profit) {
			t.Errorf("team %d: shipping %.4f storage %.4f hosting %.4f profit %.4f; want %+v",
				row.Team.ID, l.ShippingCost, l.StorageCost, l.HostingCost, row.Profit, w)
		}
		if !near(l.SetupCost, l.ShippingCost) || !near(l.CarryingCost, l.StorageCost+l.HostingCost) {
			t.Errorf("team %d: aliases setup %.4f carrying %.4f do not match shipping %.4f, storage + hosting %.4f",
				row.Team.ID, l.SetupCost, l.CarryingCost, l.ShippingCost, l.StorageCost+l.HostingCost)
		}

		sum, err := svc.GetTeamSummary(ctx, round.ID, row.Team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if sum.Lean == nil || !near(sum.Lean.ShippingCost, w.shipping) || !near(sum.Lean.StorageCost, w.storage) ||
			!near(sum.Lean.HostingCost, w.hosting) || !near(sum.Profit, w.profit) {
			t.Errorf("team %d summary: %+v profit %.4f; want %+v", row.Team.ID, sum.Lean, sum.Profit, w)
		}
	}

	charts, err := svc.GetLeanCharts(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, team := range charts.Teams {
		w := want[team.TeamID]
		if !near(team.Waste.Setup, w.shipping) || !near(team.Waste.Waiting, w.storage) || !near(team.Waste.Shelf, w.hosting) {
			t.Errorf("team %d waste %+v; want setup = shipping %.2f, waiting = storage %.2f, shelf = hosting %.2f",
				team.TeamID, team.Waste, w.shipping, w.storage, w.hosting)
		}
		if team.TeamID == 1 {
			last := team.Batches[len(team.Batches)-1]
			if last.Jokes != 5 {
				t.Errorf("unsplit batch shows %d jokes; want 5 (not batch_size 10)", last.Jokes)
			}
		}
	}
}

// Review focus 1: a pasted batch nobody splits before the round ends ships
// at the standard fee and stores 5 jokes all the way to the round's end.
func TestUnsplitBatchIsChargedUntilTheRoundEnds(t *testing.T) {
	ctx := context.Background()
	st, round := feesFixture(t)
	instructor := usecase.NewInstructorService(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := instructor.EndRound(ctx, round.ID); err != nil {
		t.Fatal(err)
	}
	ended := round.StartedAt.Add(31 * time.Minute)
	st.Rounds[round.ID].EndedAt = &ended
	// Long after the end: the clocks stopped at ended_at.
	later := ended.Add(2 * time.Hour)
	st.Now = func() time.Time { return later }

	svc := usecase.NewStatsService(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sum, err := svc.GetTeamSummary(ctx, round.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Storage: 10 joke-min (first batch) + 5 jokes × 2 min (T+29 → T+31).
	// Hosting: still 4 min; the one released joke sold at T+6.
	if math.Abs(sum.Lean.ShippingCost-8) > 1e-9 || math.Abs(sum.Lean.StorageCost-2.0) > 1e-9 ||
		math.Abs(sum.Lean.HostingCost-0.20) > 1e-9 {
		t.Fatalf("after the end: %+v; want shipping 8.00, storage 2.00, hosting 0.20", sum.Lean)
	}
}
