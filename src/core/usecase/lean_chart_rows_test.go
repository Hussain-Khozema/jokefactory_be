package usecase

import (
	"context"
	"math"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase/testutil"
)

// The lean stats fixture (lean_stats_test.go) at T+16min: one batch of 5 sent
// at T, released at T+6min with 3 published and 2 discarded, one published
// joke first sold at T+10min.
func TestMemStoreLeanChartRowsMirrorTheLeanStats(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	round, teamID := seedLeanStatsFixture(t, st)
	now := round.StartedAt.Add(16 * time.Minute)
	st.Now = func() time.Time { return now }
	at := func(min int) time.Time { return round.StartedAt.Add(time.Duration(min) * time.Minute) }
	approx := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

	rows, err := st.GetLeanChartRows(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.ClockEnd.Equal(now) {
		t.Fatalf("clock end %v; want %v", rows.ClockEnd, now)
	}
	if len(rows.Teams) != 1 {
		t.Fatalf("teams %+v", rows.Teams)
	}
	tr := rows.Teams[0]
	if tr.TeamID != teamID || tr.TeamName != "Team 1" || tr.BatchesSubmitted != 1 || tr.DiscardedJokes != 2 ||
		!approx(tr.WIPJokeMinutes, 30) || !approx(tr.InventoryJokeMinutes, 24) {
		t.Fatalf("team row %+v; want 1 submitted, 2 discarded, WIP 30, inventory 24", tr)
	}
	if len(rows.Batches) != 1 {
		t.Fatalf("batches %+v", rows.Batches)
	}
	b := rows.Batches[0]
	if b.TeamID != teamID || !b.SubmittedAt.Equal(at(0)) || b.ProcessedAt == nil || !b.ProcessedAt.Equal(at(6)) ||
		b.JokeRows != 5 || b.Published != 3 || b.Sold != 1 || b.FirstSoldAt == nil || !b.FirstSoldAt.Equal(at(10)) {
		t.Fatalf("batch row %+v", b)
	}
	// Discarded jokes never appear: only the 3 published jokes, one of them sold.
	if len(rows.Jokes) != 3 {
		t.Fatalf("jokes %+v; want the 3 published", rows.Jokes)
	}
	sold := 0
	for _, j := range rows.Jokes {
		if !j.PublishedAt.Equal(at(6)) {
			t.Errorf("joke published at %v; want %v", j.PublishedAt, at(6))
		}
		if j.FirstSoldAt != nil {
			sold++
		}
	}
	if sold != 1 {
		t.Fatalf("%d sold jokes; want 1", sold)
	}

	// The leaderboard's fees are the rows' shipping and fee × joke-minutes.
	stats, err := st.GetRoundStatsLean(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	lean := stats.Leaderboard[0].Lean
	if !approx(lean.ShippingCost, tr.ShippingCost) || !approx(tr.ShippingCost, 3) ||
		!approx(lean.StorageCost, round.StorageFeePerJokeMinute*tr.WIPJokeMinutes) ||
		!approx(lean.HostingCost, round.HostingFeePerJokeMinute*tr.InventoryJokeMinutes) {
		t.Fatalf("leaderboard %+v; rows give shipping %.4f, WIP %.4f, inventory %.4f",
			*lean, tr.ShippingCost, tr.WIPJokeMinutes, tr.InventoryJokeMinutes)
	}
}

// Review focus 2 and 3: an unsplit batch counts 5 jokes of WIP and ships as
// the standard 5, even when the round allows batches of 10, and a team with no
// batches still gets a (zero) row.
func TestMemStoreLeanChartRowsUnsplitBatchAndEmptyTeam(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	cfg := testutil.FiveFees(domain.DefaultRoundConfig())
	cfg.MarketMode = domain.MarketLean
	cfg.BatchSize = 10
	if _, err := st.InsertRoundConfig(ctx, 1, &cfg); err != nil {
		t.Fatal(err)
	}
	st.Teams = []domain.Team{{ID: 1, Name: "Team 1"}, {ID: 2, Name: "Team 2"}}
	if _, err := st.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBatch(ctx, 1, 1, nil, "one joke\n\nanother joke")
	if err != nil {
		t.Fatal(err)
	}
	st.Now = func() time.Time { return b.SubmittedAt.Add(2 * time.Minute) }

	rows, err := st.GetLeanChartRows(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Teams) != 2 || rows.Teams[0].TeamID != 1 || rows.Teams[1].TeamID != 2 {
		t.Fatalf("teams %+v; want teams 1 and 2 in id order", rows.Teams)
	}
	if got := rows.Teams[0]; got.BatchesSubmitted != 1 || math.Abs(got.WIPJokeMinutes-10) > 1e-9 || got.ShippingCost != 2 {
		t.Fatalf("team 1 %+v; want 1 submitted, 5 jokes × 2 min = 10 joke-min, standard shipping $2", got)
	}
	if got := rows.Teams[1]; got.BatchesSubmitted != 0 || got.WIPJokeMinutes != 0 || got.InventoryJokeMinutes != 0 {
		t.Fatalf("team 2 %+v; want zeros", got)
	}
	if len(rows.Batches) != 1 || rows.Batches[0].JokeRows != 0 || rows.Batches[0].Published != 0 ||
		rows.Batches[0].ProcessedAt != nil || rows.Batches[0].FirstSoldAt != nil {
		t.Fatalf("batches %+v; want one unsplit, unprocessed, unsold batch", rows.Batches)
	}
	stats, err := st.GetRoundStatsLean(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range stats.Leaderboard {
		if row.Team.ID == 1 && (math.Abs(row.Lean.StorageCost-0.05*10) > 1e-9 || row.Lean.CarryingCost != row.Lean.StorageCost) {
			t.Fatalf("leaderboard storage %.4f, carrying %.4f; want both 0.50", row.Lean.StorageCost, row.Lean.CarryingCost)
		}
	}
}

func TestMemStoreLeanChartRowsUnknownRound(t *testing.T) {
	_, err := testutil.NewStore().GetLeanChartRows(context.Background(), 99)
	if !domain.IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
}
