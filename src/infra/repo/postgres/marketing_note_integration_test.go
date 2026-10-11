//go:build integration

// Marketing's note against a real Postgres. Run with
//
//	JF_TEST_DATABASE_URL=postgres://… go test -count=1 -tags integration ./src/infra/repo/postgres/ -run TestMarketingNoteIntegration
//
// The database must already be migrated through 0004_batch_marketing_note; the
// test applies nothing. It inserts its own ENDED round (so it never collides
// with a live ACTIVE round), a uniquely named team and a Marketing user, and
// deletes them afterwards.
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
	"jokefactory/src/core/ports"
	"jokefactory/src/infra/db"
)

func TestMarketingNoteIntegration(t *testing.T) {
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

	// The migration: a nullable TEXT column on batches.
	var dataType, nullable string
	if err := pool.QueryRow(ctx, `
		SELECT data_type, is_nullable FROM information_schema.columns
		WHERE table_name = 'batches' AND column_name = 'marketing_note'`).Scan(&dataType, &nullable); err != nil {
		t.Fatalf("batches.marketing_note missing — run migration 0004 first: %v", err)
	}
	if dataType != "text" || nullable != "YES" {
		t.Fatalf("batches.marketing_note is %s nullable=%s; want text nullable=YES", dataType, nullable)
	}

	mustID := func(q string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, q, args...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
		return id
	}
	// Explicit ids above each table's maximum, as the lean integration tests do.
	nextID := func(table, col string) int64 {
		t.Helper()
		return mustID(`SELECT COALESCE(MAX(` + col + `), 0) + 1000 FROM ` + table)
	}

	var roundID, teamID, userID int64
	t.Cleanup(func() {
		// Rounds cascade to batches, jokes, events and jobs. The user goes before
		// the team: deleting the team would null a MARKETING user's team_id,
		// which users_team_id_role_chk forbids.
		for _, q := range []struct {
			sql string
			id  int64
		}{
			{`DELETE FROM rounds WHERE round_id = $1`, roundID},
			{`DELETE FROM users WHERE user_id = $1`, userID},
			{`DELETE FROM teams WHERE id = $1`, teamID},
		} {
			if q.id == 0 {
				continue
			}
			if _, err := pool.Exec(ctx, q.sql, q.id); err != nil {
				t.Errorf("cleanup %q %d: %v", q.sql, q.id, err)
			}
		}
	})

	roundID = mustID(`
		INSERT INTO rounds (round_id, round_number, status, batch_size, started_at, ended_at)
		VALUES ($1, 999, 'ENDED', 5, now() - interval '10 minutes', now())
		RETURNING round_id`, nextID("rounds", "round_id"))
	teamID = mustID(`INSERT INTO teams (id, name) VALUES ($1, $2) RETURNING id`,
		nextID("teams", "id"), fmt.Sprintf("marketing-note-it-%d", time.Now().UnixNano()))
	userID = mustID(`
		INSERT INTO users (user_id, display_name, role, team_id, status)
		VALUES ($1, 'note-it-marketer', 'MARKETING', $2, 'ASSIGNED')
		RETURNING user_id`, nextID("users", "user_id"), teamID)
	if _, err := pool.Exec(ctx, `INSERT INTO team_rounds_state (round_id, team_id) VALUES ($1, $2)`, roundID, teamID); err != nil {
		t.Fatal(err)
	}

	// newBatch inserts a batch with two PENDING jokes. A claimed batch is
	// SUBMITTED and locked by the marketer; a legacy one is PROCESSED with no
	// note, the way every batch published before 0004 looks.
	newBatch := func(claimed bool) (int64, []int64) {
		t.Helper()
		status, lockedBy := "PROCESSED", any(nil)
		if claimed {
			status, lockedBy = "SUBMITTED", userID
		}
		batchID := mustID(`
			INSERT INTO batches (batch_id, round_id, team_id, status, submitted_at, locked_at, locked_by)
			VALUES ($1, $2, $3, $4::text::batch_status, now(), now(), $5)
			RETURNING batch_id`, nextID("batches", "batch_id"), roundID, teamID, status, lockedBy)
		var jokes []int64
		for _, text := range []string{"first joke", "second joke"} {
			jokes = append(jokes, mustID(`
				INSERT INTO jokes (joke_id, batch_id, joke_text) VALUES ($1, $2, $3) RETURNING joke_id`,
				nextID("jokes", "joke_id"), batchID, text))
		}
		return batchID, jokes
	}
	decisions := func(jokes []int64, release bool) []ports.JokePublishDecision {
		return []ports.JokePublishDecision{
			{JokeID: jokes[0], Title: "Kept", IsPublished: release},
			{JokeID: jokes[1], Title: "", IsPublished: false},
		}
	}
	storedNote := func(batchID int64) *string {
		t.Helper()
		var note *string
		if err := pool.QueryRow(ctx, `SELECT marketing_note FROM batches WHERE batch_id = $1`, batchID).Scan(&note); err != nil {
			t.Fatal(err)
		}
		return note
	}

	const note = "Shorter setups.\n<b>More</b> puns — 😀"

	// A release with a note stores it and returns it on the processed batch.
	released, releasedJokes := newBatch(true)
	res, err := repo.PublishBatch(ctx, released, userID, teamID, decisions(releasedJokes, true), true, notePtr(note))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if res.Batch.MarketingNote == nil || *res.Batch.MarketingNote != note {
		t.Fatalf("release result note = %v; want %q", res.Batch.MarketingNote, note)
	}

	// A whole-batch discard with a note stores it too.
	discarded, discardedJokes := newBatch(true)
	if _, err := repo.PublishBatch(ctx, discarded, userID, teamID, decisions(discardedJokes, false), false, notePtr("Nothing landed.")); err != nil {
		t.Fatalf("discard: %v", err)
	}

	// No note stores NULL.
	silent, silentJokes := newBatch(true)
	if _, err := repo.PublishBatch(ctx, silent, userID, teamID, decisions(silentJokes, true), true, nil); err != nil {
		t.Fatalf("silent release: %v", err)
	}

	// A publish that fails inside the transaction (all-discard where one
	// publish is required) writes no note and leaves the batch SUBMITTED.
	rejected, rejectedJokes := newBatch(true)
	if _, err := repo.PublishBatch(ctx, rejected, userID, teamID, decisions(rejectedJokes, false), true, notePtr("lost")); !domain.IsValidationError(err) {
		t.Fatalf("rejected publish: err = %v; want NO_JOKE_PUBLISHED", err)
	}
	var rejectedStatus string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM batches WHERE batch_id = $1`, rejected).Scan(&rejectedStatus); err != nil {
		t.Fatal(err)
	}
	if rejectedStatus != "SUBMITTED" || storedNote(rejected) != nil {
		t.Fatalf("rejected batch: status %s note %v; want SUBMITTED and NULL", rejectedStatus, storedNote(rejected))
	}

	// A batch processed before the migration has NULL.
	legacy, _ := newBatch(false)

	want := map[int64]*string{released: notePtr(note), discarded: notePtr("Nothing landed."), silent: nil, rejected: nil, legacy: nil}
	for id, w := range want {
		if got := storedNote(id); (got == nil) != (w == nil) || (got != nil && *got != *w) {
			t.Fatalf("batch %d stored note = %v; want %v", id, got, w)
		}
	}

	// The team batch list reads the column back on every batch.
	listed, err := repo.ListBatchesByTeam(ctx, roundID, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != len(want) {
		t.Fatalf("listed %d batches; want %d", len(listed), len(want))
	}
	for _, b := range listed {
		w := want[b.ID]
		if (b.MarketingNote == nil) != (w == nil) || (b.MarketingNote != nil && *b.MarketingNote != *w) {
			t.Fatalf("listed batch %d note = %v; want %v", b.ID, b.MarketingNote, w)
		}
	}
}

func notePtr(s string) *string { return &s }
