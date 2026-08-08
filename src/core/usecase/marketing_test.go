package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/worker"
)

func TestMarketingPublishFlow(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 2
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatal(err)
	}

	users := joinMany(t, session, []string{"JM1", "Mkt1", "JM2", "Mkt2"})
	if _, err := instructor.Assign(ctx, 1, 2); err != nil {
		t.Fatalf("assign: %v", err)
	}
	cfg := defaults
	if _, err := instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := instructor.StartRound(ctx, 1); err != nil {
		t.Fatalf("start: %v", err)
	}

	jm := findJM(t, session, users)
	mkt := findMarketing(t, session, users)
	if *jm.TeamID != *mkt.TeamID {
		mkt = findMarketingOnTeam(t, session, users, *jm.TeamID)
	}

	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"Setup one?", "Punchline one."})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	count, err := marketing.QueueCount(ctx, mkt.ID, 1)
	if err != nil || count != 1 {
		t.Fatalf("queue count = %d, err=%v", count, err)
	}

	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatalf("queue next: %v", err)
	}
	if item.Batch.ID != batch.ID {
		t.Fatalf("claimed batch %d, want %d", item.Batch.ID, batch.ID)
	}
	if item.Batch.LockedBy == nil || *item.Batch.LockedBy != mkt.ID {
		t.Fatalf("expected locked_by=%d", mkt.ID)
	}

	again, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil || again.Batch.ID != batch.ID {
		t.Fatalf("reclaim: %+v err=%v", again, err)
	}

	decisions := []ports.JokePublishDecision{
		{JokeID: item.Jokes[0].ID, Title: "Corporate Comedy", IsPublished: true},
		{JokeID: item.Jokes[1].ID, Title: "Meeting Blues", IsPublished: false},
	}
	result, err := marketing.Publish(ctx, mkt.ID, batch.ID, decisions)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Batch.Status != domain.BatchProcessed {
		t.Fatalf("status = %s", result.Batch.Status)
	}
	if len(result.PublishedIDs) != 1 || len(result.DiscardedIDs) != 1 {
		t.Fatalf("published=%v discarded=%v", result.PublishedIDs, result.DiscardedIDs)
	}

	listed, err := batches.List(ctx, 1, *jm.TeamID, jm.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listed[0].Jokes[0].PublishStatus != domain.PublishPublished {
		t.Fatalf("joke0 status = %s", listed[0].Jokes[0].PublishStatus)
	}
	if listed[0].Jokes[1].PublishStatus != domain.PublishDiscarded {
		t.Fatalf("joke1 status = %s", listed[0].Jokes[1].PublishStatus)
	}

	st := store.TeamState[[2]int64{1, *jm.TeamID}]
	if st == nil || st.BatchesProcessed != 1 || st.PublishedJokes != 1 || st.DiscardedJokes != 1 {
		t.Fatalf("team state counters wrong: %+v", st)
	}

	count, err = marketing.QueueCount(ctx, mkt.ID, 1)
	if err != nil || count != 0 {
		t.Fatalf("post-publish queue count = %d err=%v", count, err)
	}

	_, err = marketing.Publish(ctx, mkt.ID, batch.ID, decisions)
	if err == nil || !domain.IsConflict(err) {
		t.Fatalf("expected BATCH_ALREADY_PROCESSED, got %v", err)
	}
}

func TestPublishRequiresAtLeastOnePublished(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 2
	_, _ = store.InsertRoundConfig(ctx, 1, &defaults)
	users := joinMany(t, session, []string{"A", "B", "C", "D"})
	_, _ = instructor.Assign(ctx, 1, 2)
	cfg := defaults
	_, _ = instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile())
	_, _ = instructor.StartRound(ctx, 1)

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"j1", "j2"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	decisions := []ports.JokePublishDecision{
		{JokeID: item.Jokes[0].ID, Title: "A", IsPublished: false},
		{JokeID: item.Jokes[1].ID, Title: "B", IsPublished: false},
	}
	_, err = marketing.Publish(ctx, mkt.ID, batch.ID, decisions)
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("expected NO_JOKE_PUBLISHED, got %v", err)
	}
}
