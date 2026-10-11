package usecase_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
	"jokefactory/src/core/usecase/testutil"
	"jokefactory/src/infra/worker"
)

// noteFixture is a claimed two-joke batch on an ACTIVE Round 2 (the in-memory
// store uses the round id as the round number), so both a release and a
// whole-batch discard are legal publishes.
type noteFixture struct {
	store     *testutil.Store
	marketing *usecase.MarketingService
	batches   *usecase.BatchService
	jm        *domain.User
	mkt       *domain.User
	batchID   int64
	jokeIDs   []int64
}

const noteRound int64 = 2

func newNoteFixture(t *testing.T) *noteFixture {
	t.Helper()
	ctx := context.Background()
	store := testutil.NewStore()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	session := usecase.NewSessionService(store, log)
	instructor := usecase.NewInstructorService(store, nil, log)
	batches := usecase.NewBatchService(store, log)
	marketing := usecase.NewMarketingService(store, worker.NoopDispatcher{}, log)

	defaults := domain.DefaultRoundConfig()
	defaults.BatchSize = 2
	if _, err := store.InsertRoundConfig(ctx, noteRound, &defaults); err != nil {
		t.Fatal(err)
	}
	users := joinMany(t, session, []string{"A", "B", "C", "D"})
	if _, err := instructor.Assign(ctx, noteRound, 2); err != nil {
		t.Fatal(err)
	}
	cfg := defaults
	if _, err := instructor.Config(ctx, noteRound, &cfg, testutil.ValidIdealProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := instructor.StartRound(ctx, noteRound); err != nil {
		t.Fatal(err)
	}
	jm := findJM(t, session, users)
	mkt := findMarketingOnTeam(t, session, users, *jm.TeamID)
	batch, err := batches.Submit(ctx, jm.ID, noteRound, *jm.TeamID, []string{"j1", "j2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := marketing.QueueNext(ctx, mkt.ID, noteRound)
	if err != nil {
		t.Fatal(err)
	}
	return &noteFixture{
		store: store, marketing: marketing, batches: batches, jm: jm, mkt: mkt,
		batchID: batch.ID, jokeIDs: []int64{item.Jokes[0].ID, item.Jokes[1].ID},
	}
}

// release publishes the first joke and discards the second; discardAll
// discards both (the whole-batch discard).
func (f *noteFixture) decisions(release bool) []ports.JokePublishDecision {
	return []ports.JokePublishDecision{
		{JokeID: f.jokeIDs[0], Title: "Kept", IsPublished: release},
		{JokeID: f.jokeIDs[1], Title: "", IsPublished: false},
	}
}

func (f *noteFixture) publish(t *testing.T, release bool, note *string) (*ports.PublishResult, error) {
	t.Helper()
	return f.marketing.PublishWithNote(context.Background(), f.mkt.ID, f.batchID, f.decisions(release), note)
}

// listedNote is what the Joke Maker's batch list returns for the batch.
func (f *noteFixture) listedNote(t *testing.T) *string {
	t.Helper()
	listed, err := f.batches.List(context.Background(), noteRound, *f.jm.TeamID, f.jm.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, b := range listed {
		if b.ID == f.batchID {
			return b.MarketingNote
		}
	}
	t.Fatalf("batch %d not listed", f.batchID)
	return nil
}

func strPtr(s string) *string { return &s }

func TestPublishStoresMarketingNote(t *testing.T) {
	for _, tc := range []struct {
		name    string
		release bool
	}{{"release", true}, {"whole-batch discard", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNoteFixture(t)
			result, err := f.publish(t, tc.release, strPtr("  Shorter setups, please.\nMore puns.  "))
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			const want = "Shorter setups, please.\nMore puns."
			if result.Batch.MarketingNote == nil || *result.Batch.MarketingNote != want {
				t.Fatalf("result note = %v; want %q", result.Batch.MarketingNote, want)
			}
			if got := f.listedNote(t); got == nil || *got != want {
				t.Fatalf("listed note = %v; want %q", got, want)
			}
		})
	}
}

func TestPublishStoresNullForAbsentOrBlankNote(t *testing.T) {
	for _, tc := range []struct {
		name string
		note *string
	}{{"absent", nil}, {"empty", strPtr("")}, {"whitespace only", strPtr("  \n\t  ")}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNoteFixture(t)
			if _, err := f.publish(t, true, tc.note); err != nil {
				t.Fatalf("publish: %v", err)
			}
			if got := f.listedNote(t); got != nil {
				t.Fatalf("listed note = %q; want nil", *got)
			}
		})
	}
}

func TestPublishWithoutNoteKeepsTheOldEntryPoint(t *testing.T) {
	f := newNoteFixture(t)
	if _, err := f.marketing.Publish(context.Background(), f.mkt.ID, f.batchID, f.decisions(true)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := f.listedNote(t); got != nil {
		t.Fatalf("listed note = %q; want nil", *got)
	}
}

func TestPublishRejectsNoteOver500Runes(t *testing.T) {
	f := newNoteFixture(t)
	// 501 emoji: 501 runes but 2004 bytes, so a byte count would be wrong twice over.
	_, err := f.publish(t, true, strPtr(strings.Repeat("😀", 501)))
	if err == nil || !domain.IsValidationError(err) {
		t.Fatalf("501 runes: err = %v; want a validation error", err)
	}
	if de, ok := err.(*domain.DomainError); !ok || de.Field != "note" || de.Message != "must be at most 500 characters" {
		t.Fatalf("501 runes: err = %#v; want field note, message %q", err, "must be at most 500 characters")
	}
	// Nothing was written: the batch is still waiting, claimed, with no note.
	if b := f.store.Batches[f.batchID]; b.Status != domain.BatchSubmitted || b.MarketingNote != nil {
		t.Fatalf("after rejection: status %s note %v; want SUBMITTED and nil", b.Status, b.MarketingNote)
	}

	// Exactly 500 runes is accepted, and padding is trimmed before counting.
	result, err := f.publish(t, true, strPtr("  "+strings.Repeat("😀", 500)+"\n"))
	if err != nil {
		t.Fatalf("500 runes: %v", err)
	}
	if n := utf8.RuneCountInString(*result.Batch.MarketingNote); n != 500 {
		t.Fatalf("stored %d runes; want 500", n)
	}
}

func TestNormalizeMarketingNote(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   *string
		want *string
	}{
		{"absent", nil, nil},
		{"blank", strPtr(" \r\n "), nil},
		{"trimmed", strPtr("\tMore puns\n"), strPtr("More puns")},
		{"inner line breaks kept", strPtr("Line one\n\nLine two"), strPtr("Line one\n\nLine two")},
		{"markup kept verbatim", strPtr("<script>alert('x')</script>"), strPtr("<script>alert('x')</script>")},
		{"NUL dropped", strPtr("a\x00b"), strPtr("ab")},
		{"only NUL", strPtr("\x00"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := usecase.NormalizeMarketingNote(tc.in)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %v; want %v", got, tc.want)
			}
		})
	}
}
