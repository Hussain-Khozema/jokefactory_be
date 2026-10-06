package llm

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

func TestHashClassifierIsDeterministicAndVaried(t *testing.T) {
	c := HashClassifier{}
	in := []ports.JokeInput{
		{JokeID: 1, Text: "My password is the last eight digits of pi."},
		{JokeID: 2, Text: "My password is the last eight digits of pi."},
		{JokeID: 3, Text: "Every product I own now has a subscription."},
	}
	out, err := c.Classify(context.Background(), in)
	if err != nil || len(out) != 3 {
		t.Fatal(err, len(out))
	}
	for i, r := range out {
		if r.JokeID != in[i].JokeID {
			t.Fatalf("result %d has joke id %d, want %d", i, r.JokeID, in[i].JokeID)
		}
	}
	for _, dim := range scoring.LLMDimensions() {
		if out[0].Categories[dim] != out[1].Categories[dim] {
			t.Fatalf("same text, different %s", dim)
		}
		for _, r := range out {
			cat := r.Categories[dim]
			if !scoring.IsValidCategory(dim, cat) {
				t.Fatalf("invalid category %q for %s", cat, dim)
			}
			if scoring.IsCatchAll(cat) {
				t.Fatalf("catch-all chosen for %s", dim)
			}
		}
	}
	differ := 0
	for _, dim := range scoring.LLMDimensions() {
		if out[0].Categories[dim] != out[2].Categories[dim] {
			differ++
		}
	}
	if differ == 0 {
		t.Fatal("different texts classified identically on every dimension")
	}
}

func TestPilotIdealIsAValidIdealProfile(t *testing.T) {
	if err := scoring.ValidateIdealProfile(PilotIdeal); err != nil {
		t.Fatal(err)
	}
}

// TestHashClassifierSpreadsCorpusFit classifies the frontend's 60-joke corpus
// the way ClassificationService does (Length from text, the rest from the
// classifier) and checks the fit against PilotIdeal is spread, with a minority
// clearing the AI customers' buy bar.
func TestHashClassifierSpreadsCorpusFit(t *testing.T) {
	raw, err := os.ReadFile("testdata/corpus.txt")
	if err != nil {
		t.Fatal(err)
	}
	var in []ports.JokeInput
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			in = append(in, ports.JokeInput{JokeID: int64(i + 1), Text: line})
		}
	}
	if len(in) != 60 {
		t.Fatalf("corpus has %d jokes, want 60", len(in))
	}
	out, err := HashClassifier{}.Classify(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	profile := scoring.IdealProfile(PilotIdeal)
	hist := make([]int, 13)
	atLeast7, max, min := 0, 0.0, 12.0
	for i, r := range out {
		cls := scoring.Classification{domain.DimLength: scoring.ClassifyLength(in[i].Text)}
		for dim, cat := range r.Categories {
			cls[dim] = cat
		}
		fit := scoring.TrueFit(cls, profile)
		hist[int(math.Floor(fit))]++
		if fit >= 7 {
			atLeast7++
		}
		max = math.Max(max, fit)
		min = math.Min(min, fit)
	}
	share := float64(atLeast7) / float64(len(out))
	for b, n := range hist {
		t.Logf("fit [%2d,%2d): %d", b, b+1, n)
	}
	t.Logf("share >= 7: %d/%d = %.2f; min %.2f; max %.2f", atLeast7, len(out), share, min, max)

	if share < 0.25 || share > 0.60 {
		t.Errorf("share of jokes with fit >= 7 is %.2f, want 0.25..0.60", share)
	}
	if max < 9 {
		t.Errorf("no joke reaches fit 9 (max %.2f)", max)
	}
	if min > 5 {
		t.Errorf("no joke at or below fit 5 (min %.2f)", min)
	}
}
