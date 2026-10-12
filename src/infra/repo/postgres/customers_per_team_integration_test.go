//go:build integration

// customers_per_team (migration 0006) and the tuned defaults against a real
// Postgres. Run with
//
//	JF_TEST_ALLOW_RESET=1 JF_TEST_DATABASE_URL=postgres://… go test -tags integration ./src/infra/repo/postgres/
//
// The database must already be migrated to 0006; the tests apply nothing. The
// round-trip test inserts its own round and deletes it afterwards. The Reset
// Game and start tests delete ALL game data, so they need JF_TEST_ALLOW_RESET=1
// and must only ever run against a throwaway database.
package postgres

import (
	"context"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

func TestCustomersPerTeamColumnRoundTripIntegration(t *testing.T) {
	ctx := context.Background()
	pool, repo := feesPool(t)
	var roundID int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(round_id), 0) + 1000 FROM rounds`).Scan(&roundID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM rounds WHERE round_id = $1`, roundID); err != nil {
			t.Errorf("cleanup round %d: %v", roundID, err)
		}
	})

	cfg := domain.DefaultRoundConfigFor(1)
	inserted, err := repo.InsertRoundConfig(ctx, roundID, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if inserted.CustomersPerTeam != 18 || domain.ConfigFromRound(inserted) != cfg {
		t.Fatalf("inserted %+v; want %+v", domain.ConfigFromRound(inserted), cfg)
	}
	cfg.CustomersPerTeam = 0
	if _, err := repo.UpdateRoundConfig(ctx, roundID, &cfg); err != nil {
		t.Fatal(err)
	}
	read, err := repo.GetRoundByID(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	if read.CustomersPerTeam != 0 {
		t.Fatalf("customers_per_team %d after update; want 0", read.CustomersPerTeam)
	}
	// The column rejects a negative value even if the API did not.
	if _, err := pool.Exec(ctx, `UPDATE rounds SET customers_per_team = -1 WHERE round_id = $1`, roundID); err == nil {
		t.Fatal("customers_per_team -1 was accepted; want the CHECK constraint to refuse it")
	}
}

func TestResetGameGivesTheTunedDefaultsIntegration(t *testing.T) {
	if os.Getenv("JF_TEST_ALLOW_RESET") != "1" {
		t.Skip("JF_TEST_ALLOW_RESET=1 not set: Reset Game would delete all game data")
	}
	ctx := context.Background()
	_, repo := feesPool(t)
	for _, n := range []int64{1, 2} {
		cfg := domain.DefaultRoundConfigFor(int(n))
		cfg.CustomersPerTeam, cfg.BuyThreshold, cfg.PreferenceBase, cfg.StorageFeePerJokeMinute = 3, 6, 2, 0.4
		if _, err := repo.InsertRoundConfig(ctx, n, &cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ResetGame(ctx); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{1, 2} {
		rd, err := repo.GetRoundByID(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		if rd.CustomersPerTeam != 18 || rd.BuyThreshold != 8 || rd.PreferenceBase != 5 ||
			rd.StorageFeePerJokeMinute != 0.05 || domain.ConfigFromRound(rd) != domain.DefaultRoundConfigFor(int(n)) {
			t.Errorf("round %d after reset: %+v; want the tuned defaults", n, domain.ConfigFromRound(rd))
		}
	}
}

// Starting a LEAN round with three staffed teams stores customer_count
// 18 × 3 = 54 in SQL and generates 54 ai_customers rows.
func TestStartRoundStoresCustomersPerTeamCountIntegration(t *testing.T) {
	if os.Getenv("JF_TEST_ALLOW_RESET") != "1" {
		t.Skip("JF_TEST_ALLOW_RESET=1 not set: the test resets the game")
	}
	ctx := context.Background()
	pool, repo := feesPool(t)
	cfg := domain.DefaultRoundConfigFor(1)
	if _, err := repo.InsertRoundConfig(ctx, 1, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResetGame(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.ResetGame(ctx); err != nil {
			t.Errorf("cleanup reset: %v", err)
		}
	})

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ai := usecase.NewAICustomerService(repo, rand.New(rand.NewSource(1)), log) //nolint:gosec
	instructor := usecase.NewInstructorService(repo, ai, log)
	session := usecase.NewSessionService(repo, log)
	for _, name := range []string{"cpt-a", "cpt-b", "cpt-c", "cpt-d", "cpt-e", "cpt-f"} {
		if _, err := session.Join(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := instructor.Assign(ctx, 1, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	round, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var stored, perTeam, customers int
	if err := pool.QueryRow(ctx, `
		SELECT r.customer_count, r.customers_per_team,
		       (SELECT COUNT(*) FROM ai_customers c WHERE c.round_id = r.round_id)
		FROM rounds r WHERE r.round_id = 1`).Scan(&stored, &perTeam, &customers); err != nil {
		t.Fatal(err)
	}
	if round.CustomerCount != 54 || stored != 54 || perTeam != 18 || customers != 54 {
		t.Fatalf("returned customer_count %d, stored %d, customers_per_team %d, ai_customers %d; want 54, 54, 18, 54",
			round.CustomerCount, stored, perTeam, customers)
	}
}
