package scoring

import (
	"math"
	"testing"

	"jokefactory/src/core/domain"
)

func TestDimFitOrdinal(t *testing.T) {
	t.Parallel()
	dim := domain.DimComplexity // Very simple, Simple, Moderate, Thoughtful, Expert

	cases := []struct {
		name        string
		ideal, joke string
		want        float64
	}{
		{"exact", "Moderate", "Moderate", 1.0},
		{"adjacent_up", "Moderate", "Thoughtful", 0.5},
		{"adjacent_down", "Moderate", "Simple", 0.5},
		{"two_steps", "Moderate", "Expert", 0.0},
		{"far", "Very simple", "Expert", 0.0},
		{"invalid_joke", "Moderate", "Nope", 0.0},
		{"empty", "Moderate", "", 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DimFit(dim, tc.ideal, tc.joke)
			if got != tc.want {
				t.Fatalf("DimFit(%s, %q, %q) = %v, want %v", dim, tc.ideal, tc.joke, got, tc.want)
			}
		})
	}
}

func TestDimFitOrdinalCatchAll(t *testing.T) {
	t.Parallel()
	// Energy has a catch-all at the end of an otherwise ordinal list.
	dim := domain.DimEnergy

	cases := []struct {
		name        string
		ideal, joke string
		want        float64
	}{
		{"catch_vs_normal", "Conversational", CatchAll, 0.0},
		{"normal_vs_catch", CatchAll, "Conversational", 0.0},
		{"both_catch", CatchAll, CatchAll, 1.0},
		// Catch-all must NOT be treated as adjacent to High-energy.
		{"high_energy_vs_catch", "High-energy", CatchAll, 0.0},
		{"exact_normal", "Conversational", "Conversational", 1.0},
		{"adjacent_normal", "Conversational", "Animated", 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DimFit(dim, tc.ideal, tc.joke)
			if got != tc.want {
				t.Fatalf("DimFit(%s, %q, %q) = %v, want %v", dim, tc.ideal, tc.joke, got, tc.want)
			}
		})
	}
}

func TestDimFitCategorical(t *testing.T) {
	t.Parallel()
	dim := domain.DimTopic

	if got := DimFit(dim, "Work", "Work"); got != 1.0 {
		t.Fatalf("exact match = %v, want 1.0", got)
	}
	if got := DimFit(dim, "Work", "Food"); got != 0.0 {
		t.Fatalf("mismatch = %v, want 0.0", got)
	}
	if got := DimFit(domain.DimHumorStyle, "Pun", CatchAll); got != 0.0 {
		t.Fatalf("catch-all vs normal = %v, want 0.0", got)
	}
	if got := DimFit(domain.DimHumorStyle, CatchAll, CatchAll); got != 1.0 {
		t.Fatalf("both catch-all = %v, want 1.0", got)
	}
}

func TestDimFitTitleFitGraded(t *testing.T) {
	t.Parallel()
	dim := domain.DimTitleFit
	grades := map[string]float64{
		"Perfect": 1.0, "Strong": 0.75, "Moderate": 0.5, "Weak": 0.25, "Mismatch": 0.0,
	}
	for cat, want := range grades {
		// Ideal is ignored for graded / intrinsic scoring.
		got := DimFit(dim, "", cat)
		if got != want {
			t.Fatalf("TitleFit %q = %v, want %v", cat, got, want)
		}
	}
	if got := DimFit(dim, "ignored", "bogus"); got != 0 {
		t.Fatalf("invalid TitleFit = %v, want 0", got)
	}
}

