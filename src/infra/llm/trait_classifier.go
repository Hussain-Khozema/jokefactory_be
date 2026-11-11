package llm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

// traitCorpusJSON is the trait corpus, a byte-identical copy of the frontend's
// scripts/traits/trait-corpus.json (generated there; trait_corpus_parity_test.go
// pins it). Embedded, so the API image needs no data file at run time.
//
//go:embed traitdata/trait-corpus.json
var traitCorpusJSON []byte

// TraitTitleFit is the Title Fit every corpus joke gets: graded, never an ideal.
const TraitTitleFit = "Strong"

// TraitCorpusVersion is the corpus file format this classifier reads.
const TraitCorpusVersion = 1

type traitCorpusFile struct {
	Version int    `json:"version"`
	Seed    string `json:"seed"`
	Jokes   []struct {
		ID     string                      `json:"id"`
		Title  string                      `json:"title"`
		Text   string                      `json:"text"`
		Traits map[domain.Dimension]string `json:"traits"`
	} `json:"jokes"`
}

// TraitClassifier is for LOCAL PILOTS ONLY (APP_LLM_MODE=traits plus
// APP_LLM_HASH_CONFIRM=local-pilot-only), like HashClassifier. A joke whose
// text (whitespace collapsed) is in the trait corpus gets that joke's
// categories, with Title Fit "Strong"; any other text falls back to
// HashClassifier. So the bots know their own jokes' traits and feedback on one
// joke says something about the next
// (docs/superpowers/specs/2026-11-10-two-costs-feedback-bots-design.md §3).
type TraitClassifier struct {
	byText   map[string]map[domain.Dimension]string
	fallback HashClassifier
}

// NewTraitClassifier parses the embedded corpus. It fails on a corpus that is
// not version 1, or whose jokes repeat a text, miss one of the 11 matched
// criteria, use a category scoring does not know (or the catch-all), or have a
// LENGTH their text does not have.
func NewTraitClassifier() (*TraitClassifier, error) {
	return newTraitClassifier(traitCorpusJSON)
}

func newTraitClassifier(raw []byte) (*TraitClassifier, error) {
	var f traitCorpusFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("trait corpus: %w", err)
	}
	if f.Version != TraitCorpusVersion || len(f.Jokes) == 0 {
		return nil, fmt.Errorf("trait corpus: version %d with %d jokes; want version %d with jokes", f.Version, len(f.Jokes), TraitCorpusVersion)
	}
	byText := make(map[string]map[domain.Dimension]string, len(f.Jokes))
	for _, j := range f.Jokes {
		key := normalizeJokeText(j.Text)
		if _, dup := byText[key]; dup || key == "" {
			return nil, fmt.Errorf("trait corpus joke %s: empty or repeated text", j.ID)
		}
		if len(j.Traits) != len(scoring.IdealDimensions()) {
			return nil, fmt.Errorf("trait corpus joke %s: %d traits; want the %d matched criteria", j.ID, len(j.Traits), len(scoring.IdealDimensions()))
		}
		for _, dim := range scoring.IdealDimensions() {
			cat := j.Traits[dim]
			if !scoring.IsValidCategory(dim, cat) || scoring.IsCatchAll(cat) {
				return nil, fmt.Errorf("trait corpus joke %s: %s %q is not one of its categories", j.ID, dim, cat)
			}
		}
		if got := scoring.ClassifyLength(j.Text); got != j.Traits[domain.DimLength] {
			return nil, fmt.Errorf("trait corpus joke %s: LENGTH %s but the text reads %s", j.ID, j.Traits[domain.DimLength], got)
		}
		byText[key] = j.Traits
	}
	return &TraitClassifier{byText: byText}, nil
}

// Len is the number of jokes in the corpus.
func (c *TraitClassifier) Len() int { return len(c.byText) }

// Classify implements ports.Classifier.
func (c *TraitClassifier) Classify(ctx context.Context, jokes []ports.JokeInput) ([]ports.JokeClassification, error) {
	out := make([]ports.JokeClassification, 0, len(jokes))
	for _, j := range jokes {
		traits, ok := c.byText[normalizeJokeText(j.Text)]
		if !ok {
			hashed, err := c.fallback.Classify(ctx, []ports.JokeInput{j})
			if err != nil {
				return nil, err
			}
			out = append(out, hashed...)
			continue
		}
		cats := make(map[domain.Dimension]string, len(scoring.LLMDimensions()))
		for _, dim := range scoring.LLMDimensions() {
			if dim == domain.DimTitleFit {
				cats[dim] = TraitTitleFit
				continue
			}
			cats[dim] = traits[dim]
		}
		out = append(out, ports.JokeClassification{JokeID: j.JokeID, Categories: cats})
	}
	return out, nil
}

// normalizeJokeText collapses runs of whitespace and trims, as the frontend's
// normalizeJokeText does, so a joke matches after a trip through a textarea.
func normalizeJokeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
