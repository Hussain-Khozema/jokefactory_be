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
	"jokefactory/src/infra/worker"
)

func TestSelectFeedbackDimensionsPreferImprove(t *testing.T) {
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 1.0
	}
	fits[domain.DimLength] = 0
	fits[domain.DimTopic] = 0
	fits[domain.DimHumorStyle] = 0
	fits[domain.DimComplexity] = 0

	good, improve := usecase.SelectFeedbackDimensions(fits, 0.75)
	wantImprove := []string{"LENGTH", "TOPIC", "HUMOR_STYLE"}
	wantGood := []string{"EDGINESS", "STRUCTURE"}
	if !reflect.DeepEqual(improve, wantImprove) {
		t.Fatalf("improve = %v, want %v", improve, wantImprove)
	}
	if !reflect.DeepEqual(good, wantGood) {
		t.Fatalf("good = %v, want %v", good, wantGood)
	}
}

func TestSelectFeedbackDimensionsBackfillPassWhenFewFails(t *testing.T) {
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 1.0
	}
	fits[domain.DimLength] = 0

	good, improve := usecase.SelectFeedbackDimensions(fits, 0.75)
	if !reflect.DeepEqual(improve, []string{"LENGTH"}) {
		t.Fatalf("improve = %v", improve)
	}
	wantGood := []string{"TOPIC", "HUMOR_STYLE", "COMPLEXITY", "EDGINESS"}
	if !reflect.DeepEqual(good, wantGood) {
		t.Fatalf("good = %v, want %v", good, wantGood)
	}
}

func TestSelectFeedbackDimensionsBackfillFailWhenFewPasses(t *testing.T) {
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 0
	}
	fits[domain.DimTitleFit] = 1.0

	good, improve := usecase.SelectFeedbackDimensions(fits, 0.75)
	if !reflect.DeepEqual(good, []string{"TITLE_FIT"}) {
		t.Fatalf("good = %v", good)
	}
	wantImprove := []string{"LENGTH", "TOPIC", "HUMOR_STYLE", "COMPLEXITY"}
	if !reflect.DeepEqual(improve, wantImprove) {
		t.Fatalf("improve = %v, want %v", improve, wantImprove)
	}
}

func TestSelectFeedbackDimensionsEmpty(t *testing.T) {
	good, improve := usecase.SelectFeedbackDimensions(nil, 0.75)
	if len(good) != 0 || len(improve) != 0 {
		t.Fatalf("got good=%v improve=%v", good, improve)
	}
}

func TestPhase8FeedbackEndpoint(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
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
	if _, err := instructor.Config(ctx, 1, &cfg, validIdealProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}

	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)

	publishAndFit := func(title string, dimFits map[domain.Dimension]float64, bought bool) int64 {
		t.Helper()
		batch, err := batches.Submit(ctx, jm.ID, 1, *jm.TeamID, []string{"a short joke text"})
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
			JokeID: jokeID, RoundID: 1, Categories: map[domain.Dimension]string{},
			DimFits: dimFits, TrueFit: 5,
		}}); err != nil {
			t.Fatal(err)
		}
		if bought {
			custID := store.nextAICust
			store.nextAICust++
			store.aiCustomers[custID] = &domain.AICustomer{
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
	if !reflect.DeepEqual(items[1].ImproveDimensions, []string{"LENGTH", "TOPIC", "HUMOR_STYLE"}) {
		t.Fatalf("mixed improve = %v", items[1].ImproveDimensions)
	}
	if !reflect.DeepEqual(items[1].GoodDimensions, []string{"EDGINESS", "STRUCTURE"}) {
		t.Fatalf("mixed good = %v", items[1].GoodDimensions)
	}

	if _, err := feedback.Get(ctx, 1, *jm.TeamID, jm.ID); err != nil {
		t.Fatalf("JM feedback: %v", err)
	}

	otherMkt := findOtherTeamMarketing(t, session, users, *jm.TeamID)
	if _, err := feedback.Get(ctx, 1, *jm.TeamID, otherMkt.ID); err == nil {
		t.Fatal("expected forbidden for other team")
	}
}

func findOtherTeamMarketing(t *testing.T, session *usecase.SessionService, users []*domain.User, excludeTeam int64) *domain.User {
	t.Helper()
	ctx := context.Background()
	for _, u := range users {
		me, err := session.Me(ctx, u.ID)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		if me.User.Role != nil && *me.User.Role == domain.RoleMarketing &&
			me.User.TeamID != nil && *me.User.TeamID != excludeTeam {
			return me.User
		}
	}
	t.Fatal("expected MARKETING on another team")
	return nil
}
