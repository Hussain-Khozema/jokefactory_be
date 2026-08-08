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

func TestEndToEndRound(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	ai := usecase.NewAICustomerService(store, rng, log)
	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, ai, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)
	feedback := usecase.NewFeedbackService(store, log)
	stats := usecase.NewStatsService(store, log)
	roundSvc := usecase.NewRoundService(store, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 2
	defaults.CustomerCount = 5
	defaults.CustomerBudget = 3
	defaults.MarketPrice = 1
	defaults.CostOfPublishing = 0.10
	defaults.CostOfDiscard = 0.01
	defaults.BuyThreshold = 7
	defaults.Jitter = 0
	defaults.FeedbackJokeCount = 3
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
	round, err := instructor.StartRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	customers, err := store.ListAICustomers(ctx, round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(customers) != 5 {
		t.Fatalf("ai customers = %d, want 5", len(customers))
	}

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)

	batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{
		"short joke for medium length check xx words here now",
		"discard me please",
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	pubID := item.Jokes[0].ID
	discID := item.Jokes[1].ID

	fixed := map[int64]map[domain.Dimension]string{
		pubID: testutil.IdealLLMCats(profile),
	}
	classSvc := usecase.NewClassificationService(store, llm.StubClassifier{Fixed: fixed}, ai, "stub", log)

	if _, err := marketing.Publish(ctx, mkt.ID, batch.ID, []ports.JokePublishDecision{
		{JokeID: pubID, Title: "Corporate Comedy", IsPublished: true},
		{JokeID: discID, Title: "", IsPublished: false},
	}); err != nil {
		t.Fatal(err)
	}
	if err := classSvc.ProcessBatch(ctx, batch.ID); err != nil {
		t.Fatal(err)
	}

	fit, err := store.GetJokeFit(ctx, pubID)
	if err != nil {
		t.Fatal(err)
	}
	if fit.TrueFit < 7 {
		t.Fatalf("true_fit = %v, want >= 7", fit.TrueFit)
	}

	if testutil.PurchaseCount(store, pubID) != 5 {
		t.Fatalf("sold = %d, want 5", testutil.PurchaseCount(store, pubID))
	}

	fb, err := feedback.Get(ctx, 1, *jm.TeamID, mkt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fb) != 1 || fb[0].JokeID != pubID || !fb[0].WasBought {
		t.Fatalf("feedback = %+v", fb)
	}
	if len(fb[0].GoodDimensions)+len(fb[0].ImproveDimensions) == 0 {
		t.Fatal("expected curated dimensions")
	}

	wantProfit := 5*1.0 - 1*0.10 - 1*0.01
	summary, err := roundSvc.TeamSummary(ctx, 1, *jm.TeamID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Points != 5 || summary.PublishedJokes != 1 || summary.DiscardedJokes != 1 {
		t.Fatalf("summary counters = %+v", summary)
	}
	if diff := summary.Profit - wantProfit; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("profit = %v, want %v", summary.Profit, wantProfit)
	}

	roundStats, err := stats.GetRoundStats(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(roundStats.Leaderboard) == 0 {
		t.Fatal("expected leaderboard rows")
	}
	found := false
	for _, row := range roundStats.Leaderboard {
		if row.Team.ID == *jm.TeamID {
			found = true
			if row.TotalSales != 5 || row.PublishedJokes != 1 {
				t.Fatalf("leaderboard row = %+v", row)
			}
			if diff := row.Profit - wantProfit; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("leaderboard profit = %v, want %v", row.Profit, wantProfit)
			}
		}
	}
	if !found {
		t.Fatal("team missing from leaderboard")
	}

	market, err := ai.Market(ctx, jm.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(market) != 1 || market[0].SoldCount != 5 {
		t.Fatalf("market = %+v", market)
	}

	if _, err := instructor.EndRound(ctx, 1); err != nil {
		t.Fatal(err)
	}
	ended, err := store.GetRoundByID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != domain.RoundEnded {
		t.Fatalf("status = %s, want ENDED", ended.Status)
	}
}
