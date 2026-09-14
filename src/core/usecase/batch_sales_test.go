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
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/llm"
	"jokefactory/src/infra/worker"
)

// salesFixture drives one joke all the way to real AI-customer purchases so the
// batches listing and the market board can be compared on the same joke.
type salesFixture struct {
	batches   *usecase.BatchService
	ai        *usecase.AICustomerService
	jm        *domain.User
	teamID    int64
	soldJoke  int64
	unsoldJok int64
	customers int
}

func newSalesFixture(t *testing.T) *salesFixture {
	t.Helper()
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	ai := usecase.NewAICustomerService(store, rng, log)
	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, ai, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 2
	defaults.CustomerCount = 5
	defaults.CustomerBudget = 3
	defaults.MarketPrice = 1
	defaults.BuyThreshold = 7
	defaults.Jitter = 0
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
	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{
		"short joke for medium length check xx words here now",
		"discard me please",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	pubID, discID := item.Jokes[0].ID, item.Jokes[1].ID

	classSvc := usecase.NewClassificationService(store, llm.StubClassifier{
		Fixed: map[int64]map[domain.Dimension]string{pubID: testutil.IdealLLMCats(profile)},
	}, ai, "stub", log)

	if _, err := marketing.Publish(ctx, mkt.ID, batch.ID, []ports.JokePublishDecision{
		{JokeID: pubID, Title: "Corporate Comedy", IsPublished: true},
		{JokeID: discID, Title: "", IsPublished: false},
	}); err != nil {
		t.Fatal(err)
	}
	if err := classSvc.ProcessBatch(ctx, batch.ID); err != nil {
		t.Fatal(err)
	}
	if got := testutil.PurchaseCount(store, pubID); got != 5 {
		t.Fatalf("fixture expected 5 purchases, got %d", got)
	}

	return &salesFixture{
		batches:   batches,
		ai:        ai,
		jm:        jm,
		teamID:    *jm.TeamID,
		soldJoke:  pubID,
		unsoldJok: discID,
		customers: 5,
	}
}

func (f *salesFixture) listedJoke(t *testing.T, jokeID int64) domain.Joke {
	t.Helper()
	listed, err := f.batches.List(context.Background(), 1, f.teamID, f.jm.ID)
	if err != nil {
		t.Fatalf("list batches: %v", err)
	}
	for _, b := range listed {
		for _, j := range b.Jokes {
			if j.ID == jokeID {
				return j
			}
		}
	}
	t.Fatalf("joke %d missing from the batches listing", jokeID)
	return domain.Joke{}
}

// The batches listing and the market board both report sold_count, and they
// must agree: the listing used to hardcode a Go zero value while the market
// board counted real purchases.
func TestBatchListingSoldCountMatchesMarket(t *testing.T) {
	f := newSalesFixture(t)

	listed := f.listedJoke(t, f.soldJoke)
	if listed.SoldCount != f.customers {
		t.Fatalf("batches listing sold_count = %d, want %d", listed.SoldCount, f.customers)
	}

	market, err := f.ai.Market(context.Background(), f.jm.ID, 1)
	if err != nil {
		t.Fatalf("market: %v", err)
	}
	var marketSold int
	for _, m := range market {
		if m.JokeID == f.soldJoke {
			marketSold = m.SoldCount
		}
	}
	if listed.SoldCount != marketSold {
		t.Fatalf("sold_count disagrees: batches listing %d, market board %d", listed.SoldCount, marketSold)
	}
}

// first_sold_at is the earliest purchase event, so a sold joke carries a
// timestamp and a joke that never sold carries none.
func TestBatchListingReportsFirstSoldAt(t *testing.T) {
	f := newSalesFixture(t)

	sold := f.listedJoke(t, f.soldJoke)
	if sold.FirstSoldAt == nil {
		t.Fatal("sold joke has no first_sold_at")
	}
	if sold.PublishedAt != nil && sold.FirstSoldAt.Before(*sold.PublishedAt) {
		t.Fatalf("first_sold_at %v predates published_at %v", *sold.FirstSoldAt, *sold.PublishedAt)
	}

	never := f.listedJoke(t, f.unsoldJok)
	if never.SoldCount != 0 {
		t.Fatalf("discarded joke sold_count = %d, want 0", never.SoldCount)
	}
	if never.FirstSoldAt != nil {
		t.Fatalf("discarded joke first_sold_at = %v, want nil", *never.FirstSoldAt)
	}
}
