package llm

import (
	"context"
	"hash/fnv"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
	"jokefactory/src/core/ports"
)

// PilotIdeal is the ideal profile HashClassifier is aligned to. It equals the
// frontend's DEFAULT_IDEAL_PROFILE, which the pilot sends as the round's ideal.
var PilotIdeal = domain.IdealProfile{
	domain.DimLength:      scoring.LengthMedium,
	domain.DimTopic:       "Work",
	domain.DimHumorStyle:  "Observational",
	domain.DimComplexity:  "Moderate",
	domain.DimEdginess:    "Clean",
	domain.DimStructure:   "Setup–punchline",
	domain.DimWordplay:    "Light",
	domain.DimFreshness:   "Timeless",
	domain.DimSetupPayoff: "Balanced",
	domain.DimClarity:     "Crystal clear",
	domain.DimEnergy:      "Conversational",
}

// The hidden quality of a joke is drawn uniformly from [hashQualityMin,
// hashQualityMax]. Tuned so that over the frontend's 60-joke corpus the fit
// spreads from ~2 to ~10.5 and about 40% of jokes clear the AI customers' buy bar
// (~7); see TestHashClassifierSpreadsCorpusFit.
const (
	hashQualityMin = 0.05
	hashQualityMax = 0.70
)

// titleFitGood is the Title Fit category used for a 'good' outcome, since Title
// Fit has no ideal and is graded intrinsically.
const titleFitGood = "Perfect"

// HashClassifier is for LOCAL PILOTS ONLY (APP_LLM_MODE=hash plus
// APP_LLM_HASH_CONFIRM=local-pilot-only). It knows nothing
// about humour. Each joke gets a hidden "quality" q derived from a hash of its
// text; for every LLM dimension, a second hash of text|dimension decides with
// probability q whether the joke lands on PilotIdeal's category (Perfect for
// Title Fit) or on some other hashed, non-catch-all category. The same text
// always gets the same categories.
//
// Because the "good" outcome is PilotIdeal, fit is only meaningful when the
// round's ideal profile is PilotIdeal (the frontend's DEFAULT_IDEAL_PROFILE);
// pilots must use that profile. Unlike StubClassifier, which gives every joke
// identical categories, this spreads a corpus across the fit range so
// fit-weighted buying can be exercised.
type HashClassifier struct{}

// Classify implements ports.Classifier.
func (HashClassifier) Classify(_ context.Context, jokes []ports.JokeInput) ([]ports.JokeClassification, error) {
	out := make([]ports.JokeClassification, 0, len(jokes))
	for _, j := range jokes {
		q := hashQuality(j.Text)
		cats := make(map[domain.Dimension]string, len(scoring.LLMDimensions()))
		for _, dim := range scoring.LLMDimensions() {
			cats[dim] = hashedCategory(j.Text, dim, q)
		}
		out = append(out, ports.JokeClassification{JokeID: j.JokeID, Categories: cats})
	}
	return out, nil
}

// hashQuality maps text to a quality in [hashQualityMin, hashQualityMax).
func hashQuality(text string) float64 {
	return hashQualityMin + (hashQualityMax-hashQualityMin)*unitHash("quality|"+text)
}

func hashedCategory(text string, dim domain.Dimension, q float64) string {
	good := goodCategory(dim)
	if good != "" && unitHash("hit|"+string(dim)+"|"+text) < q {
		return good
	}
	var options []string
	for _, c := range scoring.Specs[dim].Categories {
		if !scoring.IsCatchAll(c) && c != good {
			options = append(options, c)
		}
	}
	if len(options) == 0 {
		if good != "" {
			return good
		}
		return defaultCategory(dim)
	}
	idx := int(unitHash("pick|"+string(dim)+"|"+text) * float64(len(options)))
	return options[idx]
}

func goodCategory(dim domain.Dimension) string {
	if dim == domain.DimTitleFit {
		return titleFitGood
	}
	return PilotIdeal[dim]
}

// unitHash maps s to [0, 1) using the top 53 bits of FNV-1a 64.
func unitHash(s string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return float64(h.Sum64()>>11) / float64(uint64(1)<<53)
}
