package usecase

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
)

var lcStart = time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)

// lcAt is the round start plus s seconds.
func lcAt(s float64) time.Time { return lcStart.Add(time.Duration(s * float64(time.Second))) }

func lcAtPtr(s float64) *time.Time {
	v := lcAt(s)
	return &v
}

func lcNear(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

// lcRound is a started LEAN round: window 900 s (one sample a minute), batch
// size 5, setup $3, carrying $0.02/joke-min, publishing $0.10, discard $0.01,
// WIP limit 2.
func lcRound() *domain.Round {
	start := lcStart
	return &domain.Round{
		ID: 7, MarketMode: domain.MarketLean, BatchSize: 5, MarketPrice: 1,
		CostOfPublishing: 0.10, CostOfDiscard: 0.01, SetupCostPerBatch: 3,
		CarryingCostPerJokeMinute: 0.02, ArrivalWindowSeconds: 900, WIPLimit: 2,
		StartedAt: &start,
	}
}

func TestLeanChartSampling(t *testing.T) {
	cases := []struct {
		window      int
		elapsed     float64
		step, count int
	}{
		{900, 900, 60, 16},   // t = 0..900, last sample exactly at the end
		{900, 901, 60, 17},   // first sample at or after the end is 960
		{900, 0, 60, 1},      // started this instant
		{900, -5, 60, 1},     // clock skew never gives zero samples
		{180, 180, 12, 16},   // the pilot window: one sample every 12 s
		{1, 0.5, 1, 2},       // step never below 1
		{900, 7140, 60, 120}, // exactly at the cap
		{900, 7141, 61, 119}, // past the cap: the step widens
	}
	for _, c := range cases {
		step, count := LeanChartSampling(c.window, c.elapsed)
		if step != c.step || count != c.count {
			t.Errorf("LeanChartSampling(%d, %v) = %d, %d; want %d, %d", c.window, c.elapsed, step, count, c.step, c.count)
		}
	}
	for elapsed := 0.0; elapsed < 50000; elapsed += 37.3 {
		step, count := LeanChartSampling(180, elapsed)
		if count > MaxLeanChartSamples || float64((count-1)*step) < elapsed {
			t.Fatalf("elapsed %.1f: step %d count %d does not reach the end within the cap", elapsed, step, count)
		}
	}
}

func TestBuildLeanChartsWasteMatchesTheLeaderboardFormula(t *testing.T) {
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(16 * 60),
		Teams: []ports.LeanChartTeamRow{{
			TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 1, DiscardedJokes: 2,
			WIPJokeMinutes: 30, InventoryJokeMinutes: 24,
		}},
		Batches: []ports.LeanChartBatchRow{{
			BatchID: 1, TeamID: 1, SubmittedAt: lcAt(0), ProcessedAt: lcAtPtr(360),
			JokeRows: 5, Published: 3, Sold: 1, FirstSoldAt: lcAtPtr(600),
		}},
		Jokes: []ports.LeanChartJokeRow{
			{TeamID: 1, PublishedAt: lcAt(360), FirstSoldAt: lcAtPtr(600)},
			{TeamID: 1, PublishedAt: lcAt(360)},
			{TeamID: 1, PublishedAt: lcAt(360)},
		},
	}
	got := BuildLeanCharts(lcRound(), rows).Teams[0].Waste
	want := LeanWaste{Setup: 3, Waiting: 0.60, Shelf: 0.48, NeverSold: 0.20, Discarded: 0.02, Total: 4.30}
	if !lcNear(got.Setup, want.Setup) || !lcNear(got.Waiting, want.Waiting) || !lcNear(got.Shelf, want.Shelf) ||
		!lcNear(got.NeverSold, want.NeverSold) || !lcNear(got.Discarded, want.Discarded) || !lcNear(got.Total, want.Total) {
		t.Fatalf("waste %+v; want %+v", got, want)
	}
	// The leaderboard's setup_cost + carrying_cost for the same rows.
	if !lcNear(got.Setup+got.Waiting+got.Shelf, 3+0.02*(30+24)) {
		t.Fatalf("setup+waiting+shelf %.4f; want the leaderboard's 4.08", got.Setup+got.Waiting+got.Shelf)
	}
}

