//go:build integration

// Lean fee columns against a real Postgres. Run with
//
//	JF_TEST_DATABASE_URL=postgres://… go test -tags integration ./src/infra/repo/postgres/
//
// The database must already be migrated to 0005; the tests apply nothing.
// The round-trip test inserts its own round and deletes it afterwards. The
// Reset Game test deletes ALL game data, so it also needs JF_TEST_ALLOW_RESET=1
// and must only ever run against a throwaway database.
package postgres

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"jokefactory/src/core/domain"
	"jokefactory/src/infra/db"
)

func feesPool(t *testing.T) (*pgxpool.Pool, *Repositories) {
	t.Helper()
	dsn := os.Getenv("JF_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JF_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Registered first so it runs last: the data cleanup needs the pool.
	t.Cleanup(pool.Close)
	return pool, New(&db.Postgres{Pool: pool}, slog.Default())
}

// The five fee columns round-trip through InsertRoundConfig, UpdateRoundConfig
// and every round read, at their stored scale.
func TestLeanFeeColumnsRoundTripIntegration(t *testing.T) {
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

	cfg := domain.DefaultRoundConfigFor(2)
	inserted, err := repo.InsertRoundConfig(ctx, roundID, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := domain.ConfigFromRound(inserted); got != cfg {
		t.Fatalf("inserted %+v; want %+v", got, cfg)
	}

	cfg.ShippingFeeSmall, cfg.ShippingFeeStandard, cfg.ShippingFeeLarge = 0.5, 1.25, 4
	cfg.StorageFeePerJokeMinute, cfg.HostingFeePerJokeMinute = 0.125, 0.0375
	if _, err := repo.UpdateRoundConfig(ctx, roundID, &cfg); err != nil {
		t.Fatal(err)
	}
	read, err := repo.GetRoundByID(ctx, roundID)
	if err != nil {
		t.Fatal(err)
	}
	if read.ShippingFeeSmall != 0.5 || read.ShippingFeeStandard != 1.25 || read.ShippingFeeLarge != 4 ||
		read.StorageFeePerJokeMinute != 0.125 || read.HostingFeePerJokeMinute != 0.0375 || read.BatchSize != 10 {
		t.Fatalf("read back %+v", read)
	}
}

// Reset Game truncates every game table, so this test runs only when
// JF_TEST_ALLOW_RESET=1 says the database is a throwaway one.
func TestResetGameRestoresTheLeanRoundsIntegration(t *testing.T) {
	if os.Getenv("JF_TEST_ALLOW_RESET") != "1" {
		t.Skip("JF_TEST_ALLOW_RESET=1 not set: Reset Game would delete all game data")
	}
	ctx := context.Background()
	_, repo := feesPool(t)
	round1, round2 := domain.DefaultRoundConfigFor(1), domain.DefaultRoundConfigFor(2)
	round1.BatchSize = 7
	round2.StorageFeePerJokeMinute = 0.9
	round2.ShippingFeeLarge = 9
	if _, err := repo.InsertRoundConfig(ctx, 1, &round1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertRoundConfig(ctx, 2, &round2); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResetGame(ctx); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{1, 2} {
		rd, err := repo.GetRoundByID(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := domain.ConfigFromRound(rd), domain.DefaultRoundConfigFor(int(n)); got != want || rd.Status != domain.RoundConfigured {
			t.Errorf("round %d after reset: %+v (%s); want %+v, CONFIGURED", n, got, rd.Status, want)
		}
	}
}
