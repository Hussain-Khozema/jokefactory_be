package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

// setupSubmitRound starts an active round with the given batch size and returns
// the batch service plus a JM on an assigned team, ready to submit.
func setupSubmitRound(t *testing.T, batchSize int) (*usecase.BatchService, *domain.User) {
	t.Helper()
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = batchSize
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatalf("seed round: %v", err)
	}

	users := joinMany(t, session, []string{"JM1", "Mkt1"})
	if _, err := instructor.Assign(ctx, 1, 1); err != nil {
		t.Fatalf("assign: %v", err)
	}
	cfg := defaults
	if _, err := instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := instructor.StartRound(ctx, 1); err != nil {
		t.Fatalf("start: %v", err)
	}

	return batches, findJM(t, session, users)
}

func TestSubmitRawTextStoresBlob(t *testing.T) {
	ctx := context.Background()
	batches, jm := setupSubmitRound(t, 2)

	raw := "1) Why did the chicken cross the road? 2) I told my boss a joke."
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, nil, raw)
	if err != nil {
		t.Fatalf("raw submit: %v", err)
	}
	if batch.RawText == nil || *batch.RawText != raw {
		t.Fatalf("raw_text = %v, want %q", batch.RawText, raw)
	}
	if batch.RawTextOriginal == nil || *batch.RawTextOriginal != raw {
		t.Fatalf("raw_text_original = %v, want %q", batch.RawTextOriginal, raw)
	}
	if len(batch.Jokes) != 0 {
		t.Fatalf("expected no jokes on a raw submit, got %d", len(batch.Jokes))
	}
	if batch.Status != domain.BatchSubmitted {
		t.Fatalf("status = %s, want SUBMITTED", batch.Status)
	}
}

func TestSubmitRejectsBothJokesAndRawText(t *testing.T) {
	ctx := context.Background()
	batches, jm := setupSubmitRound(t, 2)

	raw := "1) Why did the chicken cross the road? 2) I told my boss a joke."
	_, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"Setup one?", "Punchline one."}, raw)
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected a validation error for jokes+raw_text, got %v", err)
	}
}

func TestSubmitRejectsNeitherJokesNorRawText(t *testing.T) {
	ctx := context.Background()
	batches, jm := setupSubmitRound(t, 2)

	_, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, nil, "")
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected a validation error for an empty payload, got %v", err)
	}
}

func TestSubmitRejectsShortRawText(t *testing.T) {
	ctx := context.Background()
	batches, jm := setupSubmitRound(t, 2)

	_, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, nil, "too short")
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected a validation error for a short raw blob, got %v", err)
	}
}

// TestSubmitJokesArrayStillWorks pins the pre-existing jokes-array path,
// including R1's exact-batch-size rule, which must survive the raw path.
func TestSubmitJokesArrayStillWorks(t *testing.T) {
	ctx := context.Background()
	batches, jm := setupSubmitRound(t, 2)

	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"Setup one?", "Punchline one."}, "")
	if err != nil {
		t.Fatalf("jokes submit: %v", err)
	}
	if len(batch.Jokes) != 2 {
		t.Fatalf("expected 2 jokes, got %d", len(batch.Jokes))
	}
	if batch.RawText != nil {
		t.Fatalf("expected raw_text nil on the jokes path, got %q", *batch.RawText)
	}

	// R1 requires exactly batch_size jokes.
	if _, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"Only one."}, ""); err == nil ||
		!domain.IsValidationError(err) {
		t.Fatalf("expected R1 exact-batch-size validation error, got %v", err)
	}
	if _, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID,
		[]string{"One.", "Two.", "Three."}, ""); err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected R1 exact-batch-size validation error for 3 jokes, got %v", err)
	}
}
