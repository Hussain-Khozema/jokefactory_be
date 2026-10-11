//go:build integration

// Lean fees in the stats and lean charts SQL against a real Postgres. Run with
//
//	JF_TEST_DATABASE_URL=postgres://… go test -tags integration ./src/infra/repo/postgres/
//
// The database must already be migrated to 0005; the test applies nothing. It
// inserts its own round and uniquely named teams and deletes them afterwards.
// feesPool is in lean_fees_integration_test.go.
package postgres

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"jokefactory/src/core/usecase"
)

// Fixture (round ENDED 10:00–10:30, batch_size 10, the default fees: $1 / $2
// / $3 shipping, storage $0.10 and hosting $0.05 per joke-minute; price $1,
// listing $0.10, discard $0.01) — the memstore fixture of
// src/core/usecase/lean_fees_stats_test.go, one minute longer:
//
//	Team A: batch of 5 sent 10:00, released 10:02 (WIP 10): 1 published at
//	        10:02 and sold at 10:06 (hosting 4 min), 4 discarded. Batches of 3
//	        and 8 sent and released at 10:03 / 10:04, all discarded. An
//	        unsplit batch sent 10:29 and never split (WIP 5 × 1 = 5).
//	        shipping 2+1+3+2 = 8.00; storage 0.10 × 15 = 1.50;
//	        hosting 0.05 × 4 = 0.20; profit 1 − 0.10 − 0.15 − 9.70 = −8.95
//	Team B: batches of exactly 4, 5 and 6, released when sent, all discarded:
//	        shipping 1+2+3 = 6.00; profit −0.15 − 6 = −6.15
func TestLeanFeesSQLIntegration(t *testing.T) {
	ctx := context.Background()
	pool, repo := feesPool(t)
	base := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }
	suffix := fmt.Sprintf("lean-fees-it-%d", time.Now().UnixNano())

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
	// Explicit ids above each table's maximum, so the test never depends on
	// (or advances) the id sequences.
	nextID := func(table, col string) int64 {
		t.Helper()
		return mustID(`SELECT COALESCE(MAX(` + col + `), 0) + 1000 FROM ` + table)
	}

	roundID = mustID(`
		INSERT INTO rounds (round_id, round_number, status, batch_size, market_price, cost_of_publishing, cost_of_discard,
		                    market_mode, shipping_fee_small, shipping_fee_standard, shipping_fee_large,
		                    storage_fee_per_joke_minute, hosting_fee_per_joke_minute, started_at, ended_at)
		VALUES ($1, 998, 'ENDED', 10, 1, 0.10, 0.01, 'LEAN', 1, 2, 3, 0.10, 0.05, $2, $3)
		RETURNING round_id`, nextID("rounds", "round_id"), at(0), at(30))
	team := func(name string) int64 {
		id := mustID(`INSERT INTO teams (id, name) VALUES ($1, $2) RETURNING id`, nextID("teams", "id"), suffix+name)
		teams = append(teams, id)
		return id
	}
	teamA, teamB := team("-A"), team("-B")
	mustExec(`
		INSERT INTO team_rounds_state (round_id, team_id, points_earned, batches_created, batches_processed, published_jokes, discarded_jokes)
		VALUES ($1, $2, 1, 4, 3, 1, 15), ($1, $3, 0, 3, 3, 0, 15)`, roundID, teamA, teamB)

	// batch inserts a sent batch with `jokes` joke rows, the first `published`
	// of them PUBLISHED at its processed time, the rest DISCARDED. jokes 0 is an
	// unsplit batch (raw text, no joke rows). processed nil leaves it SUBMITTED.
	batch := func(teamID int64, sent time.Time, processed *time.Time, jokes, published int) []int64 {
		t.Helper()
		status, raw := "PROCESSED", any(nil)
		if processed == nil {
			status = "SUBMITTED"
		}
		if jokes == 0 {
			raw = "an unsplit blob of several jokes"
		}
		id := mustID(`
			INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at, processed_at, raw_text, raw_text_original)
			VALUES ($1, $2, $3, $4::text::batch_status, $5, $6, $7, $7) RETURNING batch_id`,
			nextID("batches", "batch_id"), roundID, teamID, status, sent, processed, raw)
		mustExec(`
			INSERT INTO batch_submission_events (event_id, round_id, team_id, batch_id, jokes_count, delta, created_at)
			VALUES ($1, $2, $3, $4, $5, 1, $6)`,
			nextID("batch_submission_events", "event_id"), roundID, teamID, id, jokes, sent)
		var ids []int64
		for i := 0; i < jokes; i++ {
			if i < published {
				ids = append(ids, mustID(`
					INSERT INTO jokes (joke_id, batch_id, joke_text, joke_title, publish_status, published_at)
					VALUES ($1, $2, 'j', 't', 'PUBLISHED', $3) RETURNING joke_id`, nextID("jokes", "joke_id"), id, *processed))
			} else {
				ids = append(ids, mustID(`
					INSERT INTO jokes (joke_id, batch_id, joke_text, publish_status)
					VALUES ($1, $2, 'j', 'DISCARDED') RETURNING joke_id`, nextID("jokes", "joke_id"), id))
			}
		}
		return ids
	}
	ptr := func(v time.Time) *time.Time { return &v }
	soldJoke := batch(teamA, at(0), ptr(at(2)), 5, 1)[0]
	batch(teamA, at(3), ptr(at(3)), 3, 0)
	batch(teamA, at(4), ptr(at(4)), 8, 0)
	batch(teamA, at(29), nil, 0, 0)
	batch(teamB, at(1), ptr(at(1)), 4, 0)
	batch(teamB, at(2), ptr(at(2)), 5, 0)
	batch(teamB, at(3), ptr(at(3)), 6, 0)

	cust := mustID(`
		INSERT INTO ai_customers (ai_customer_id, round_id, personal_threshold, starting_budget, remaining_budget)
		VALUES ($1, $2, 7, 3, 2) RETURNING ai_customer_id`, nextID("ai_customers", "ai_customer_id"), roundID)
	mustExec(`
		INSERT INTO purchase_events (event_id, round_id, ai_customer_id, joke_id, team_id, delta, price, created_at)
		VALUES ($1, $2, $3, $4, $5, 1, 1, $6)`,
		nextID("purchase_events", "event_id"), roundID, cust, soldJoke, teamA, at(6))
	mustExec(`
		INSERT INTO purchases (purchase_id, round_id, ai_customer_id, joke_id, team_id, price, created_at)
		VALUES ($1, $2, $3, $4, $5, 1, $6)`,
		nextID("purchases", "purchase_id"), roundID, cust, soldJoke, teamA, at(6))

	near := func(got, want float64) bool { return math.Abs(got-want) < 0.005 }
	want := map[int64]struct{ shipping, storage, hosting, profit float64 }{
		teamA: {8, 1.50, 0.20, -8.95},
		teamB: {6, 0, 0, -6.15},
	}

	stats, err := repo.GetRoundStatsLean(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Leaderboard) != 2 {
		t.Fatalf("leaderboard has %d rows", len(stats.Leaderboard))
	}
	for _, row := range stats.Leaderboard {
		w, l := want[row.Team.ID], row.Lean
		if !near(l.ShippingCost, w.shipping) || !near(l.StorageCost, w.storage) || !near(l.HostingCost, w.hosting) ||
			!near(row.Profit, w.profit) {
			t.Errorf("leaderboard team %d: shipping %.4f storage %.4f hosting %.4f profit %.4f; want %+v",
				row.Team.ID, l.ShippingCost, l.StorageCost, l.HostingCost, row.Profit, w)
		}
		if !near(l.SetupCost, w.shipping) || !near(l.CarryingCost, w.storage+w.hosting) {
			t.Errorf("leaderboard team %d: aliases setup %.4f carrying %.4f; want %.2f and %.2f",
				row.Team.ID, l.SetupCost, l.CarryingCost, w.shipping, w.storage+w.hosting)
		}
		sum, err := repo.GetTeamSummaryLean(ctx, roundID, row.Team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !near(sum.Lean.ShippingCost, w.shipping) || !near(sum.Lean.StorageCost, w.storage) ||
			!near(sum.Lean.HostingCost, w.hosting) || !near(sum.Lean.SetupCost, w.shipping) ||
			!near(sum.Lean.CarryingCost, w.storage+w.hosting) || !near(sum.Profit, w.profit) {
			t.Errorf("summary team %d: %+v profit %.4f; want %+v", row.Team.ID, *sum.Lean, sum.Profit, w)
		}
	}

	round, err := repo.GetRoundByID(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.GetLeanChartRows(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	charts := usecase.BuildLeanCharts(round, rows)
	for _, tm := range charts.Teams {
		w := want[tm.TeamID]
		if !near(tm.Waste.Setup, w.shipping) || !near(tm.Waste.Waiting, w.storage) || !near(tm.Waste.Shelf, w.hosting) {
			t.Errorf("charts team %d waste %+v; want setup %.2f, waiting %.2f, shelf %.2f",
				tm.TeamID, tm.Waste, w.shipping, w.storage, w.hosting)
		}
		if tm.TeamID == teamA {
			if last := tm.Batches[len(tm.Batches)-1]; last.Jokes != 5 {
				t.Errorf("unsplit batch shows %d jokes; want 5, not batch_size 10", last.Jokes)
			}
		}
	}
}
