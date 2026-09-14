package usecase_test

import (
	"context"
	"testing"

	"jokefactory/src/core/domain"
)

func TestUnsplitRestoresTheOriginalBlobExactly(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	// Split into texts that deliberately lose the blob's numbering and blank
	// lines, so a re-join of the split texts cannot pass this test.
	if _, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID,
		[]string{"first", "second", "third"}); err != nil {
		t.Fatalf("split: %v", err)
	}

	res, err := f.marketing.Unsplit(ctx, f.mkt.ID, batch.ID)
	if err != nil {
		t.Fatalf("unsplit: %v", err)
	}
	if len(res.Jokes) != 0 {
		t.Fatalf("unsplit left %d joke rows, want 0", len(res.Jokes))
	}
	if res.Batch.RawText == nil {
		t.Fatal("raw_text is nil after unsplit, want the original blob restored")
	}
	if *res.Batch.RawText != rawBlob {
		t.Fatalf("raw_text = %q, want it byte-identical to the submitted blob %q",
			*res.Batch.RawText, rawBlob)
	}
	if res.Batch.RawTextOriginal == nil || *res.Batch.RawTextOriginal != rawBlob {
		t.Fatalf("raw_text_original = %v, want it still intact", res.Batch.RawTextOriginal)
	}
	if res.QueueSize != 1 {
		t.Fatalf("queue_size = %d, want 1", res.QueueSize)
	}
}

func TestUnsplitFallsBackToJoiningLegacyJokes(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)

	// A legacy jokes-array submission has no original blob to restore.
	batch, err := f.batches.Submit(ctx, f.jm.ID, f.roundID, *f.jm.TeamID,
		[]string{"alpha", "beta", "gamma"}, "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	res, err := f.marketing.Unsplit(ctx, f.mkt.ID, batch.ID)
	if err != nil {
		t.Fatalf("unsplit: %v", err)
	}
	const want = "alpha\n\nbeta\n\ngamma"
	if res.Batch.RawText == nil || *res.Batch.RawText != want {
		t.Fatalf("raw_text = %v, want the joke texts re-joined as %q", res.Batch.RawText, want)
	}
	if len(res.Jokes) != 0 {
		t.Fatalf("unsplit left %d joke rows, want 0", len(res.Jokes))
	}
}

func TestUnsplitRefusesDecidedJokes(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch, err := f.batches.Submit(ctx, f.jm.ID, f.roundID, *f.jm.TeamID,
		[]string{"a", "b", "c"}, "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	f.store.Batches[batch.ID].Jokes[0].PublishStatus = domain.PublishDiscarded
	if _, err := f.marketing.Unsplit(ctx, f.mkt.ID, batch.ID); err == nil || !domain.IsConflict(err) {
		t.Fatalf("decided joke: expected conflict, got %v", err)
	}

	f.store.Batches[batch.ID].Jokes[0].PublishStatus = domain.PublishPending
	f.store.Batches[batch.ID].Status = domain.BatchProcessed
	if _, err := f.marketing.Unsplit(ctx, f.mkt.ID, batch.ID); err == nil || !domain.IsConflict(err) {
		t.Fatalf("processed batch: expected conflict, got %v", err)
	}
}

func TestUnsplitRequiresTheLock(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)

	if _, err := f.marketing.Unsplit(ctx, f.mkt.ID, batch.ID); err == nil || !domain.IsForbidden(err) {
		t.Fatalf("unclaimed batch: expected forbidden, got %v", err)
	}
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}
	if _, err := f.marketing.Unsplit(ctx, f.other.ID, batch.ID); err == nil || !domain.IsForbidden(err) {
		t.Fatalf("other-team marketer: expected forbidden, got %v", err)
	}
}

func TestSplitUnsplitSplitAssignsFreshJokeIDs(t *testing.T) {
	ctx := context.Background()
	f := newSplitFixture(t, 1, 3)
	batch := f.submitRaw(t)
	if _, err := f.marketing.QueueNext(ctx, f.mkt.ID, f.roundID); err != nil {
		t.Fatalf("queue next: %v", err)
	}

	texts := []string{"first", "second", "third"}
	first, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, texts)
	if err != nil {
		t.Fatalf("first split: %v", err)
	}
	firstIDs := make(map[int64]struct{}, len(first.Jokes))
	for _, j := range first.Jokes {
		firstIDs[j.ID] = struct{}{}
	}

	if _, err := f.marketing.Unsplit(ctx, f.mkt.ID, batch.ID); err != nil {
		t.Fatalf("unsplit: %v", err)
	}

	second, err := f.marketing.Split(ctx, f.mkt.ID, batch.ID, texts)
	if err != nil {
		t.Fatalf("second split: %v", err)
	}
	if len(second.Jokes) != len(texts) {
		t.Fatalf("second split produced %d jokes, want %d", len(second.Jokes), len(texts))
	}
	for _, j := range second.Jokes {
		if _, reused := firstIDs[j.ID]; reused {
			t.Fatalf("joke id %d was reused across splits; the frontend keys selection on joke_id", j.ID)
		}
	}
	if second.Batch.RawText != nil {
		t.Fatalf("raw_text = %q after the second split, want nil", *second.Batch.RawText)
	}
}
