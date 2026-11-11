package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/worker"
)

func TestFeedbackService(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)
	feedback := usecase.NewFeedbackService(store, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 1
	defaults.FeedbackJokeCount = 2
	defaults.FeedbackPassThreshold = 0.75
	if _, err := store.InsertRoundConfig(ctx, 1, &defaults); err != nil {
		t.Fatal(err)
	}
	users := joinMany(t, session, []string{"JM1", "Mkt1", "JM2", "Mkt2"})
	if _, err := instructor.Assign(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	cfg := defaults
	if _, err := instructor.Config(ctx, 1, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)

	cats := map[domain.Dimension]string{}
	publishAndFit := func(title string, dimFits map[domain.Dimension]float64, bought bool) int64 {
		t.Helper()
		batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"a short joke text"}, "")
		if err != nil {
			t.Fatal(err)
		}
		item, err := marketing.QueueNext(ctx, mkt.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		jokeID := item.Jokes[0].ID
		if _, err := marketing.Publish(ctx, mkt.ID, batch.ID, []ports.JokePublishDecision{
			{JokeID: jokeID, Title: title, IsPublished: true},
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.PersistJokeFits(ctx, []ports.JokeFitMaterialization{{
			JokeID: jokeID, RoundID: 1, Categories: cats,
			DimFits: dimFits, TrueFit: 5,
		}}); err != nil {
			t.Fatal(err)
		}
		if bought {
			custID := store.NextAICust
			store.NextAICust++
			store.AICustomers[custID] = &domain.AICustomer{
				ID: custID, RoundID: 1, PersonalThreshold: 1,
				StartingBudget: 3, RemainingBudget: 2, CreatedAt: time.Now().UTC(),
			}
			if err := store.BuyJoke(ctx, 1, custID, jokeID, *jm.TeamID, 1); err != nil {
				t.Fatal(err)
			}
		}
		return jokeID
	}

	allFail := map[domain.Dimension]float64{}
	mixed := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		allFail[dim] = 0.0
		mixed[dim] = 1.0
	}
	mixed[domain.DimLength] = 0
	mixed[domain.DimTopic] = 0
	mixed[domain.DimHumorStyle] = 0
	mixed[domain.DimComplexity] = 0

	_ = publishAndFit("Older Joke", mixed, false)
	time.Sleep(2 * time.Millisecond)
	newerID := publishAndFit("Corporate Comedy", mixed, true)
	time.Sleep(2 * time.Millisecond)
	// The newest joke misses everything and is classified off the ideal on every
	// ordered scale, so each ordinal dimension it shows carries a direction.
	cats = map[domain.Dimension]string{
		domain.DimLength: "Long", domain.DimTopic: "Food", domain.DimHumorStyle: "Pun",
		domain.DimComplexity: "Expert", domain.DimEdginess: "Slightly edgy", domain.DimStructure: "One-liner",
		domain.DimWordplay: "None", domain.DimFreshness: "Current", domain.DimSetupPayoff: "Immediate",
		domain.DimClarity: "Ambiguous", domain.DimEnergy: "Deadpan", domain.DimTitleFit: "Weak",
	}
	newestID := publishAndFit("Newest", allFail, false)

	items, err := feedback.Get(ctx, 1, *jm.TeamID, mkt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].JokeID != newestID || items[1].JokeID != newerID {
		t.Fatalf("order = [%d,%d], want [%d,%d]", items[0].JokeID, items[1].JokeID, newestID, newerID)
	}
	if !items[1].WasBought {
		t.Fatal("Corporate Comedy should be was_bought=true")
	}
	// Each joke walks the dimensions from its own starting point (jokeID mod 12).
	wantGood, wantImprove := usecase.SelectFeedbackDimensionsFrom(mixed, 0.75, usecase.FeedbackStart(newerID))
	if !reflect.DeepEqual(items[1].ImproveDimensions, wantImprove) || !reflect.DeepEqual(items[1].GoodDimensions, wantGood) {
		t.Fatalf("mixed good=%v improve=%v, want good=%v improve=%v",
			items[1].GoodDimensions, items[1].ImproveDimensions, wantGood, wantImprove)
	}
	failed := map[string]bool{"LENGTH": true, "TOPIC": true, "HUMOR_STYLE": true, "COMPLEXITY": true}
	if len(items[1].ImproveDimensions) != 3 || len(items[1].GoodDimensions) != 2 {
		t.Fatalf("mixed wants 3 improve + 2 good, got %v / %v", items[1].ImproveDimensions, items[1].GoodDimensions)
	}
	for _, d := range items[1].ImproveDimensions {
		if !failed[d] {
			t.Fatalf("improve %s is not a failed dimension", d)
		}
	}
	for _, d := range items[1].GoodDimensions {
		if failed[d] {
			t.Fatalf("good %s is a failed dimension", d)
		}
	}
	// No categories were stored for these jokes, so no direction hints.
	if len(items[1].ImproveHints) != 0 {
		t.Fatalf("hints without categories: %v", items[1].ImproveHints)
	}
	wantHint := map[string]string{
		"LENGTH": usecase.HintLower, "COMPLEXITY": usecase.HintLower, "WORDPLAY": usecase.HintHigher,
		"FRESHNESS": usecase.HintLower, "SETUP_PAYOFF": usecase.HintHigher, "CLARITY": usecase.HintLower,
		"ENERGY": usecase.HintHigher,
	}
	if len(items[0].ImproveDimensions) == 0 {
		t.Fatal("newest joke has no improve dimensions")
	}
	for _, d := range items[0].ImproveDimensions {
		if got := items[0].ImproveHints[d]; got != wantHint[d] {
			t.Fatalf("hint for %s = %q, want %q (hints %v)", d, got, wantHint[d], items[0].ImproveHints)
		}
	}

	if _, err := feedback.Get(ctx, 1, *jm.TeamID, jm.ID); err != nil {
		t.Fatalf("JM feedback: %v", err)
	}

	otherMkt := findOtherTeamMarketing(t, session, users, *jm.TeamID)
	if _, err := feedback.Get(ctx, 1, *jm.TeamID, otherMkt.ID); err == nil {
		t.Fatal("expected forbidden for other team")
	}
}
