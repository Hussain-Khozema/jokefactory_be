package usecase

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase/testutil"
)

// The headline promise, through the service: waste setup + waiting + shelf
// equals the leaderboard's setup + carrying at the same instant.
func TestStatsServiceLeanChartsAgreeWithTheLeaderboard(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	round, teamID := seedLeanStatsFixture(t, st)
	now := round.StartedAt.Add(16 * time.Minute)
	st.Now = func() time.Time { return now }
	svc := NewStatsService(st, slog.Default())
	approx := func(got, want float64) bool { return math.Abs(got-want) < 0.005 }

	charts, err := svc.GetLeanCharts(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := svc.GetRoundStats(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(charts.Teams) != 1 || charts.Teams[0].TeamID != teamID {
		t.Fatalf("teams %+v", charts.Teams)
	}
	w := charts.Teams[0].Waste
	lean := stats.Leaderboard[0].Lean
	if !approx(w.Setup+w.Waiting+w.Shelf, lean.SetupCost+lean.CarryingCost) {
		t.Fatalf("charts setup+waiting+shelf %.4f; leaderboard setup+carrying %.4f", w.Setup+w.Waiting+w.Shelf, lean.SetupCost+lean.CarryingCost)
	}
	// 2 published jokes never sold, 2 discarded.
	if !approx(w.NeverSold, 0.20) || !approx(w.Discarded, 0.02) {
		t.Fatalf("waste %+v", w)
	}
	// Default window 900 s → one sample a minute; 16 minutes → 17 samples.
	if charts.RoundID != round.ID || charts.StartedAt == nil || charts.SampleSeconds != 60 || len(charts.Teams[0].Idle) != 17 {
		t.Fatalf("round %d started %v step %d samples %d", charts.RoundID, charts.StartedAt, charts.SampleSeconds, len(charts.Teams[0].Idle))
	}
}

func TestStatsServiceLeanChartsRefuseAClassicRound(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	round, teamID := seedLeanStatsFixture(t, st)
	st.Rounds[round.ID].MarketMode = domain.MarketClassic
	svc := NewStatsService(st, slog.Default())
	for name, call := range map[string]func() error{
		"instructor": func() error { _, err := svc.GetLeanCharts(ctx, round.ID); return err },
		"team":       func() error { _, err := svc.GetTeamLeanCharts(ctx, round.ID, teamID); return err },
	} {
		err := call()
		if !domain.IsConflict(err) || !strings.Contains(err.Error(), notLeanRoundCode) {
			t.Fatalf("%s: want a %s conflict, got %v", name, notLeanRoundCode, err)
		}
	}
}

func TestStatsServiceLeanChartsUnknownRoundIsNotFound(t *testing.T) {
	_, err := NewStatsService(testutil.NewStore(), slog.Default()).GetLeanCharts(context.Background(), 99)
	if !domain.IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
}

// Review focus 3: the team route returns exactly one team — a zero row for a
// team with nothing (or no row) in the round — and the class average of all.
func TestStatsServiceTeamLeanChartsReturnOnlyThatTeam(t *testing.T) {
	ctx := context.Background()
	st := testutil.NewStore()
	round, teamID := seedLeanStatsFixture(t, st)
	// Team 2 plays the round but sends nothing; team 3 exists but has no row in it yet.
	st.Teams = append(st.Teams, domain.Team{ID: 2, Name: "Team 2"}, domain.Team{ID: 3, Name: "Team 3"})
	if err := st.EnsureTeamRoundState(ctx, round.ID, 2); err != nil {
		t.Fatal(err)
	}
	now := round.StartedAt.Add(16 * time.Minute)
	st.Now = func() time.Time { return now }
	svc := NewStatsService(st, slog.Default())

	full, err := svc.GetLeanCharts(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Teams) != 2 {
		t.Fatalf("instructor view teams %+v; want teams %d and 2", full.Teams, teamID)
	}
	two, err := svc.GetTeamLeanCharts(ctx, round.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(two.Teams) != 1 || two.Teams[0].TeamID != 2 || two.Teams[0].Waste.Total != 0 {
		t.Fatalf("team 2 view %+v; want only team 2, with no waste", two.Teams)
	}
	if math.Abs(two.ClassAverage.Waste.Total-full.ClassAverage.Waste.Total) > 1e-9 {
		t.Fatalf("class average %.4f; want the instructor view's %.4f", two.ClassAverage.Waste.Total, full.ClassAverage.Waste.Total)
	}
	three, err := svc.GetTeamLeanCharts(ctx, round.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(three.Teams) != 1 || three.Teams[0].TeamID != 3 || three.Teams[0].TeamName != "Team 3" ||
		len(three.Teams[0].Idle) != len(three.ClassAverage.Idle) || len(three.Teams[0].Batches) != 0 {
		t.Fatalf("team 3 view %+v; want a zero row with one idle sample per class sample", three.Teams)
	}
	for i, s := range three.Teams[0].Idle {
		if s.T != three.ClassAverage.Idle[i].T || s.Waiting != 0 || s.Shelf != 0 {
			t.Fatalf("team 3 idle %+v", three.Teams[0].Idle)
		}
	}
	if _, err := svc.GetTeamLeanCharts(ctx, round.ID, 99); !domain.IsNotFound(err) {
		t.Fatalf("unknown team: want not found, got %v", err)
	}
}
