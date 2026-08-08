package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/llm"
	"jokefactory/src/infra/worker"
)

func TestPublishClassifiesAndMaterializesFit(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 1
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatal(err)
	}
	users := joinMany(t, session, []string{"JM1", "Mkt1", "JM2", "Mkt2"})
	if _, err := instructor.Assign(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	cfg := defaults
	profile := testutil.ValidIdealProfile()
	if _, err := instructor.Config(ctx, 1, &cfg, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)

	jokeText := "I told my boss I needed a raise so he said do more work please today now"
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{jokeText})
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	jokeID := item.Jokes[0].ID

	fixed := map[int64]map[domain.Dimension]string{
		jokeID: testutil.IdealLLMCats(profile),
	}
	classSvc := usecase.NewClassificationService(store, llm.StubClassifier{Fixed: fixed}, nil, "stub", log)

	_, err = marketing.Publish(ctx, mkt.ID, batch.ID, []ports.JokePublishDecision{
		{JokeID: jokeID, Title: "Corporate Comedy", IsPublished: true},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if err := classSvc.ProcessBatch(ctx, batch.ID); err != nil {
		t.Fatalf("process: %v", err)
	}

	fit, err := store.GetJokeFit(ctx, jokeID)
	if err != nil {
		t.Fatalf("get fit: %v", err)
	}
	if fit.TrueFit != 12.0 {
		t.Fatalf("true_fit = %v, want 12", fit.TrueFit)
	}

	vals, err := store.ListJokeDimensionValues(ctx, jokeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != len(scoring.AllDimensions) {
		t.Fatalf("dimension values = %d, want %d", len(vals), len(scoring.AllDimensions))
	}

	job := store.ClassJobs[batch.ID]
	if job == nil || job.Status != domain.ClassificationDone {
		t.Fatalf("job status = %+v", job)
	}
}

func TestReconcilerReclassifiesOrphans(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 1
	_, _ = store.InsertRoundConfig(ctx, 1, &defaults)
	users := joinMany(t, session, []string{"A", "B", "C", "D"})
	_, _ = instructor.Assign(ctx, 1, 2)
	cfg := defaults
	_, _ = instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile())
	_, _ = instructor.StartRound(ctx, 1)

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"short joke text here"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = marketing.Publish(ctx, mkt.ID, batch.ID, []ports.JokePublishDecision{
		{JokeID: item.Jokes[0].ID, Title: "Title", IsPublished: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	job := store.ClassJobs[batch.ID]
	job.Status = domain.ClassificationProcessing
	job.UpdatedAt = time.Now().UTC().Add(-ports.StaleClassificationAfter - time.Second)

	classSvc := usecase.NewClassificationService(store, llm.StubClassifier{}, nil, "stub", log)
	dispatcher := &recordingDispatcher{proc: classSvc}
	reconciler := worker.NewReconciler(store, dispatcher, time.Hour, log)
	reconciler.Sweep(ctx)

	if store.ClassJobs[batch.ID].Status != domain.ClassificationDone {
		t.Fatalf("reconciler did not complete job: %+v", store.ClassJobs[batch.ID])
	}
	if _, err := store.GetJokeFit(ctx, item.Jokes[0].ID); err != nil {
		t.Fatalf("expected fit after reconcile: %v", err)
	}
}

func TestStubClassifierOmitsLength(t *testing.T) {
	ctx := context.Background()
	out, err := llm.StubClassifier{}.Classify(ctx, []ports.JokeInput{
		{JokeID: 1, Text: "hello", Title: "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results", len(out))
	}
	for _, dim := range scoring.LLMDimensions() {
		cat := out[0].Categories[dim]
		if !scoring.IsValidCategory(dim, cat) {
			t.Fatalf("invalid stub category %q for %s", cat, dim)
		}
	}
	if _, ok := out[0].Categories[domain.DimLength]; ok {
		t.Fatal("stub must not classify Length")
	}
}

type recordingDispatcher struct {
	proc interface {
		ProcessBatch(ctx context.Context, batchID int64) error
	}
}

func (d *recordingDispatcher) Enqueue(ctx context.Context, batchID int64) error {
	return d.proc.ProcessBatch(ctx, batchID)
}
