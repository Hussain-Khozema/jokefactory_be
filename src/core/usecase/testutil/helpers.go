// Package testutil provides an in-memory Store implementation for usecase tests.
package testutil

import (
	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
)

func ValidIdealProfile() domain.IdealProfile {
	return domain.IdealProfile{
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
}

func IdealLLMCats(profile domain.IdealProfile) map[domain.Dimension]string {
	return map[domain.Dimension]string{
		domain.DimTopic:       profile[domain.DimTopic],
		domain.DimHumorStyle:  profile[domain.DimHumorStyle],
		domain.DimComplexity:  profile[domain.DimComplexity],
		domain.DimEdginess:    profile[domain.DimEdginess],
		domain.DimStructure:   profile[domain.DimStructure],
		domain.DimWordplay:    profile[domain.DimWordplay],
		domain.DimFreshness:   profile[domain.DimFreshness],
		domain.DimSetupPayoff: profile[domain.DimSetupPayoff],
		domain.DimClarity:     profile[domain.DimClarity],
		domain.DimEnergy:      profile[domain.DimEnergy],
		domain.DimTitleFit:    "Perfect",
	}
}

func PurchaseCount(store *Store, jokeID int64) int {
	n := 0
	for _, p := range store.Purchases {
		if p.JokeID == jokeID {
			n++
		}
	}
	return n
}
