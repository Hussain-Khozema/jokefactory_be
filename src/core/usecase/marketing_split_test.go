package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/worker"
)

// rawBlob is an unsplit Joke Maker submission. The formatting is deliberately
// distinctive (numbering plus blank lines) so tests can assert it survives
// byte-for-byte in raw_text_original.
const rawBlob = "1. Why did the developer cross the road?\n\n2. To get to the other IDE.\n\n3. Nobody knows, really."

type splitFixture struct {
	store     *testutil.Store
	batches   *usecase.BatchService
	marketing *usecase.MarketingService
	jm        *domain.User
	mkt       *domain.User
	other     *domain.User
	roundID   int64
}

// newSplitFixture builds an ACTIVE round with two teams, each with one JM and
// one marketer. roundID doubles as the round number in the in-memory store, so
// pass 1 for R1 rules and 2 for R2 rules.
func newSplitFixture(t *testing.T, roundID int64, batchSize int) *splitFixture {
	t.Helper()
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = batchSize
	if _, err := store.InsertRoundConfig(ctx, roundID, &defaults); err != nil {
		t.Fatalf("insert round config: %v", err)
	}

	users := joinMany(t, session, []string{"JM1", "Mkt1", "JM2", "Mkt2"})
	if _, err := instructor.Assign(ctx, roundID, 2); err != nil {
		t.Fatalf("assign: %v", err)
	}
	cfg := defaults
	if _, err := instructor.Config(ctx, roundID, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := instructor.StartRound(ctx, roundID); err != nil {
		t.Fatalf("start: %v", err)
	}

	jm := findJM(t, session, users)
	return &splitFixture{
		store:     store,
		batches:   batches,
		marketing: marketing,
		jm:        jm,
		mkt:       findMarketingOnTeam(t, session, users, *jm.TeamID),
		other:     findOtherTeamMarketing(t, session, users, *jm.TeamID),
		roundID:   roundID,
	}
}

func (f *splitFixture) submitRaw(t *testing.T) *domain.Batch {
	t.Helper()
	b, err := f.batches.Submit(context.Background(), f.jm.ID, f.roundID, *f.jm.TeamID, nil, rawBlob)
	if err != nil {
		t.Fatalf("submit raw: %v", err)
	}
	return b
}

func TestQueueNextExposesRawTextAndNoJokes(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)

	item, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID)
	if err != nil {
		t.Fatalf("queue next: %v", err)
	}
	if item.Batch.ID != batch.ID {
		t.Fatalf("claimed batch %d, want %d", item.Batch.ID, batch.ID)
	}
	if item.Batch.RawText == nil || *item.Batch.RawText != rawBlob {
		t.Fatalf("raw_text = %v, want the submitted blob", item.Batch.RawText)
	}
	if len(item.Jokes) != 0 {
		t.Fatalf("unsplit batch returned %d jokes, want 0", len(item.Jokes))
	}
}

func TestSplitCreatesJokesAndClearsRawText(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	texts := []string{
		"Why did the developer cross the road?",
		"To get to the other IDE.",
		"Nobody knows, really.",
	}
	res, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, texts)
	if err != nil {
		t.Fatalf("split: %v", err)
	}

	if res.Batch.RawText != nil {
		t.Fatalf("raw_text = %q after split, want nil", *res.Batch.RawText)
	}
	if res.Batch.RawTextOriginal == nil || *res.Batch.RawTextOriginal != rawBlob {
		t.Fatalf("raw_text_original = %v, want the submitted blob preserved", res.Batch.RawTextOriginal)
	}
	if len(res.Jokes) != len(texts) {
		t.Fatalf("split produced %d jokes, want %d", len(res.Jokes), len(texts))
	}
	seen := make(map[int64]struct{}, len(res.Jokes))
	for i, j := range res.Jokes {
		if j.ID == 0 {
			t.Fatalf("joke %d has no id", i)
		}
		if _, dup := seen[j.ID]; dup {
			t.Fatalf("duplicate joke id %d", j.ID)
		}
		seen[j.ID] = struct{}{}
		if j.Text != texts[i] {
			t.Fatalf("joke %d text = %q, want %q", i, j.Text, texts[i])
		}
		if j.PublishStatus != domain.PublishPending {
			t.Fatalf("joke %d status = %s, want PENDING", i, j.PublishStatus)
		}
	}
	if res.QueueSize != 1 {
		t.Fatalf("queue_size = %d, want 1 (batch is still SUBMITTED)", res.QueueSize)
	}

	// The queue must now show the split state and never both states at once.
	again, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID)
	if err != nil {
		t.Fatalf("queue next after split: %v", err)
	}
	if again.Batch.RawText != nil {
		t.Fatalf("queue/next still exposes raw_text after split")
	}
	if len(again.Jokes) != len(texts) {
		t.Fatalf("queue/next returned %d jokes after split, want %d", len(again.Jokes), len(texts))
	}
}

