// Package llm contains adapters implementing the ports.Classifier interface:
// an Azure AI Foundry (gpt-4o-mini) client and an offline stub for local dev.
package llm

import (
	"context"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

// StubClassifier returns deterministic categories for local/dev/tests.
// When Fixed is set for a joke ID, those categories are used; otherwise every
// LLM dimension is filled with the first non-catch-all category from Specs.
type StubClassifier struct {
	Fixed map[int64]map[domain.Dimension]string
	Err   error
}

// Classify implements ports.Classifier.
func (s StubClassifier) Classify(_ context.Context, jokes []ports.JokeInput) ([]ports.JokeClassification, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]ports.JokeClassification, 0, len(jokes))
	for _, j := range jokes {
		cats := make(map[domain.Dimension]string, len(scoring.LLMDimensions()))
		if fixed, ok := s.Fixed[j.JokeID]; ok {
			for dim, cat := range fixed {
				cats[dim] = cat
			}
		} else {
			for _, dim := range scoring.LLMDimensions() {
				cats[dim] = defaultCategory(dim)
			}
		}
		out = append(out, ports.JokeClassification{JokeID: j.JokeID, Categories: cats})
	}
	return out, nil
}

func defaultCategory(dim domain.Dimension) string {
	spec := scoring.Specs[dim]
	for _, c := range spec.Categories {
		if !scoring.IsCatchAll(c) {
			return c
		}
	}
	if len(spec.Categories) > 0 {
		return spec.Categories[0]
	}
	return ""
}
