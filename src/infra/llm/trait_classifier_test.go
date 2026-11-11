package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

func corpusJokes(t *testing.T) traitCorpusFile {
	t.Helper()
	var f traitCorpusFile
	if err := json.Unmarshal(traitCorpusJSON, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTraitClassifierLoadsTheCorpus(t *testing.T) {
	c, err := NewTraitClassifier()
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 1000 {
		t.Fatalf("corpus has %d jokes; want 1000", c.Len())
	}
}

// A corpus joke gets exactly its corpus categories for the LLM dimensions,
// Title Fit "Strong", whatever its spacing; Length stays the code's job.
func TestTraitClassifierGivesCorpusJokesTheirTraits(t *testing.T) {
	c, err := NewTraitClassifier()
	if err != nil {
		t.Fatal(err)
	}
	f := corpusJokes(t)
	first, last := f.Jokes[0], f.Jokes[len(f.Jokes)-1]
	out, err := c.Classify(context.Background(), []ports.JokeInput{
		{JokeID: 7, Text: first.Text, Title: "anything"},
		{JokeID: 8, Text: "  " + strings.ReplaceAll(last.Text, " ", "\n  ") + "\t"},
	})
	if err != nil || len(out) != 2 {
		t.Fatal(err, len(out))
	}
	for i, j := range []struct {
		id     int64
		traits map[domain.Dimension]string
	}{{7, first.Traits}, {8, last.Traits}} {
		got := out[i]
		if got.JokeID != j.id {
			t.Fatalf("result %d has joke id %d; want %d", i, got.JokeID, j.id)
		}
		if len(got.Categories) != len(scoring.LLMDimensions()) {
			t.Fatalf("joke %d: %d categories; want %d", j.id, len(got.Categories), len(scoring.LLMDimensions()))
		}
		if _, ok := got.Categories[domain.DimLength]; ok {
			t.Fatalf("joke %d: the classifier returned Length", j.id)
		}
		for _, dim := range scoring.LLMDimensions() {
			want := j.traits[dim]
			if dim == domain.DimTitleFit {
				want = "Strong"
			}
			if got.Categories[dim] != want {
				t.Errorf("joke %d %s = %q; want %q", j.id, dim, got.Categories[dim], want)
			}
		}
	}
}

// Text outside the corpus is graded exactly as the hash classifier grades it.
func TestTraitClassifierFallsBackToHashForOtherText(t *testing.T) {
	c, err := NewTraitClassifier()
	if err != nil {
		t.Fatal(err)
	}
	in := []ports.JokeInput{{JokeID: 3, Text: "My password is the last eight digits of pi."}}
	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := HashClassifier{}.Classify(context.Background(), in)
	for _, dim := range scoring.LLMDimensions() {
		if got[0].Categories[dim] != want[0].Categories[dim] || got[0].JokeID != 3 {
			t.Fatalf("%s = %q; want the hash classifier's %q", dim, got[0].Categories[dim], want[0].Categories[dim])
		}
	}
}

func TestTraitClassifierRefusesABadCorpus(t *testing.T) {
	good := `{"id":"T1","title":"t","text":"one two three four five six seven","traits":{"LENGTH":"Short","TOPIC":"Work","HUMOR_STYLE":"Pun","COMPLEXITY":"Simple","EDGINESS":"Clean","STRUCTURE":"One-liner","WORDPLAY":"None","FRESHNESS":"Timeless","SETUP_PAYOFF":"Quick","CLARITY":"Crystal clear","ENERGY":"Low"}}`
	if _, err := newTraitClassifier([]byte(`{"version":1,"seed":"s","jokes":[` + good + `]}`)); err != nil {
		t.Fatalf("a good one-joke corpus: %v", err)
	}
	for name, raw := range map[string]string{
		"wrong version": `{"version":2,"seed":"s","jokes":[` + good + `]}`,
		"no jokes":      `{"version":1,"seed":"s","jokes":[]}`,
		"repeated text": `{"version":1,"seed":"s","jokes":[` + good + `,` + strings.Replace(good, `"T1"`, `"T2"`, 1) + `]}`,
		"catch-all":     `{"version":1,"seed":"s","jokes":[` + strings.Replace(good, `"Low"`, `"None of the above"`, 1) + `]}`,
		"unknown topic": `{"version":1,"seed":"s","jokes":[` + strings.Replace(good, `"Work"`, `"Weather"`, 1) + `]}`,
		"wrong length":  `{"version":1,"seed":"s","jokes":[` + strings.Replace(good, `"Short"`, `"Long"`, 1) + `]}`,
		"missing trait": `{"version":1,"seed":"s","jokes":[` + strings.Replace(good, `,"ENERGY":"Low"`, ``, 1) + `]}`,
	} {
		if _, err := newTraitClassifier([]byte(raw)); err == nil {
			t.Errorf("%s: accepted; want an error", name)
		}
	}
}

// The corpus scores as the pilot expects against PilotIdeal: most jokes far
// from the customers' bar of 8, a few (2–6 %) over it.
func TestTraitCorpusFitAgainstThePilotIdeal(t *testing.T) {
	c, err := NewTraitClassifier()
	if err != nil {
		t.Fatal(err)
	}
	f := corpusJokes(t)
	in := make([]ports.JokeInput, len(f.Jokes))
	for i, j := range f.Jokes {
		in[i] = ports.JokeInput{JokeID: int64(i + 1), Text: j.Text}
	}
	out, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	over := 0
	for i, r := range out {
		cls := scoring.Classification{domain.DimLength: scoring.ClassifyLength(f.Jokes[i].Text)}
		for dim, cat := range r.Categories {
			cls[dim] = cat
		}
		if scoring.TrueFit(cls, scoring.IdealProfile(PilotIdeal)) >= 8 {
			over++
		}
	}
	if share := float64(over) / float64(len(out)); share < 0.02 || share > 0.06 {
		t.Fatalf("%d of %d jokes clear 8; want 2–6 %%", over, len(out))
	}
}
