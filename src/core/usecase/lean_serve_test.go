package usecase

import (
	"context"
	"log/slog"
	"math/rand"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase/testutil"
)

// leanRound seeds a LEAN round with two published, classified jokes (fit 11
// and fit 7) on two teams and returns the round and the joke ids.
func leanRound(t *testing.T, st *testutil.Store) (*domain.Round, int64, int64) {
	t.Helper()
	ctx := context.Background()
	cfg := domain.DefaultRoundConfig()
	cfg.MarketMode = domain.MarketLean
	cfg.CustomerCount = 10
	cfg.CustomerBudget = 3
	cfg.MarketPrice = 1
	cfg.BuyThreshold = 7
	cfg.Jitter = 0
	cfg.ArrivalWindowSeconds = 100
	cfg.PreferenceBase = 2
	if _, err := st.InsertRoundConfig(ctx, 1, &cfg); err != nil {
		t.Fatal(err)
	}
	st.Teams = []domain.Team{{ID: 1, Name: "Team 1"}, {ID: 2, Name: "Team 2"}}
	if _, err := st.StartRound(ctx, 1); err != nil {
		t.Fatal(err)
	}
	round, err := st.GetRoundByID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	const hi, lo int64 = 101, 201
	title := "t"
	st.Batches[1] = &domain.Batch{ID: 1, RoundID: 1, TeamID: 1, Status: domain.BatchProcessed, Jokes: []domain.Joke{
		{ID: hi, BatchID: 1, Text: "strong", Title: &title, PublishStatus: domain.PublishPublished},
	}}
	st.Batches[2] = &domain.Batch{ID: 2, RoundID: 1, TeamID: 2, Status: domain.BatchProcessed, Jokes: []domain.Joke{
		{ID: lo, BatchID: 2, Text: "fair", Title: &title, PublishStatus: domain.PublishPublished},
	}}
	st.NextBatch, st.NextJoke = 3, 202
	st.JokeFits[hi] = &domain.JokeFit{JokeID: hi, RoundID: 1, TrueFit: 11}
	st.JokeFits[lo] = &domain.JokeFit{JokeID: lo, RoundID: 1, TrueFit: 7}
	return round, hi, lo
}

func mustList(t *testing.T, st *testutil.Store, roundID int64) []domain.AICustomer {
	t.Helper()
	custs, err := st.ListAICustomers(context.Background(), roundID)
	if err != nil {
		t.Fatal(err)
	}
	return custs
}

func countServed(t *testing.T, st *testutil.Store, roundID int64) int {
	t.Helper()
	n := 0
	for _, c := range mustList(t, st, roundID) {
		if c.ShoppedAt != nil {
			n++
		}
	}
	return n
}

func soldCount(t *testing.T, st *testutil.Store, jokeID int64) int {
	t.Helper()
	return testutil.PurchaseCount(st, jokeID)
}

func TestLeanCustomersAreStampedWithArrivals(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	svc := NewAICustomerService(st, rand.New(rand.NewSource(1)), slog.Default())
	if err := svc.GenerateCustomers(context.Background(), round); err != nil {
		t.Fatal(err)
	}
	custs, _ := st.ListAICustomers(context.Background(), round.ID)
	if len(custs) != 10 {
		t.Fatalf("len %d", len(custs))
	}
	for _, c := range custs {
		if c.ArrivesAt == nil || c.ShoppedAt != nil {
			t.Fatalf("customer not scheduled: %+v", c)
		}
		off := c.ArrivesAt.Sub(*round.StartedAt).Seconds()
		if off < 10 || off > 100 {
			t.Fatalf("arrival offset %.0fs outside window", off)
		}
	}
}

