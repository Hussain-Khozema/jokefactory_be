//go:build integration

// Lean charts SQL against a real Postgres. Run with
//
//	JF_TEST_DATABASE_URL=postgres://… go test -count=1 -tags integration ./src/infra/repo/postgres/
//
// The database must already be migrated; the test applies nothing. It inserts
// its own rounds and uniquely named teams and deletes them afterwards.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"jokefactory/src/app/http/handler"
	"jokefactory/src/core/usecase"
	"jokefactory/src/infra/db"
)

// Fixture (round ENDED 10:00–10:30, arrival window 900 s so one sample a
// minute, WIP limit 2, shipping $3 for every batch size, storage and hosting
// $0.02/joke-min, batch size 5, publishing $0.10, discard $0.01) — the lean
// stats fixture:
//
//	Team A: batch of 5 sent 10:00, released 10:06, 3 published / 2 discarded.
//	        Joke 1 bought 10:10 and returned 10:12; joke 2 bought 10:20 and
//	        10:25; joke 3 never sold. Leaderboard: setup 3, carrying 1.44.
//	        Waste: setup 3, waiting 0.60 (30 joke-min), shelf 0.84 (42),
//	        never sold 0.10 (joke 3), discarded 0.02; total 4.56.
//	Team B: unsplit batch sent 10:20, still waiting. Leaderboard: setup 3,
//	        carrying 1.00. Waste: setup 3, waiting 1.00 (5 × 10 min); total 4.00.
//	Team C: nothing. All zeros.
//
// Plus a running LEAN round (ended_at NULL) and an ENDED CLASSIC round.
func TestLeanChartsSQLIntegration(t *testing.T) {
	dsn := os.Getenv("JF_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JF_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Registered first so it runs last: the data cleanup below needs the pool.
	t.Cleanup(pool.Close)
	repo := New(&db.Postgres{Pool: pool}, slog.Default())

	base := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }
	suffix := fmt.Sprintf("lean-charts-it-%d", time.Now().UnixNano())
	approx := func(got, want float64) bool { return math.Abs(got-want) < 0.005 }

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	mustID := func(q string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, q, args...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
		return id
	}

	var rounds, teams []int64
	t.Cleanup(func() {
		// Deleting the rounds and teams cascades to every row the test made.
		for _, id := range rounds {
			if _, err := pool.Exec(ctx, `DELETE FROM rounds WHERE round_id = $1`, id); err != nil {
				t.Errorf("cleanup round %d: %v", id, err)
			}
		}
		for _, id := range teams {
			if _, err := pool.Exec(ctx, `DELETE FROM teams WHERE id = $1`, id); err != nil {
				t.Errorf("cleanup team %d: %v", id, err)
			}
		}
	})

	// Explicit ids above each table's maximum, so the test never depends on
	// (or advances) the id sequences.
	nextID := func(table, col string) int64 {
		t.Helper()
		return mustID(`SELECT COALESCE(MAX(` + col + `), 0) + 1000 FROM ` + table)
	}
	newRound := func(mode, status string, startedAt, endedAt any) int64 {
		t.Helper()
		id := mustID(`
			INSERT INTO rounds (round_id, round_number, status, batch_size, market_price, cost_of_publishing, cost_of_discard,
			                    market_mode, arrival_window_seconds, shipping_fee_small, shipping_fee_standard,
			                    shipping_fee_large, storage_fee_per_joke_minute, hosting_fee_per_joke_minute,
			                    wip_limit, started_at, ended_at)
			VALUES ($1, 999, $2::text::round_status, 5, 1, 0.10, 0.01, $3, 900, 3, 3, 3, 0.02, 0.02, 2, $4, $5)
			RETURNING round_id`, nextID("rounds", "round_id"), status, mode, startedAt, endedAt)
		rounds = append(rounds, id)
		return id
	}
	team := func(name string) int64 {
		t.Helper()
		id := mustID(`INSERT INTO teams (id, name) VALUES ($1, $2) RETURNING id`, nextID("teams", "id"), suffix+name)
		teams = append(teams, id)
		return id
	}
	submitEvent := func(roundID, teamID, batchID int64, jokes, delta int, created time.Time) {
		t.Helper()
		mustExec(`
			INSERT INTO batch_submission_events (event_id, round_id, team_id, batch_id, jokes_count, delta, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			nextID("batch_submission_events", "event_id"), roundID, teamID, batchID, jokes, delta, created)
	}

	// ---- The ENDED charts round -------------------------------------------
	chartsRound := newRound("LEAN", "ENDED", at(0), at(30))
	teamA, teamB, teamC := team("-A"), team("-B"), team("-C")
	mustExec(`
		INSERT INTO team_rounds_state (round_id, team_id, points_earned, batches_created, batches_processed, published_jokes, discarded_jokes)
		VALUES ($1, $2, 2, 1, 1, 3, 2), ($1, $3, 0, 1, 0, 0, 0), ($1, $4, 0, 0, 0, 0, 0)`,
		chartsRound, teamA, teamB, teamC)

	batchA := mustID(`
		INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at, processed_at)
		VALUES ($1, $2, $3, 'PROCESSED', $4, $5) RETURNING batch_id`,
		nextID("batches", "batch_id"), chartsRound, teamA, at(0), at(6))
	var jokes [5]int64
	for i := range jokes {
		if i < 3 {
			jokes[i] = mustID(`
				INSERT INTO jokes (joke_id, batch_id, joke_text, joke_title, publish_status, published_at)
				VALUES ($1, $2, 'j', 't', 'PUBLISHED', $3) RETURNING joke_id`, nextID("jokes", "joke_id"), batchA, at(6))
		} else {
			jokes[i] = mustID(`
				INSERT INTO jokes (joke_id, batch_id, joke_text, publish_status)
				VALUES ($1, $2, 'j', 'DISCARDED') RETURNING joke_id`, nextID("jokes", "joke_id"), batchA)
		}
	}
	submitEvent(chartsRound, teamA, batchA, 5, 1, at(0))
	submitEvent(chartsRound, teamA, batchA, 5, -1, at(6))

	batchB := mustID(`
		INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at, raw_text, raw_text_original)
		VALUES ($1, $2, $3, 'SUBMITTED', $4, 'a', 'a') RETURNING batch_id`,
		nextID("batches", "batch_id"), chartsRound, teamB, at(20))
	submitEvent(chartsRound, teamB, batchB, 0, 1, at(20))

	cust := func() int64 {
		return mustID(`
			INSERT INTO ai_customers (ai_customer_id, round_id, personal_threshold, starting_budget, remaining_budget)
			VALUES ($1, $2, 7, 3, 2) RETURNING ai_customer_id`, nextID("ai_customers", "ai_customer_id"), chartsRound)
	}
	c1, c2, c3 := cust(), cust(), cust()
	purchaseEvent := func(custID, jokeID int64, delta int, created time.Time) {
		t.Helper()
		mustExec(`
			INSERT INTO purchase_events (event_id, round_id, ai_customer_id, joke_id, team_id, delta, price, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, 1, $7)`,
			nextID("purchase_events", "event_id"), chartsRound, custID, jokeID, teamA, delta, created)
	}
	purchaseEvent(c1, jokes[0], 1, at(10))
	purchaseEvent(c1, jokes[0], -1, at(12))
	purchaseEvent(c2, jokes[1], 1, at(20))
	purchaseEvent(c3, jokes[1], 1, at(25))
	for _, h := range []struct {
		cust int64
		at   time.Time
	}{{c2, at(20)}, {c3, at(25)}} {
		mustExec(`
			INSERT INTO purchases (purchase_id, round_id, ai_customer_id, joke_id, team_id, price, created_at)
			VALUES ($1, $2, $3, $4, $5, 1, $6)`,
			nextID("purchases", "purchase_id"), chartsRound, h.cust, jokes[1], teamA, h.at)
	}

	round, err := repo.GetRoundByID(ctx, chartsRound)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.GetLeanChartRows(ctx, chartsRound)
	if err != nil {
		t.Fatal(err)
	}
	charts := usecase.BuildLeanCharts(round, rows)
	stats, err := repo.GetRoundStatsLean(ctx, chartsRound)
	if err != nil {
		t.Fatal(err)
	}
	if len(charts.Teams) != 3 || len(stats.Leaderboard) != 3 {
		t.Fatalf("charts %d teams, leaderboard %d rows; want 3 and 3", len(charts.Teams), len(stats.Leaderboard))
	}
	byTeam := map[int64]usecase.LeanChartsTeam{}
	for _, tm := range charts.Teams {
		byTeam[tm.TeamID] = tm
	}

	// The headline promise: setup + waiting + shelf == leaderboard setup + carrying.
	for _, row := range stats.Leaderboard {
		w := byTeam[row.Team.ID].Waste
		if !approx(w.Setup+w.Waiting+w.Shelf, row.Lean.SetupCost+row.Lean.CarryingCost) {
			t.Errorf("team %d: charts setup+waiting+shelf %.4f; leaderboard setup+carrying %.4f",
				row.Team.ID, w.Setup+w.Waiting+w.Shelf, row.Lean.SetupCost+row.Lean.CarryingCost)
		}
	}
	wantWaste := map[int64]usecase.LeanWaste{
		teamA: {Setup: 3, Waiting: 0.60, Shelf: 0.84, NeverSold: 0.10, Discarded: 0.02, Total: 4.56},
		teamB: {Setup: 3, Waiting: 1.00, Total: 4.00},
		teamC: {},
	}
	for id, want := range wantWaste {
		got := byTeam[id].Waste
		if !approx(got.Setup, want.Setup) || !approx(got.Waiting, want.Waiting) || !approx(got.Shelf, want.Shelf) ||
			!approx(got.NeverSold, want.NeverSold) || !approx(got.Discarded, want.Discarded) || !approx(got.Total, want.Total) {
			t.Errorf("team %d waste %+v; want %+v", id, got, want)
		}
	}

	a := byTeam[teamA].Batches
	if len(a) != 1 || a[0].Seq != 1 || a[0].SentS != 0 || a[0].Jokes != 5 || a[0].Published != 3 || a[0].Sold != 2 ||
		a[0].FirstSaleS == nil || !approx(*a[0].FirstSaleS, 600) {
		t.Errorf("team A batches %+v; want 5 jokes, 3 published, 2 sold, first sale after 600 s", a)
	}
	b := byTeam[teamB].Batches
	if len(b) != 1 || b[0].SentS != 1200 || b[0].Jokes != 5 || b[0].Published != 0 || b[0].FirstSaleS != nil {
		t.Errorf("team B batches %+v; want the unsplit batch counted as 5 jokes, no sale", b)
	}

	if charts.SampleSeconds != 60 || charts.WIPLimit != 2 || len(byTeam[teamA].Idle) != 31 {
		t.Fatalf("sample_seconds %d wip_limit %d samples %d; want 60, 2, 31", charts.SampleSeconds, charts.WIPLimit, len(byTeam[teamA].Idle))
	}
	idleA, idleB := byTeam[teamA].Idle, byTeam[teamB].Idle
	for _, c := range []struct {
		k              int
		waiting, shelf float64
	}{{0, 5, 0}, {6, 0, 3}, {10, 0, 2}, {20, 0, 1}, {30, 0, 1}} {
		if got := idleA[c.k]; got.T != c.k*60 || got.Waiting != c.waiting || got.Shelf != c.shelf {
			t.Errorf("team A idle[%d] = %+v; want waiting %v shelf %v", c.k, got, c.waiting, c.shelf)
		}
	}
	if idleB[19].Waiting != 0 || idleB[20].Waiting != 5 {
		t.Errorf("team B waiting at minutes 19, 20 = %v, %v; want 0, 5", idleB[19].Waiting, idleB[20].Waiting)
	}
	sell := charts.ClassAverage.SellThrough
	if sell[0].Value == nil || !approx(*sell[0].Value, 2.0/3) || sell[20].Value != nil {
		t.Errorf("class sell-through at minutes 0 and 20: %v, %v; want 2/3 and null", sell[0].Value, sell[20].Value)
	}
	if !approx(charts.ClassAverage.Waste.Total, (4.56+4.00)/3) {
		t.Errorf("class average total %.4f; want (4.56 + 4.00 + 0) / 3", charts.ClassAverage.Waste.Total)
	}

	// ---- Review focus 1: a running round (ended_at NULL, clock_end = now()) --
	// Status CONFIGURED keeps the single-ACTIVE-round index free; the charts
	// only read started_at and ended_at.
	running := newRound("LEAN", "CONFIGURED", nil, nil)
	mustExec(`UPDATE rounds SET started_at = now() - interval '10 minutes' WHERE round_id = $1`, running)
	mustExec(`
		INSERT INTO team_rounds_state (round_id, team_id, points_earned, batches_created, batches_processed, published_jokes, discarded_jokes)
		VALUES ($1, $2, 0, 1, 0, 0, 0)`, running, teamA)
	runBatch := mustID(`
		INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at)
		VALUES ($1, $2, $3, 'SUBMITTED', now() - interval '8 minutes') RETURNING batch_id`,
		nextID("batches", "batch_id"), running, teamA)
	for i := 0; i < 2; i++ {
		mustExec(`INSERT INTO jokes (joke_id, batch_id, joke_text, publish_status) VALUES ($1, $2, 'j', 'PENDING')`,
			nextID("jokes", "joke_id"), runBatch)
	}
	mustExec(`
		INSERT INTO batch_submission_events (event_id, round_id, team_id, batch_id, jokes_count, delta, created_at)
		VALUES ($1, $2, $3, $4, 2, 1, now() - interval '8 minutes')`,
		nextID("batch_submission_events", "event_id"), running, teamA, runBatch)

	runRound, err := repo.GetRoundByID(ctx, running)
	if err != nil {
		t.Fatal(err)
	}
	runRows, err := repo.GetLeanChartRows(ctx, running)
	if err != nil {
		t.Fatal(err)
	}
	runCharts := usecase.BuildLeanCharts(runRound, runRows)
	runStats, err := repo.GetRoundStatsLean(ctx, running)
	if err != nil {
		t.Fatal(err)
	}
	rw, rl := runCharts.Teams[0].Waste, runStats.Leaderboard[0].Lean
	// Two reads, two now()s milliseconds apart: they agree within a cent.
	if math.Abs(rw.Setup+rw.Waiting+rw.Shelf-(rl.SetupCost+rl.CarryingCost)) > 0.01 || math.Abs(rw.Waiting-0.02*2*8) > 0.01 {
		t.Errorf("running round: charts %+v vs leaderboard setup %.4f carrying %.4f; want waiting ≈ 0.32", rw, rl.SetupCost, rl.CarryingCost)
	}
	if d := time.Since(runCharts.ClockEnd); d < -5*time.Second || d > 5*time.Second {
		t.Errorf("running round clock_end %v is not now", runCharts.ClockEnd)
	}
	runIdle := runCharts.Teams[0].Idle
	last := runIdle[len(runIdle)-1]
	if elapsed := runCharts.ClockEnd.Sub(*runCharts.StartedAt).Seconds(); float64(last.T) < elapsed || last.Waiting != 2 {
		t.Errorf("running round last sample %+v; want t ≥ %.0f with the 2 waiting jokes", last, elapsed)
	}

	// ---- HTTP: Classic round → 409; team route → exactly one team ----------
	classic := newRound("CLASSIC", "ENDED", at(0), at(30))
	mustExec(`
		INSERT INTO team_rounds_state (round_id, team_id, points_earned, batches_created, batches_processed, published_jokes, discarded_jokes)
		VALUES ($1, $2, 0, 0, 0, 0, 0)`, classic, teamA)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := slog.Default()
	router.GET("/v1/instructor/rounds/:round_id/lean-charts",
		handler.NewInstructorHandler(usecase.NewInstructorService(repo, nil, log)).LeanCharts)
	router.GET("/v1/rounds/:round_id/teams/:team_id/lean-charts",
		handler.NewRoundHandler(usecase.NewRoundService(repo, log)).TeamLeanCharts)
	call := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}

	if code, body := call(fmt.Sprintf("/v1/instructor/rounds/%d/lean-charts", classic)); code != http.StatusConflict || !strings.Contains(body, "NOT_LEAN_ROUND") {
		t.Errorf("classic round: %d %s; want 409 NOT_LEAN_ROUND", code, body)
	}
	code, body := call(fmt.Sprintf("/v1/rounds/%d/teams/%d/lean-charts", chartsRound, teamB))
	if code != http.StatusOK {
		t.Fatalf("team route: %d %s", code, body)
	}
	var env struct {
		Data usecase.LeanCharts `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Teams) != 1 || env.Data.Teams[0].TeamID != teamB {
		t.Errorf("team route teams %+v; want exactly team B", env.Data.Teams)
	}
	if !approx(env.Data.ClassAverage.Waste.Total, (4.56+4.00)/3) {
		t.Errorf("team route class average %.4f; want every team's mean", env.Data.ClassAverage.Waste.Total)
	}
}
