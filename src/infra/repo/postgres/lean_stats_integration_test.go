//go:build integration

// Lean stats SQL against a real Postgres. Run with
//
//	JF_TEST_DATABASE_URL=postgres://… go test -tags integration ./src/infra/repo/postgres/
//
// The database must already be migrated; the test applies nothing. It inserts
// its own round and uniquely named teams and deletes them afterwards.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"jokefactory/src/core/ports"
	"jokefactory/src/infra/db"
)

// Fixture (round ENDED, 10:00–10:30, shipping $3 for every batch size,
// storage and hosting $0.02/joke-min — so setup_cost is $3 a batch and
// carrying_cost is $0.02 a joke-minute, as before the fees were split — batch
// size 5, price $1, publishing $0.10, discard $0.01):
//
//	Team A: batch submitted 10:00, processed 10:06, 3 published / 2 discarded.
//	        Joke 1 bought 10:10, returned 10:12, never re-bought; joke 2 bought
//	        10:20 and again 10:25 (two customers); joke 3 never sold.
//	        WIP 5×6 = 30; inventory 4 + 14 + 24 = 42; carrying 0.02×72 = 1.44
//	        profit = 2×1 − 0.30 − 0.02 − 3 − 1.44 = −2.76
//	        avg submit→publish 360 s; avg publish→first sale (240+840)/2 = 540 s
//	Team B: unsplit pending batch submitted 10:20 (+ a DRAFT batch, not
//	        submitted). WIP 5×10 = 50; carrying 1.00; profit −4.00
//	Team C: no batches; all zeros, profit 0.
//	Ranks: C=1, A=2, B=3.
func TestLeanStatsSQLIntegration(t *testing.T) {
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
	suffix := fmt.Sprintf("lean-it-%d", time.Now().UnixNano())

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

	var roundID int64
	var teams []int64
	t.Cleanup(func() {
		// Deleting the round and teams cascades to every row the test made.
		if roundID != 0 {
			if _, err := pool.Exec(ctx, `DELETE FROM rounds WHERE round_id = $1`, roundID); err != nil {
				t.Errorf("cleanup round %d: %v", roundID, err)
			}
		}
		for _, id := range teams {
			if _, err := pool.Exec(ctx, `DELETE FROM teams WHERE id = $1`, id); err != nil {
				t.Errorf("cleanup team %d: %v", id, err)
			}
		}
	})

	// Every row gets an explicit id above the table's current maximum, so the
	// test never depends on (or advances) the id sequences: after a reset that
	// re-seeds explicit ids, a sequence can lag behind its table.
	nextID := func(table, col string) int64 {
		t.Helper()
		return mustID(`SELECT COALESCE(MAX(` + col + `), 0) + 1000 FROM ` + table)
	}

	roundID = mustID(`
		INSERT INTO rounds (round_id, round_number, status, batch_size, market_price, cost_of_publishing, cost_of_discard,
		                    market_mode, shipping_fee_small, shipping_fee_standard, shipping_fee_large,
		                    storage_fee_per_joke_minute, hosting_fee_per_joke_minute, started_at, ended_at)
		VALUES ($1, 999, 'ENDED', 5, 1, 0.10, 0.01, 'LEAN', 3, 3, 3, 0.02, 0.02, $2, $3)
		RETURNING round_id`, nextID("rounds", "round_id"), at(0), at(30))
	team := func(name string) int64 {
		id := mustID(`INSERT INTO teams (id, name) VALUES ($1, $2) RETURNING id`, nextID("teams", "id"), suffix+name)
		teams = append(teams, id)
		return id
	}
	teamA, teamB, teamC := team("-A"), team("-B"), team("-C")

	mustExec(`
		INSERT INTO team_rounds_state (round_id, team_id, points_earned, batches_created, batches_processed, published_jokes, discarded_jokes)
		VALUES ($1, $2, 2, 1, 1, 3, 2), ($1, $3, 0, 1, 0, 0, 0), ($1, $4, 0, 0, 0, 0, 0)`,
		roundID, teamA, teamB, teamC)

	submitEvent := func(teamID, batchID int64, jokes, delta int, created time.Time) {
		t.Helper()
		mustExec(`
			INSERT INTO batch_submission_events (event_id, round_id, team_id, batch_id, jokes_count, delta, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			nextID("batch_submission_events", "event_id"), roundID, teamID, batchID, jokes, delta, created)
	}

	// Team A: one processed batch of 5.
	batchA := mustID(`
		INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at, processed_at)
		VALUES ($1, $2, $3, 'PROCESSED', $4, $5) RETURNING batch_id`,
		nextID("batches", "batch_id"), roundID, teamA, at(0), at(6))
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
	submitEvent(teamA, batchA, 5, 1, at(0))
	submitEvent(teamA, batchA, 5, -1, at(6))

	// Team B: an unsplit pending batch, plus a DRAFT batch that was never submitted.
	batchB := mustID(`
		INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at, raw_text, raw_text_original)
		VALUES ($1, $2, $3, 'SUBMITTED', $4, 'a', 'a') RETURNING batch_id`,
		nextID("batches", "batch_id"), roundID, teamB, at(20))
	mustExec(`INSERT INTO batches (batch_id, round_id, team_id, status) VALUES ($1, $2, $3, 'DRAFT')`,
		nextID("batches", "batch_id"), roundID, teamB)
	submitEvent(teamB, batchB, 0, 1, at(20))

	// Purchases: joke 1 bought then returned; joke 2 bought by two customers.
	cust := func() int64 {
		return mustID(`
			INSERT INTO ai_customers (ai_customer_id, round_id, personal_threshold, starting_budget, remaining_budget)
			VALUES ($1, $2, 7, 3, 2) RETURNING ai_customer_id`, nextID("ai_customers", "ai_customer_id"), roundID)
	}
	c1, c2, c3 := cust(), cust(), cust()
	purchaseEvent := func(custID, jokeID int64, delta int, created time.Time) {
		t.Helper()
		mustExec(`
			INSERT INTO purchase_events (event_id, round_id, ai_customer_id, joke_id, team_id, delta, price, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, 1, $7)`,
			nextID("purchase_events", "event_id"), roundID, custID, jokeID, teamA, delta, created)
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
			nextID("purchases", "purchase_id"), roundID, h.cust, jokes[1], teamA, h.at)
	}

	approx := func(got, want float64) bool { return math.Abs(got-want) < 0.005 }
	type want struct {
		rank                               int
		setup, carrying, profit, s2p, p2fs float64
		submitted                          int
		series                             []ports.BatchPoint
	}
	wants := map[int64]want{
		teamA: {rank: 2, setup: 3, carrying: 1.44, profit: -2.76, s2p: 360, p2fs: 540, submitted: 1,
			series: []ports.BatchPoint{{Seq: 1, SubmittedAt: at(0), Jokes: 5, Published: 3, Sold: 2}}},
		teamB: {rank: 3, setup: 3, carrying: 1.00, profit: -4, submitted: 1,
			series: []ports.BatchPoint{{Seq: 1, SubmittedAt: at(20)}}},
		teamC: {rank: 1, series: []ports.BatchPoint{}},
	}
	check := func(label string, teamID int64, rank int, profit float64, l *ports.LeanTeamStats) {
		t.Helper()
		w := wants[teamID]
		if l == nil {
			t.Fatalf("%s team %d: no lean block", label, teamID)
		}
		if rank != w.rank || !approx(profit, w.profit) || l.BatchesSubmitted != w.submitted ||
			!approx(l.SetupCost, w.setup) || !approx(l.CarryingCost, w.carrying) ||
			!approx(l.AvgSubmitToPublishS, w.s2p) || !approx(l.AvgPublishToFirstSaleS, w.p2fs) {
			t.Errorf("%s team %d: rank=%d profit=%.4f lean=%+v; want %+v", label, teamID, rank, profit, *l, w)
		}
		if len(l.BatchSeries) != len(w.series) {
			t.Fatalf("%s team %d: series %+v; want %+v", label, teamID, l.BatchSeries, w.series)
		}
		for i, p := range l.BatchSeries {
			ws := w.series[i]
			if p.Seq != ws.Seq || !p.SubmittedAt.Equal(ws.SubmittedAt) || p.SubmittedAt.Location() != time.UTC ||
				p.Jokes != ws.Jokes || p.Published != ws.Published || p.Sold != ws.Sold {
				t.Errorf("%s team %d: series[%d]=%+v; want %+v", label, teamID, i, p, ws)
			}
		}
	}

	stats, err := repo.GetRoundStatsLean(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Leaderboard) != 3 {
		t.Fatalf("leaderboard has %d rows", len(stats.Leaderboard))
	}
	if order := []int64{stats.Leaderboard[0].Team.ID, stats.Leaderboard[1].Team.ID, stats.Leaderboard[2].Team.ID}; order[0] != teamC || order[1] != teamA || order[2] != teamB {
		t.Errorf("leaderboard order %v; want C, A, B", order)
	}
	for _, row := range stats.Leaderboard {
		check("leaderboard", row.Team.ID, row.Rank, row.Profit, row.Lean)
	}
	for _, id := range teams {
		sum, err := repo.GetTeamSummaryLean(ctx, roundID, id)
		if err != nil {
			t.Fatal(err)
		}
		check("summary", id, sum.Rank, sum.Profit, sum.Lean)
	}
	if sum, _ := repo.GetTeamSummaryLean(ctx, roundID, teamB); sum.UnprocessedBatches != 1 {
		t.Errorf("team B unprocessed_batches=%d; want 1", sum.UnprocessedBatches)
	}

	// The Classic queries on the same data carry no lean block.
	classic, err := repo.GetRoundStatsV2(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range classic.Leaderboard {
		if row.Lean != nil {
			t.Errorf("classic leaderboard team %d has a lean block", row.Team.ID)
		}
	}
	csum, err := repo.GetTeamSummary(ctx, roundID, teamA)
	if err != nil {
		t.Fatal(err)
	}
	if csum.Lean != nil || !approx(csum.Profit, 2-0.30-0.02) {
		t.Errorf("classic summary: lean=%v profit=%.4f", csum.Lean, csum.Profit)
	}
}
