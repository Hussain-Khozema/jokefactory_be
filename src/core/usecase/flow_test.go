package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

func TestPreMarketingFlow(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)

	defaults := domain.DefaultRoundConfig()
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatalf("seed round: %v", err)
	}

	names := []string{"Alice", "Bob", "Carol", "Dave"}
	users := make([]*domain.User, 0, len(names))
	for _, name := range names {
		res, err := session.Join(ctx, name)
		if err != nil {
			t.Fatalf("join %s: %v", name, err)
		}
		if res.User.Status != domain.ParticipantWaiting {
			t.Fatalf("expected WAITING, got %s", res.User.Status)
		}
		users = append(users, res.User)
	}

	lobby, err := instructor.Assign(ctx, 1, 2)
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if lobby.Summary.TeamCount != 2 {
		t.Fatalf("expected 2 teams, got %d", lobby.Summary.TeamCount)
	}
	if lobby.Summary.Assigned != 4 {
		t.Fatalf("expected 4 assigned, got %d", lobby.Summary.Assigned)
	}

	jm := findJM(t, session, users)

	cfg := domain.DefaultRoundConfig()
	cfg.BatchSize = 2
	cfg.BuyThreshold = 7.5
	profile := testutil.ValidIdealProfile()
	result, err := instructor.Config(ctx, 1, &cfg, profile)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if result.Round.BatchSize != 2 || result.Round.BuyThreshold != 7.5 {
		t.Fatalf("config not persisted: %+v", result.Round)
	}
	if len(result.IdealProfile) != len(scoring.IdealDimensions()) {
		t.Fatalf("ideal profile size = %d", len(result.IdealProfile))
	}

	started, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Status != domain.RoundActive {
		t.Fatalf("expected ACTIVE, got %s", started.Status)
	}

	jokes := []string{"Why did the chicken cross the road?", "Because it could."}
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, jokes)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if batch.Status != domain.BatchSubmitted {
		t.Fatalf("expected SUBMITTED, got %s", batch.Status)
	}

	listed, err := batches.List(ctx, 1, *jm.TeamID, jm.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || len(listed[0].Jokes) != 2 {
		t.Fatalf("list unexpected: %+v", listed)
	}
}

func TestStartRequiresIdealProfile(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	instructor := usecase.NewInstructorService(store, nil, log)

	defaults := domain.DefaultRoundConfig()
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatal(err)
	}
	_, err := instructor.StartRound(ctx, 1)
	if err == nil || !domain.IsConflict(err) {
		t.Fatalf("expected conflict without ideal profile, got %v", err)
	}
}

func TestConfigRejectsInvalidIdeal(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	instructor := usecase.NewInstructorService(store, nil, log)

	bad := testutil.ValidIdealProfile()
	bad[domain.DimTopic] = "NotARealTopic"
	cfg := domain.DefaultRoundConfig()
	_, err := instructor.Config(ctx, 1, &cfg, bad)
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