func TestBuildLeanChartsIdleCountsAtSampleBoundaries(t *testing.T) {
	// A batch of 3 sent exactly at t=60 and released exactly at t=180; two of
	// its jokes published at 180, one of them first sold exactly at 300.
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(360),
		Teams:    []ports.LeanChartTeamRow{{TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 1, DiscardedJokes: 1}},
		Batches: []ports.LeanChartBatchRow{{
			BatchID: 1, TeamID: 1, SubmittedAt: lcAt(60), ProcessedAt: lcAtPtr(180),
			JokeRows: 3, Published: 2, Sold: 1, FirstSoldAt: lcAtPtr(300),
		}},
		Jokes: []ports.LeanChartJokeRow{
			{TeamID: 1, PublishedAt: lcAt(180), FirstSoldAt: lcAtPtr(300)},
			{TeamID: 1, PublishedAt: lcAt(180)},
		},
	}
	idle := BuildLeanCharts(lcRound(), rows).Teams[0].Idle
	want := []IdleSample{{0, 0, 0}, {60, 3, 0}, {120, 3, 0}, {180, 0, 2}, {240, 0, 2}, {300, 0, 1}, {360, 0, 1}}
	if len(idle) != len(want) {
		t.Fatalf("idle %+v; want %+v", idle, want)
	}
	for i := range want {
		if idle[i] != want[i] {
			t.Errorf("idle[%d] = %+v; want %+v (sent at t counts, released or sold at t does not)", i, idle[i], want[i])
		}
	}
}

// Review focus 2.
func TestBuildLeanChartsUnsplitBatchCountsBatchSize(t *testing.T) {
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(120),
		Teams:    []ports.LeanChartTeamRow{{TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 1, WIPJokeMinutes: 10}},
		Batches:  []ports.LeanChartBatchRow{{BatchID: 1, TeamID: 1, SubmittedAt: lcAt(0)}},
	}
	team := BuildLeanCharts(lcRound(), rows).Teams[0]
	if len(team.Idle) != 3 {
		t.Fatalf("idle %+v; want 3 samples", team.Idle)
	}
	for _, s := range team.Idle {
		if s.Waiting != 5 || s.Shelf != 0 {
			t.Fatalf("idle %+v; an unsplit batch waits as batch_size (5) jokes", team.Idle)
		}
	}
	b := team.Batches[0]
	if b.Jokes != 5 || b.Published != 0 || b.Sold != 0 || b.FirstSaleS != nil {
		t.Fatalf("batch %+v; want 5 jokes, nothing published, no sale", b)
	}
	if !lcNear(team.Waste.Waiting, 0.20) {
		t.Fatalf("waiting %.4f; want 0.02 × 10 = 0.20", team.Waste.Waiting)
	}
}

