package usecase

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"strings"
	"testing"
	"time"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/llm"
)

// seedJokeMaker adds a JM on team 1 and returns the user id and team id.
func seedJokeMaker(t *testing.T, st *testutil.Store) (int64, int64) {
	t.Helper()
	u, err := st.CreateUser(context.Background(), "JM-lean")
	if err != nil {
		t.Fatal(err)
	}
	role := domain.RoleJM
	teamID := int64(1)
	if err := st.UpdateUserAssignment(context.Background(), u.ID, &role, &teamID); err != nil {
		t.Fatal(err)
	}
	return u.ID, teamID
}

// seedAnyUser adds an unassigned user (enough to read the market).
func seedAnyUser(t *testing.T, st *testutil.Store) int64 {
	t.Helper()
	u, err := st.CreateUser(context.Background(), "viewer")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// fullBatch returns batch_size joke texts for the round.
func fullBatch(round *domain.Round) []string {
	jokes := make([]string, round.BatchSize)
	for i := range jokes {
		jokes[i] = "joke"
	}
	return jokes
}

// seedPublishedBatchAwaitingClassification adds a batch with one published
// joke on team 1, an ideal profile, and a pending classification job.
func seedPublishedBatchAwaitingClassification(t *testing.T, st *testutil.Store, roundID int64) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertIdealProfile(ctx, roundID, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBatch(ctx, roundID, 1, []string{"a published joke"}, "")
	if err != nil {
		t.Fatal(err)
	}
	stored := st.Batches[b.ID]
	stored.Status = domain.BatchProcessed
	stored.Jokes[0].PublishStatus = domain.PublishPublished
	if err := st.EnsureClassificationJob(ctx, b.ID, roundID); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func TestSubmitRefusesWhenWIPLimitReached(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	round.WIPLimit = 1
	st.Rounds[round.ID] = round
	jm, teamID := seedJokeMaker(t, st)
	svc := NewBatchService(st, slog.Default())

	jokes := fullBatch(round)
	if _, err := svc.Submit(context.Background(), jm, round.ID, teamID, jokes, ""); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_, err := svc.Submit(context.Background(), jm, round.ID, teamID, jokes, "")
	if !domain.IsConflict(err) || !strings.Contains(err.Error(), "WIP_LIMIT_REACHED: 1 of 1 batches waiting") {
		t.Fatalf("want WIP_LIMIT_REACHED conflict, got %v", err)
	}
	// The raw-text path is held to the same limit.
	_, err = svc.Submit(context.Background(), jm, round.ID, teamID, nil, "an unsplit blob of several jokes here")
	if !domain.IsConflict(err) || !strings.Contains(err.Error(), "WIP_LIMIT_REACHED") {
		t.Fatalf("raw text: want WIP_LIMIT_REACHED conflict, got %v", err)
	}
}

func TestSubmitWIPLimitCountsRawTextBatches(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	round.WIPLimit = 2
	st.Rounds[round.ID] = round
	jm, teamID := seedJokeMaker(t, st)
	svc := NewBatchService(st, slog.Default())
	raw := "an unsplit blob of several jokes here"
	for i := 0; i < 2; i++ {
		if _, err := svc.Submit(context.Background(), jm, round.ID, teamID, nil, raw); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	_, err := svc.Submit(context.Background(), jm, round.ID, teamID, fullBatch(round), "")
	if !domain.IsConflict(err) || !strings.Contains(err.Error(), "WIP_LIMIT_REACHED: 2 of 2 batches waiting") {
		t.Fatalf("want WIP_LIMIT_REACHED conflict, got %v", err)
	}
}

func TestSubmitIgnoresWIPLimitInClassic(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	round.MarketMode = domain.MarketClassic
	round.WIPLimit = 1
	st.Rounds[round.ID] = round
	jm, teamID := seedJokeMaker(t, st)
	svc := NewBatchService(st, slog.Default())
	jokes := fullBatch(round)
	for i := 0; i < 3; i++ {
		if _, err := svc.Submit(context.Background(), jm, round.ID, teamID, jokes, ""); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
}

type recordingEvaluator struct{ served, evaluated int }

func (r *recordingEvaluator) EvaluatePurchases(context.Context, int64, []int64) error {
	r.evaluated++
	return nil
}

func (r *recordingEvaluator) ServeArrivals(context.Context, int64, time.Time) error {
	r.served++
	return nil
}

func TestClassificationServesArrivalsInLeanInsteadOfEvaluating(t *testing.T) {
	rec := &recordingEvaluator{}
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	svc := NewClassificationService(st, llm.StubClassifier{}, rec, "stub", slog.Default())
	batchID := seedPublishedBatchAwaitingClassification(t, st, round.ID)
	if err := svc.ProcessBatch(context.Background(), batchID); err != nil {
		t.Fatal(err)
	}
	if rec.served != 1 || rec.evaluated != 0 {
		t.Fatalf("lean: served=%d evaluated=%d", rec.served, rec.evaluated)
	}
}

func TestClassificationEvaluatesPurchasesInClassic(t *testing.T) {
	rec := &recordingEvaluator{}
	st := testutil.NewStore()
	round, _, _ := leanRound(t, st)
	round.MarketMode = domain.MarketClassic
	st.Rounds[round.ID] = round
	svc := NewClassificationService(st, llm.StubClassifier{}, rec, "stub", slog.Default())
	batchID := seedPublishedBatchAwaitingClassification(t, st, round.ID)
	if err := svc.ProcessBatch(context.Background(), batchID); err != nil {
		t.Fatal(err)
	}
	if rec.served != 0 || rec.evaluated != 1 {
		t.Fatalf("classic: served=%d evaluated=%d", rec.served, rec.evaluated)
	}
}

// leanRoundAllDue seeds a Lean round whose customers have all arrived.
func leanRoundAllDue(t *testing.T, st *testutil.Store) (*domain.Round, *AICustomerService, int64) {
	t.Helper()
	round, hi, _ := leanRound(t, st)
	svc := NewAICustomerService(st, rand.New(rand.NewSource(1)), slog.Default())
	if err := svc.GenerateCustomers(context.Background(), round); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	round.StartedAt = &past
	st.Rounds[round.ID] = round
	for _, c := range mustList(t, st, round.ID) {
		at := past.Add(time.Second)
		c.ArrivesAt = &at
		st.AICustomers[c.ID] = &c
	}
	return round, svc, hi
}

func TestMarketReadServesArrivals(t *testing.T) {
	st := testutil.NewStore()
	round, svc, hi := leanRoundAllDue(t, st)
	viewer := seedAnyUser(t, st)
	if _, err := svc.Market(context.Background(), viewer, round.ID); err != nil {
		t.Fatal(err)
	}
	if soldCount(t, st, hi) == 0 {
		t.Fatal("reading the market did not serve due customers")
	}
}

func TestTeamSummaryReadServesArrivals(t *testing.T) {
	st := testutil.NewStore()
	round, svc, _ := leanRoundAllDue(t, st)
	rs := NewRoundServiceWithArrivals(st, svc, slog.Default())
	if _, err := rs.TeamSummary(context.Background(), round.ID, 1); err != nil {
		t.Fatal(err)
	}
	if n := countServed(t, st, round.ID); n != 10 {
		t.Fatalf("team summary read served %d of 10", n)
	}
}

func TestInstructorStatsReadServesArrivals(t *testing.T) {
	st := testutil.NewStore()
	round, svc, _ := leanRoundAllDue(t, st)
	is := NewInstructorService(st, svc, slog.Default())
	if _, err := is.Stats(context.Background(), round.ID); err != nil {
		t.Fatal(err)
	}
	if n := countServed(t, st, round.ID); n != 10 {
		t.Fatalf("instructor stats read served %d of 10", n)
	}
}

func TestStatsReadWithoutArrivalsServesNothing(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRoundAllDue(t, st)
	rs := NewRoundService(st, slog.Default())
	if _, err := rs.TeamSummary(context.Background(), round.ID, 1); err != nil {
		t.Fatal(err)
	}
	is := NewInstructorService(st, nil, slog.Default())
	if _, err := is.Stats(context.Background(), round.ID); err != nil {
		t.Fatal(err)
	}
	if n := countServed(t, st, round.ID); n != 0 {
		t.Fatalf("served %d without an arrivals server", n)
	}
}

// failingClaimStore fails ClaimDueCustomers and counts the calls.
type failingClaimStore struct {
	*testutil.Store
	claims *int
}

func (s failingClaimStore) ClaimDueCustomers(context.Context, int64, time.Time) ([]domain.AICustomer, error) {
	*s.claims++
	return nil, errors.New("claim failed")
}

// Customers who arrived after the last poll must still shop before the round
// ends: ServeArrivals is a no-op once the round is ENDED.
func TestEndRoundServesDueArrivalsFirst(t *testing.T) {
	st := testutil.NewStore()
	round, svc, _ := leanRoundAllDue(t, st)
	is := NewInstructorService(st, svc, slog.Default())
	ended, err := is.EndRound(context.Background(), round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != domain.RoundEnded {
		t.Fatalf("status = %s, want ENDED", ended.Status)
	}
	if n := countServed(t, st, round.ID); n != 10 {
		t.Fatalf("end round served %d of 10", n)
	}
}

func TestEndRoundEndsEvenWhenServingFails(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRoundAllDue(t, st)
	claims := 0
	fs := failingClaimStore{Store: st, claims: &claims}
	svc := NewAICustomerService(fs, rand.New(rand.NewSource(1)), slog.Default())
	is := NewInstructorService(fs, svc, slog.Default())
	ended, err := is.EndRound(context.Background(), round.ID)
	if err != nil {
		t.Fatalf("a failed serve blocked end round: %v", err)
	}
	if ended.Status != domain.RoundEnded || claims != 1 {
		t.Fatalf("status=%s claims=%d, want ENDED after one serve attempt", ended.Status, claims)
	}
}

func TestEndRoundDoesNotServeInClassic(t *testing.T) {
	st := testutil.NewStore()
	round, _, _ := leanRoundAllDue(t, st)
	round.MarketMode = domain.MarketClassic
	st.Rounds[round.ID] = round
	claims := 0
	fs := failingClaimStore{Store: st, claims: &claims}
	svc := NewAICustomerService(fs, rand.New(rand.NewSource(1)), slog.Default())
	is := NewInstructorService(fs, svc, slog.Default())
	ended, err := is.EndRound(context.Background(), round.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != domain.RoundEnded || claims != 0 || countServed(t, st, round.ID) != 0 {
		t.Fatalf("classic end round: status=%s claims=%d served=%d", ended.Status, claims, countServed(t, st, round.ID))
	}
}
