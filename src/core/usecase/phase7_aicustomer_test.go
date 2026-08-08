package usecase_test

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
	"jokefactory/src/infra/llm"
	"jokefactory/src/infra/worker"
)

func TestPhase7GenerateCustomersAndBuy(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	ai := usecase.NewAICustomerService(store, rng, log)
	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, ai, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 1
	defaults.CustomerCount = 10
	defaults.CustomerBudget = 3
	defaults.MarketPrice = 1
	defaults.BuyThreshold = 7
	defaults.Jitter = 0 // deterministic thresholds == 7
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatal(err)
	}
	users := joinMany(t, session, []string{"JM1", "Mkt1", "JM2", "Mkt2"})
	if _, err := instructor.Assign(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	cfg := defaults
	if _, err := instructor.Config(ctx, 1, &cfg, validIdealProfile()); err != nil {
		t.Fatal(err)
	}
	round, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	customers, err := store.ListAICustomers(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(customers) != 10 {
		t.Fatalf("customers = %d, want 10", len(customers))
	}
	for _, c := range customers {
		if c.PersonalThreshold != 7 {
			t.Fatalf("threshold = %v, want 7", c.PersonalThreshold)
		}
		if c.RemainingBudget != 3 {
			t.Fatalf("budget = %v", c.RemainingBudget)
		}
	}

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"short joke for medium length check xx"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	jokeID := item.Jokes[0].ID
	profile := validIdealProfile()
	fixed := map[int64]map[domain.Dimension]string{
		jokeID: idealLLMCats(profile),
	}
	classSvc := usecase.NewClassificationService(store, llm.StubClassifier{Fixed: fixed}, ai, "stub", log)

	if _, err := marketing.Publish(ctx, mkt.ID, batch.ID, []ports.JokePublishDecision{
		{JokeID: jokeID, Title: "Corporate Comedy", IsPublished: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := classSvc.ProcessBatch(ctx, batch.ID); err != nil {
		t.Fatal(err)
	}

	fit, err := store.GetJokeFit(ctx, jokeID)
	if err != nil {
		t.Fatal(err)
	}
	if fit.TrueFit < 7 {
		t.Fatalf("true_fit = %v, need >= 7 for buys", fit.TrueFit)
	}

	sold := 0
	for _, p := range store.purchases {
		if p.JokeID == jokeID {
			sold++
		}
	}
	if sold != 10 {
		t.Fatalf("sold_count = %d, want 10 (all customers buy)", sold)
	}

	st := store.teamState[[2]int64{1, *jm.TeamID}]
	if st == nil || st.PointsEarned != 10 {
		t.Fatalf("points = %+v, want 10", st)
	}

	market, err := ai.Market(ctx, jm.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(market) != 1 || market[0].SoldCount != 10 {
		t.Fatalf("market = %+v", market)
	}
}

func TestPhase7SwapWhenOutOfBudget(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(7)) //nolint:gosec
	ai := usecase.NewAICustomerService(store, rng, log)

	defaults := domain.DefaultRoundConfig()
	defaults.CustomerCount = 1
	defaults.CustomerBudget = 1 // can hold exactly one joke at price 1
	defaults.MarketPrice = 1
	defaults.BuyThreshold = 5
	defaults.Jitter = 0
	defaults.SwapMargin = 0.5
	round, err := store.InsertRoundConfig(ctx, 1, &defaults)
	if err != nil {
		t.Fatal(err)
	}
	round.Status = domain.RoundActive
	store.rounds[1] = round

	if err := ai.GenerateCustomers(ctx, round); err != nil {
		t.Fatal(err)
	}

	// Seed two published jokes on the same team with different fits.
	store.teams = []domain.Team{{ID: 1, Name: "Team 1"}}
	store.ensureState(1, 1)
	b := &domain.Batch{ID: 1, RoundID: 1, TeamID: 1, Status: domain.BatchProcessed}
	weak := domain.Joke{ID: 101, BatchID: 1, Text: "weak", PublishStatus: domain.PublishPublished}
	strong := domain.Joke{ID: 102, BatchID: 1, Text: "strong", PublishStatus: domain.PublishPublished}
	title := "t"
	weak.Title, strong.Title = &title, &title
	b.Jokes = []domain.Joke{weak, strong}
	store.batches[1] = b
	store.nextBatch, store.nextJoke = 2, 103
	store.jokeFits[101] = &domain.JokeFit{JokeID: 101, RoundID: 1, TrueFit: 6}
	store.jokeFits[102] = &domain.JokeFit{JokeID: 102, RoundID: 1, TrueFit: 9}

	// First evaluate weak joke → buy (budget spent).
	if err := ai.EvaluatePurchases(ctx, 1, []int64{101}); err != nil {
		t.Fatal(err)
	}
	if len(store.purchases) != 1 {
		t.Fatalf("expected 1 purchase after first buy, got %d", len(store.purchases))
	}

	// Strong joke should trigger swap (9 > 6 + 0.5).
	if err := ai.EvaluatePurchases(ctx, 1, []int64{102}); err != nil {
		t.Fatal(err)
	}
	heldStrong := false
	heldWeak := false
	for _, p := range store.purchases {
		if p.JokeID == 102 {
			heldStrong = true
		}
		if p.JokeID == 101 {
			heldWeak = true
		}
	}
	if !heldStrong || heldWeak {
		t.Fatalf("expected swap to strong only; purchases=%v", store.purchases)
	}
	st := store.teamState[[2]int64{1, 1}]
	if st.PointsEarned != 1 { // -1 return +1 buy
		t.Fatalf("points = %d, want 1", st.PointsEarned)
	}
}

func TestPhase7BelowThresholdSkipped(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(1)) //nolint:gosec
	ai := usecase.NewAICustomerService(store, rng, log)

	defaults := domain.DefaultRoundConfig()
	defaults.CustomerCount = 5
	defaults.BuyThreshold = 10
	defaults.Jitter = 0
	round, _ := store.InsertRoundConfig(ctx, 1, &defaults)
	round.Status = domain.RoundActive
	store.rounds[1] = round
	_ = ai.GenerateCustomers(ctx, round)

	store.teams = []domain.Team{{ID: 1, Name: "T1"}}
	store.ensureState(1, 1)
	b := &domain.Batch{ID: 1, RoundID: 1, TeamID: 1, Status: domain.BatchProcessed}
	j := domain.Joke{ID: 1, BatchID: 1, Text: "x", PublishStatus: domain.PublishPublished}
	b.Jokes = []domain.Joke{j}
	store.batches[1] = b
	store.jokeFits[1] = &domain.JokeFit{JokeID: 1, RoundID: 1, TrueFit: 8} // below 10

	if err := ai.EvaluatePurchases(ctx, 1, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if len(store.purchases) != 0 {
		t.Fatalf("expected no purchases, got %d", len(store.purchases))
	}
}

func idealLLMCats(profile domain.IdealProfile) map[domain.Dimension]string {
	return map[domain.Dimension]string{
		domain.DimTopic:       profile[domain.DimTopic],
		domain.DimHumorStyle:  profile[domain.DimHumorStyle],
		domain.DimComplexity:  profile[domain.DimComplexity],
		domain.DimEdginess:    profile[domain.DimEdginess],
		domain.DimStructure:   profile[domain.DimStructure],
		domain.DimWordplay:    profile[domain.DimWordplay],
		domain.DimFreshness:   profile[domain.DimFreshness],
		domain.DimSetupPayoff: profile[domain.DimSetupPayoff],
		domain.DimClarity:     profile[domain.DimClarity],
		domain.DimEnergy:      profile[domain.DimEnergy],
		domain.DimTitleFit:    "Perfect",
	}
}