func TestBuildLeanChartsFirstSalePerBatch(t *testing.T) {
	// Given out of order on purpose: seq follows submission time.
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(300),
		Teams:    []ports.LeanChartTeamRow{{TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 2}},
		Batches: []ports.LeanChartBatchRow{
			{BatchID: 2, TeamID: 1, SubmittedAt: lcAt(200), ProcessedAt: lcAtPtr(230), JokeRows: 2, Published: 2},
			{BatchID: 1, TeamID: 1, SubmittedAt: lcAt(30), ProcessedAt: lcAtPtr(60), JokeRows: 4, Published: 4, Sold: 3, FirstSoldAt: lcAtPtr(125)},
		},
	}
	charts := BuildLeanCharts(lcRound(), rows)
	bs := charts.Teams[0].Batches
	if len(bs) != 2 {
		t.Fatalf("batches %+v", bs)
	}
	if bs[0].Seq != 1 || bs[0].SentS != 30 || bs[0].Jokes != 4 || bs[0].Sold != 3 || bs[0].FirstSaleS == nil || *bs[0].FirstSaleS != 95 {
		t.Fatalf("batch 1 %+v; want seq 1 sent at 30 s, first sale 95 s after sending", bs[0])
	}
	if bs[1].Seq != 2 || bs[1].SentS != 200 || bs[1].FirstSaleS != nil {
		t.Fatalf("batch 2 %+v; want seq 2, no sale yet", bs[1])
	}
	body, err := json.Marshal(charts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"first_sale_s":95`) || !strings.Contains(string(body), `"first_sale_s":null`) {
		t.Fatalf("json %s", body)
	}
}

// Review focus 3.
func TestBuildLeanChartsClassAverageIncludesTeamsWithNoBatches(t *testing.T) {
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(60),
		Teams: []ports.LeanChartTeamRow{
			{TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 1, WIPJokeMinutes: 1},
			{TeamID: 2, TeamName: "Team 2", BatchesSubmitted: 2, WIPJokeMinutes: 2},
			{TeamID: 3, TeamName: "Team 3"},
		},
		Batches: []ports.LeanChartBatchRow{
			{BatchID: 1, TeamID: 1, SubmittedAt: lcAt(0), JokeRows: 3},
			{BatchID: 2, TeamID: 2, SubmittedAt: lcAt(0)},
		},
	}
	charts := BuildLeanCharts(lcRound(), rows)
	if len(charts.Teams) != 3 {
		t.Fatalf("teams %+v", charts.Teams)
	}
	three := charts.Teams[2]
	if three.TeamID != 3 || len(three.Idle) != 2 || three.Idle[1] != (IdleSample{60, 0, 0}) || three.Batches == nil || len(three.Batches) != 0 || three.Waste.Total != 0 {
		t.Fatalf("team 3 %+v; want zero idle samples, no batches, no waste", three)
	}
	avg := charts.ClassAverage
	if !lcNear(avg.Waste.Setup, (3+6+0)/3.0) || !lcNear(avg.Waste.Waiting, (0.02+0.04+0)/3) || !lcNear(avg.Waste.Total, 3.02) {
		t.Fatalf("class average waste %+v; want setup 3, waiting 0.02, total 3.02 over all 3 teams", avg.Waste)
	}
	if len(avg.Idle) != 2 || !lcNear(avg.Idle[1].Waiting, (3+5+0)/3.0) || avg.Idle[1].T != 60 {
		t.Fatalf("class average idle %+v; want (3 + 5 + 0) / 3 waiting at t=60", avg.Idle)
	}
}

// Review focus 4: a batch with nothing published never divides by zero.
func TestBuildLeanChartsPooledSellThrough(t *testing.T) {
	// Minute 0: team 1 sends a batch, 4 published and 1 sold; team 2 one with 2
	// published and 2 sold → (1+2)/(4+2) = 0.5. Minute 1: only an unsplit batch
	// with nothing published → null. Minute 2: nothing sent → null.
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(120),
		Teams: []ports.LeanChartTeamRow{
			{TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 2},
			{TeamID: 2, TeamName: "Team 2", BatchesSubmitted: 1},
		},
		Batches: []ports.LeanChartBatchRow{
			{BatchID: 1, TeamID: 1, SubmittedAt: lcAt(10), ProcessedAt: lcAtPtr(20), JokeRows: 4, Published: 4, Sold: 1, FirstSoldAt: lcAtPtr(40)},
			{BatchID: 2, TeamID: 2, SubmittedAt: lcAt(50), ProcessedAt: lcAtPtr(55), JokeRows: 2, Published: 2, Sold: 2, FirstSoldAt: lcAtPtr(58)},
			{BatchID: 3, TeamID: 1, SubmittedAt: lcAt(70)},
		},
	}
	charts := BuildLeanCharts(lcRound(), rows)
	st := charts.ClassAverage.SellThrough
	if len(st) != 3 || st[0].T != 0 || st[1].T != 60 || st[2].T != 120 {
		t.Fatalf("sell-through %+v; want samples at 0, 60, 120", st)
	}
	if st[0].Value == nil || !lcNear(*st[0].Value, 0.5) || st[1].Value != nil || st[2].Value != nil {
		t.Fatalf("sell-through values %v %v %v; want 0.5, null, null", st[0].Value, st[1].Value, st[2].Value)
	}
	body, err := json.Marshal(charts)
	if err != nil {
		t.Fatalf("marshal must not fail (NaN would): %v", err)
	}
	if !strings.Contains(string(body), `"value":null`) {
		t.Fatalf("json %s", body)
	}
}

func TestBuildLeanChartsCapsSamplesAndStillReachesClockEnd(t *testing.T) {
	round := lcRound()
	round.ArrivalWindowSeconds = 180
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(10000),
		Teams:    []ports.LeanChartTeamRow{{TeamID: 1, TeamName: "Team 1"}},
	}
	charts := BuildLeanCharts(round, rows)
	idle := charts.Teams[0].Idle
	if len(idle) > MaxLeanChartSamples || len(charts.ClassAverage.Idle) != len(idle) || len(charts.ClassAverage.SellThrough) != len(idle) {
		t.Fatalf("%d team samples, %d class idle, %d sell-through; want equal and at most %d",
			len(idle), len(charts.ClassAverage.Idle), len(charts.ClassAverage.SellThrough), MaxLeanChartSamples)
	}
	if last := idle[len(idle)-1]; last.T < 10000 {
		t.Fatalf("last sample t=%d; must reach clock_end (10000 s)", last.T)
	}
	if charts.SampleSeconds != 85 {
		t.Fatalf("sample_seconds %d; want the widened step ceil(10000/119) = 85", charts.SampleSeconds)
	}
}

func TestBuildLeanChartsRoundNotStarted(t *testing.T) {
	round := lcRound()
	round.StartedAt = nil
	charts := BuildLeanCharts(round, &ports.LeanChartRows{
		ClockEnd: lcAt(0),
		Teams:    []ports.LeanChartTeamRow{{TeamID: 1, TeamName: "Team 1"}},
	})
	if charts.StartedAt != nil || charts.SampleSeconds != 60 {
		t.Fatalf("started_at %v sample_seconds %d; want nil and 60", charts.StartedAt, charts.SampleSeconds)
	}
	team := charts.Teams[0]
	if team.Idle == nil || len(team.Idle) != 0 || team.Batches == nil || len(team.Batches) != 0 ||
		charts.ClassAverage.Idle == nil || len(charts.ClassAverage.Idle) != 0 ||
		charts.ClassAverage.SellThrough == nil || len(charts.ClassAverage.SellThrough) != 0 {
		t.Fatalf("a round that has not started has no samples: %+v", charts)
	}
	body, err := json.Marshal(charts)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"started_at":null`, `"idle":[]`, `"batches":[]`, `"sell_through":[]`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("json lacks %s: %s", want, body)
		}
	}
	empty, err := json.Marshal(BuildLeanCharts(round, &ports.LeanChartRows{ClockEnd: lcAt(0)}))
	if err != nil || !strings.Contains(string(empty), `"teams":[]`) {
		t.Fatalf("no teams must marshal as []: %s %v", empty, err)
	}
}

// Review focus 1: ended_at is NULL, so the repository passes the database's
// now() as ClockEnd, and the samples must reach it.
func TestBuildLeanChartsRunningRoundReachesClockEnd(t *testing.T) {
	rows := &ports.LeanChartRows{
		ClockEnd: lcAt(150),
		Teams:    []ports.LeanChartTeamRow{{TeamID: 1, TeamName: "Team 1", BatchesSubmitted: 1}},
		Batches:  []ports.LeanChartBatchRow{{BatchID: 1, TeamID: 1, SubmittedAt: lcAt(100), JokeRows: 2}},
	}
	charts := BuildLeanCharts(lcRound(), rows)
	if !charts.ClockEnd.Equal(lcAt(150)) {
		t.Fatalf("clock_end %v; want %v", charts.ClockEnd, lcAt(150))
	}
	idle := charts.Teams[0].Idle
	if len(idle) != 4 || idle[3].T != 180 {
		t.Fatalf("idle %+v; want samples 0, 60, 120, 180 (the first at or after 150)", idle)
	}
	if idle[1].Waiting != 0 || idle[2].Waiting != 2 || idle[3].Waiting != 2 {
		t.Fatalf("idle %+v; the open batch waits from t=100 on", idle)
	}
}