func TestServeArrivalsServesOnlyDueCustomersOnce(t *testing.T) {
	st := testutil.NewStore()
	round, hi, lo := leanRound(t, st)
	svc := NewAICustomerService(st, rand.New(rand.NewSource(1)), slog.Default())
	_ = svc.GenerateCustomers(context.Background(), round)

	halfway := round.StartedAt.Add(50 * time.Second)
	if err := svc.ServeArrivals(context.Background(), round.ID, halfway); err != nil {
		t.Fatal(err)
	}
	served := 0
	for _, c := range mustList(t, st, round.ID) {
		if c.ShoppedAt != nil {
			served++
			if c.RemainingBudget != 1 { // budget 3, price 1, two candidates → bought both
				t.Fatalf("budget %.2f after shopping", c.RemainingBudget)
			}
		}
	}
	if served != 5 {
		t.Fatalf("served %d at halfway, want 5", served)
	}

	// Serving again at the same instant does nothing new.
	_ = svc.ServeArrivals(context.Background(), round.ID, halfway)
	if n := countServed(t, st, round.ID); n != 5 {
		t.Fatalf("re-serve changed count to %d", n)
	}

	// Everyone is served by the end of the window; the fit-11 joke sold to all 10.
	_ = svc.ServeArrivals(context.Background(), round.ID, round.StartedAt.Add(100*time.Second))
	if n := countServed(t, st, round.ID); n != 10 {
		t.Fatalf("served %d at window end", n)
	}
	if sold := soldCount(t, st, hi); sold != 10 {
		t.Fatalf("fit-11 joke sold %d times", sold)
	}
	if sold := soldCount(t, st, lo); sold != 10 {
		t.Fatalf("fit-7 joke sold %d times (budget allows both)", sold)
	}
}

func TestServeArrivalsSkipsJokesBelowThreshold(t *testing.T) {
	st := testutil.NewStore()
	round, hi, lo := leanRound(t, st)
	round.BuyThreshold = 9 // fit-7 joke no longer qualifies
	st.Rounds[round.ID] = round
	svc := NewAICustomerService(st, rand.New(rand.NewSource(1)), slog.Default())
	_ = svc.GenerateCustomers(context.Background(), round)
	_ = svc.ServeArrivals(context.Background(), round.ID, round.StartedAt.Add(time.Hour))
	if soldCount(t, st, lo) != 0 || soldCount(t, st, hi) != 10 {
		t.Fatalf("threshold ignored: lo=%d hi=%d", soldCount(t, st, lo), soldCount(t, st, hi))
	}
}

func TestServeArrivalsIsANoOpInClassic(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	round.MarketMode = domain.MarketClassic
	st.Rounds[round.ID] = round
	svc := NewAICustomerService(st, rand.New(rand.NewSource(1)), slog.Default())
	_ = svc.GenerateCustomers(context.Background(), round)
	for _, c := range mustList(t, st, round.ID) {
		if c.ArrivesAt != nil {
			t.Fatal("classic customers must not be scheduled")
		}
	}
	if err := svc.ServeArrivals(context.Background(), round.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if countServed(t, st, round.ID) != 0 {
		t.Fatal("classic customers were served")
	}
}

// ctxStore fails the serve-path calls when their ctx is done, as pgx would.
type ctxStore struct{ *testutil.Store }

func (s ctxStore) ClaimDueCustomers(ctx context.Context, roundID int64, now time.Time) ([]domain.AICustomer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.ClaimDueCustomers(ctx, roundID, now)
}

func (s ctxStore) ListPublishedCandidates(ctx context.Context, roundID int64) ([]ports.CandidateJoke, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Store.ListPublishedCandidates(ctx, roundID)
}

func (s ctxStore) BuyJoke(ctx context.Context, roundID, aiCustomerID, jokeID, teamID int64, price float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Store.BuyJoke(ctx, roundID, aiCustomerID, jokeID, teamID, price)
}

func TestServeArrivalsSurvivesCancelledRequest(t *testing.T) {
	st := testutil.NewStore()
	round, hi, lo := leanRound(t, st)
	svc := NewAICustomerService(ctxStore{st}, rand.New(rand.NewSource(1)), slog.Default())
	if err := svc.GenerateCustomers(context.Background(), round); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.ServeArrivals(ctx, round.ID, round.StartedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := countServed(t, st, round.ID); n != 10 {
		t.Fatalf("served %d, want 10", n)
	}
	if soldCount(t, st, hi) != 10 || soldCount(t, st, lo) != 10 {
		t.Fatalf("purchases lost to cancellation: hi=%d lo=%d", soldCount(t, st, hi), soldCount(t, st, lo))
	}
}
