package usecase

import (
	"math/rand"
	"testing"
	"time"

	"jokefactory/src/core/ports"
)

func TestArrivalTimesSpanTheWindow(t *testing.T) {
	start := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	got := arrivalTimes(start, 300, 4, rand.New(rand.NewSource(1)))
	if len(got) != 4 {
		t.Fatalf("len %d", len(got))
	}
	// Multiset of offsets is {75,150,225,300}s regardless of shuffle order.
	seen := map[int]bool{}
	for _, at := range got {
		seen[int(at.Sub(start).Seconds())] = true
	}
	for _, want := range []int{75, 150, 225, 300} {
		if !seen[want] {
			t.Fatalf("missing offset %ds in %v", want, got)
		}
	}
}

func TestChooseBasketPrefersHigherFit(t *testing.T) {
	cands := []ports.CandidateJoke{
		{JokeID: 1, TeamID: 1, TrueFit: 7},
		{JokeID: 2, TeamID: 2, TrueFit: 11},
	}
	rng := rand.New(rand.NewSource(7))
	wins := 0
	for i := 0; i < 2000; i++ {
		if chooseBasket(cands, 1, 2, rng)[0].JokeID == 2 {
			wins++
		}
	}
	// weight 2^11 vs 2^7 = 16:1 → P(2) = 16/17 ≈ 0.94
	if wins < 1800 || wins > 1960 {
		t.Fatalf("higher fit chosen %d/2000, want ≈1880", wins)
	}
}

func TestChooseBasketIsWithoutReplacementAndBounded(t *testing.T) {
	cands := []ports.CandidateJoke{{JokeID: 1, TrueFit: 8}, {JokeID: 2, TrueFit: 8}}
	got := chooseBasket(cands, 5, 2, rand.New(rand.NewSource(3)))
	if len(got) != 2 || got[0].JokeID == got[1].JokeID {
		t.Fatalf("got %+v", got)
	}
	if cands[0].JokeID != 1 || cands[1].JokeID != 2 {
		t.Fatalf("chooseBasket mutated its input: %+v", cands)
	}
	if n := len(chooseBasket(nil, 3, 2, rand.New(rand.NewSource(3)))); n != 0 {
		t.Fatalf("empty candidates gave %d", n)
	}
}