func TestWorkedExample_TrueFit975(t *testing.T) {
	t.Parallel()
	// Sandbox / REFACTOR_PLAN worked example → 9.75 / 12.
	profile := IdealProfile{
		domain.DimLength:      "Medium",
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
	classification := Classification{
		domain.DimLength:      "Medium",           // 1.0 exact
		domain.DimTopic:       "Work",             // 1.0 match
		domain.DimHumorStyle:  "Observational",    // 1.0 match
		domain.DimComplexity:  "Thoughtful",       // 0.5 adjacent
		domain.DimEdginess:    "Clean",            // 1.0 match
		domain.DimStructure:   "Setup–punchline",  // 1.0 match
		domain.DimWordplay:    "Moderate",         // 0.5 adjacent
		domain.DimFreshness:   "Slightly current", // 0.5 adjacent
		domain.DimSetupPayoff: "Balanced",         // 1.0 exact
		domain.DimClarity:     "Mostly clear",     // 0.5 adjacent
		domain.DimEnergy:      "Conversational",   // 1.0 exact
		domain.DimTitleFit:    "Strong",           // 0.75 graded
	}

	wantDims := map[domain.Dimension]float64{
		domain.DimLength: 1.0, domain.DimTopic: 1.0, domain.DimHumorStyle: 1.0,
		domain.DimComplexity: 0.5, domain.DimEdginess: 1.0, domain.DimStructure: 1.0,
		domain.DimWordplay: 0.5, domain.DimFreshness: 0.5, domain.DimSetupPayoff: 1.0,
		domain.DimClarity: 0.5, domain.DimEnergy: 1.0, domain.DimTitleFit: 0.75,
	}
	for dim, want := range wantDims {
		got := DimFit(dim, profile[dim], classification[dim])
		if got != want {
			t.Errorf("dim_fit[%s] = %v, want %v", dim, got, want)
		}
	}

	got := TrueFit(classification, profile)
	const want = 9.75
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("TrueFit = %v, want %v", got, want)
	}
}

func TestOppositeJoke_NearZero(t *testing.T) {
	t.Parallel()
	// Ideal at one extreme of each ordinal so an opposite can land at distance ≥2
	// (Length only has 3 buckets — Short↔Long is the only zero-scoring pair).
	profile := IdealProfile{
		domain.DimLength:      "Short",
		domain.DimTopic:       "Work",
		domain.DimHumorStyle:  "Observational",
		domain.DimComplexity:  "Expert",
		domain.DimEdginess:    "Clean",
		domain.DimStructure:   "Setup–punchline",
		domain.DimWordplay:    "None",
		domain.DimFreshness:   "Timeless",
		domain.DimSetupPayoff: "Immediate",
		domain.DimClarity:     "Crystal clear",
		domain.DimEnergy:      "Deadpan",
	}
	classification := Classification{
		domain.DimLength:      "Long",             // Short↔Long → 0
		domain.DimTopic:       "Politics",         // categorical miss → 0
		domain.DimHumorStyle:  CatchAll,           // catch-all → 0
		domain.DimComplexity:  "Very simple",      // Expert↔Very simple → 0
		domain.DimEdginess:    "Slightly edgy",    // categorical miss → 0
		domain.DimStructure:   "List/build-up",    // categorical miss → 0
		domain.DimWordplay:    "Heavy",            // None↔Heavy → 0
		domain.DimFreshness:   "Time-sensitive",   // Timeless↔Time-sensitive → 0
		domain.DimSetupPayoff: "Very long build",  // Immediate↔Very long → 0
		domain.DimClarity:     "Reinterpretation", // Crystal clear↔Reinterpretation → 0
		domain.DimEnergy:      CatchAll,           // catch-all → 0
		domain.DimTitleFit:    "Mismatch",         // graded 0
	}

	got := TrueFit(classification, profile)
	if got != 0 {
		t.Fatalf("opposite TrueFit = %v, want 0", got)
	}
}

func TestPerfectMatch_TrueFit12(t *testing.T) {
	t.Parallel()
	profile := IdealProfile{
		domain.DimLength:      "Medium",
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
	classification := Classification{
		domain.DimLength:      "Medium",
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
		domain.DimTitleFit:    "Perfect",
	}
	if got := TrueFit(classification, profile); got != 12.0 {
		t.Fatalf("perfect TrueFit = %v, want 12.0", got)
	}
}
