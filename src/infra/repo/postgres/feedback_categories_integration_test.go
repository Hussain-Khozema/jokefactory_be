//go:build integration

// The feedback query returns each joke's categories with its fits. Run with
//
//	JF_TEST_DATABASE_URL=postgres://… go test -count=1 -tags integration ./src/infra/repo/postgres/ -run TestFeedbackCategoriesIntegration
//
// It inserts its own ENDED round, team and joke and deletes them afterwards.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"jokefactory/src/core/domain"
	"jokefactory/src/infra/db"
)

func TestFeedbackCategoriesIntegration(t *testing.T) {
	dsn := os.Getenv("JF_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JF_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repo := New(&db.Postgres{Pool: pool}, slog.Default())

	mustID := func(q string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, q, args...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
		return id
	}
	nextID := func(table, col string) int64 {
		t.Helper()
		return mustID(`SELECT COALESCE(MAX(` + col + `), 0) + 1000 FROM ` + table)
	}

	var roundID, teamID int64
	t.Cleanup(func() {
		if roundID != 0 {
			if _, err := pool.Exec(ctx, `DELETE FROM rounds WHERE round_id = $1`, roundID); err != nil {
				t.Errorf("cleanup round: %v", err)
			}
		}
		if teamID != 0 {
			if _, err := pool.Exec(ctx, `DELETE FROM teams WHERE id = $1`, teamID); err != nil {
				t.Errorf("cleanup team: %v", err)
			}
		}
	})

	roundID = mustID(`
		INSERT INTO rounds (round_id, round_number, status, batch_size, started_at, ended_at)
		VALUES ($1, 998, 'ENDED', 5, now() - interval '10 minutes', now())
		RETURNING round_id`, nextID("rounds", "round_id"))
	teamID = mustID(`INSERT INTO teams (id, name) VALUES ($1, $2) RETURNING id`,
		nextID("teams", "id"), fmt.Sprintf("feedback-cats-it-%d", time.Now().UnixNano()))
	batchID := mustID(`
		INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at)
		VALUES ($1, $2, $3, 'PROCESSED', now())
		RETURNING batch_id`, nextID("batches", "batch_id"), roundID, teamID)
	jokeID := mustID(`
		INSERT INTO jokes (joke_id, batch_id, joke_text, joke_title, publish_status, published_at)
		VALUES ($1, $2, 'a joke', 'Titled', 'PUBLISHED', now())
		RETURNING joke_id`, nextID("jokes", "joke_id"), batchID)
	for _, r := range []struct {
		dim, cat string
		fit      float64
	}{{"LENGTH", "Long", 0}, {"TOPIC", "Food", 1}} {
		if _, err := pool.Exec(ctx, `INSERT INTO joke_dim_fit (joke_id, dimension, dim_fit) VALUES ($1, $2, $3)`, jokeID, r.dim, r.fit); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO joke_dimension_values (joke_id, dimension, category) VALUES ($1, $2, $3)`, jokeID, r.dim, r.cat); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := repo.ListTeamFeedbackJokes(ctx, roundID, teamID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].JokeID != jokeID {
		t.Fatalf("rows = %+v; want the one joke %d", rows, jokeID)
	}
	r := rows[0]
	if r.DimFits[domain.DimLength] != 0 || r.DimFits[domain.DimTopic] != 1 {
		t.Fatalf("fits = %v", r.DimFits)
	}
	if r.Categories[domain.DimLength] != "Long" || r.Categories[domain.DimTopic] != "Food" || len(r.Categories) != 2 {
		t.Fatalf("categories = %v; want LENGTH=Long, TOPIC=Food", r.Categories)
	}
}
