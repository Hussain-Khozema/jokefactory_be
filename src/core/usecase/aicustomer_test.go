package usecase_test

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
)

func TestSwapWhenOutOfBudget(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(7)) //nolint:gosec
	ai := usecase.NewAICustomerService(store, rng, log)

	defaults := domain.DefaultRoundConfig()
	defaults.CustomerCount = 1
	defaults.CustomerBudget = 1
	defaults.MarketPrice = 1
	defaults.BuyThreshold = 5
	defaults.Jitter = 0
	defaults.SwapMargin = 0.5
	round, err := store.InsertRoundConfig(ctx, 1, &defaults)
	if err != nil {
		t.Fatal(err)
	}
	round.Status = domain.RoundActive
	store.Rounds[1] = round

	if err := ai.GenerateCustomers(ctx, round); err != nil {
		t.Fatal(err)
	}

	store.Teams = []domain.Team{{ID: 1, Name: "Team 1"}}
	_ = store.EnsureTeamRoundState(ctx, 1, 1)
	b := &domain.Batch{ID: 1, RoundID: 1, TeamID: 1, Status: domain.BatchProcessed}
	weak := domain.Joke{ID: 101, BatchID: 1, Text: "weak", PublishStatus: domain.PublishPublished}
	strong := domain.Joke{ID: 102, BatchID: 1, Text: "strong", PublishStatus: domain.PublishPublished}
	title := "t"
	weak.Title, strong.Title = &title, &title
	b.Jokes = []domain.Joke{weak, strong}
	store.Batches[1] = b
	store.NextBatch, store.NextJoke = 2, 103
	store.JokeFits[101] = &domain.JokeFit{JokeID: 101, RoundID: 1, TrueFit: 6}
	store.JokeFits[102] = &domain.JokeFit{JokeID: 102, RoundID: 1, TrueFit: 9}

	if err := ai.EvaluatePurchases(ctx, 1, []int64{101}); err != nil {
		t.Fatal(err)
	}
	if len(store.Purchases) != 1 {
		t.Fatalf("expected 1 purchase after first buy, got %d", len(store.Purchases))
	}

	if err := ai.EvaluatePurchases(ctx, 1, []int64{102}); err != nil {
		t.Fatal(err)
	}
	heldStrong := false
	heldWeak := false
	for _, p := range store.Purchases {
		if p.JokeID == 102 {
			heldStrong = true
		}
		if p.JokeID == 101 {
			heldWeak = true
		}
	}
	if !heldStrong || heldWeak {
		t.Fatalf("expected swap to strong only; purchases=%v", store.Purchases)
	}
	st := store.TeamState[[2]int64{1, 1}]
	if st.PointsEarned != 1 {
		t.Fatalf("points = %d, want 1", st.PointsEarned)
	}
}

func TestBelowThresholdSkipped(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(1)) //nolint:gosec
	ai := usecase.NewAICustomerService(store, rng, log)

	defaults := domain.DefaultRoundConfig()
	defaults.CustomerCount = 5
	defaults.BuyThreshold = 10
	defaults.Jitter = 0
	round, _ := store.InsertRoundConfig(ctx, 1, &defaults)
	round.Status = domain.RoundActive
	store.Rounds[1] = round
	_ = ai.GenerateCustomers(ctx, round)

	store.Teams = []domain.Team{{ID: 1, Name: "T1"}}
	_ = store.EnsureTeamRoundState(ctx, 1, 1)
	b := &domain.Batch{ID: 1, RoundID: 1, TeamID: 1, Status: domain.BatchProcessed}
	j := domain.Joke{ID: 1, BatchID: 1, Text: "x", PublishStatus: domain.PublishPublished}
	b.Jokes = []domain.Joke{j}
	store.Batches[1] = b
	store.JokeFits[1] = &domain.JokeFit{JokeID: 1, RoundID: 1, TrueFit: 8}

	if err := ai.EvaluatePurchases(ctx, 1, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if len(store.Purchases) != 0 {
		t.Fatalf("expected no purchases, got %d", len(store.Purchases))
	}
}