func TestSplitRoundOneRequiresExactBatchSize(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	for _, tc := range [][]string{{"one", "two"}, {"one", "two", "three", "four"}} {
		_, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, tc)
		if err == nil || !domain.IsValidationError(err) {
			t.Fatalf("split with %d jokes: expected validation error, got %v", len(tc), err)
		}
		if !strings.Contains(err.Error(), "expected 3 jokes") {
			t.Fatalf("split with %d jokes: error = %q, want it to mention \"expected 3 jokes\"", len(tc), err.Error())
		}
	}

	if _, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"one", "two", "three"}); err != nil {
		t.Fatalf("split with exactly 3: %v", err)
	}
}

func TestSplitRoundTwoCapsAtBatchSize(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 2, 3)
	batch := f.submitRaw(t)
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	_, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"a", "b", "c", "d"})
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("over the cap: expected validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "expected up to 3 jokes") {
		t.Fatalf("over the cap: error = %q, want it to mention \"expected up to 3 jokes\"", err.Error())
	}

	// Fewer than batch_size is allowed in R2, and blank entries are dropped.
	res, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"  first  ", "   ", "second"})
	if err != nil {
		t.Fatalf("split under the cap: %v", err)
	}
	if len(res.Jokes) != 2 {
		t.Fatalf("got %d jokes, want 2 after dropping the blank", len(res.Jokes))
	}
	if res.Jokes[0].Text != "first" {
		t.Fatalf("joke text = %q, want it trimmed to %q", res.Jokes[0].Text, "first")
	}

	if _, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"   ", ""}); err == nil ||
		!domain.IsValidationError(err) {
		t.Fatalf("all-blank split: expected validation error, got %v", err)
	}
}

func TestSplitRequiresTheLock(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)

	// Never claimed: locked_by is NULL.
	_, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"a", "b", "c"})
	if err == nil || !domain.IsForbidden(err) {
		t.Fatalf("unclaimed batch: expected forbidden, got %v", err)
	}

	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	// A marketer on another team may not split it either.
	if _, err := f.marketing.Split(ctx, f.other.ID, batch.ID, []string{"a", "b", "c"}); err == nil ||
		!domain.IsForbidden(err) {
		t.Fatalf("other-team marketer: expected forbidden, got %v", err)
	}

	// The lock is held by somebody else (an expired lock must not let a second
	// marketer overwrite the first marketer's work).
	otherID := f.mkt.ID + 1000
	f.store.Batches[batch.ID].LockedBy = &otherID
	if _, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"a", "b", "c"}); err == nil ||
		!domain.IsForbidden(err) {
		t.Fatalf("lock held elsewhere: expected forbidden, got %v", err)
	}
}

func TestSplitRefusesDecidedJokes(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)

	batch, err := f.batches.Submit(ctx, f.jm.ID, f.roundID, *f.jm.TeamID, []string{"a", "b", "c"}, "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	// One joke already decided while the batch is still SUBMITTED: re-splitting
	// would destroy that decision.
	f.store.Batches[batch.ID].Jokes[0].PublishStatus = domain.PublishPublished
	if _, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"x", "y", "z"}); err == nil ||
		!domain.IsConflict(err) {
		t.Fatalf("decided joke: expected conflict, got %v", err)
	}

	// A PROCESSED batch is a conflict too.
	f.store.Batches[batch.ID].Jokes[0].PublishStatus = domain.PublishPending
	f.store.Batches[batch.ID].Status = domain.BatchProcessed
	if _, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, []string{"x", "y", "z"}); err == nil ||
		!domain.IsConflict(err) {
		t.Fatalf("processed batch: expected conflict, got %v", err)
	}
}
