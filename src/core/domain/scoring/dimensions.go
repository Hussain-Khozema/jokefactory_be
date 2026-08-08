package scoring

import "jokefactory/src/core/domain"

// Type describes how a dimension contributes to dim_fit.
type Type string

const (
	// Ordinal scores 1.0 exact, 0.5 adjacent, else 0.
	Ordinal Type = "ordinal"
	// Categorical scores 1.0 on exact match, else 0.
	Categorical Type = "categorical"
	// Graded maps Title Fit categories to an intrinsic [0,1] score.
	Graded Type = "graded"
)

// CatchAll is the sentinel category that scores 0 against any normal ideal.
const CatchAll = "None of the above"

// DimensionSpec holds the static configuration for one judging dimension.
type DimensionSpec struct {
	Dimension  domain.Dimension
	Scoring    Type
	Categories []string
	// HasIdeal is false only for Title Fit (intrinsic / graded).
	HasIdeal bool
}

// AllDimensions is the fixed order used when summing true_fit (0–12).
var AllDimensions = []domain.Dimension{
	domain.DimLength,
	domain.DimTopic,
	domain.DimHumorStyle,
	domain.DimComplexity,
	domain.DimEdginess,
	domain.DimStructure,
	domain.DimWordplay,
	domain.DimFreshness,
	domain.DimSetupPayoff,
	domain.DimClarity,
	domain.DimEnergy,
	domain.DimTitleFit,
}

// Specs is the single source of truth for category lists and scoring types.
var Specs = map[domain.Dimension]DimensionSpec{
	domain.DimLength: {
		Dimension:  domain.DimLength,
		Scoring:    Ordinal,
		Categories: []string{LengthShort, LengthMedium, LengthLong},
		HasIdeal:   true,
	},
	domain.DimTopic: {
		Dimension: domain.DimTopic,
		Scoring:   Categorical,
		Categories: []string{
			"Work", "Relationships", "Family", "Food", "Technology",
			"Animals", "School", "Money", "Travel", "Health",
			"Sports", "Politics", "Everyday", "Language", "Other",
		},
		HasIdeal: true,
	},
	domain.DimHumorStyle: {
		Dimension: domain.DimHumorStyle,
		Scoring:   Categorical,
		Categories: []string{
			"Pun", "Observational", "Irony", "Absurdity", "Exaggeration",
			"Self-deprecating", "Anti-joke", "Callback", CatchAll,
		},
		HasIdeal: true,
	},
	domain.DimComplexity: {
		Dimension: domain.DimComplexity,
		Scoring:   Ordinal,
		Categories: []string{
			"Very simple", "Simple", "Moderate", "Thoughtful", "Expert",
		},
		HasIdeal: true,
	},
	domain.DimEdginess: {
		Dimension:  domain.DimEdginess,
		Scoring:    Categorical,
		Categories: []string{"Clean", "Slightly edgy", CatchAll},
		HasIdeal:   true,
	},
	domain.DimStructure: {
		Dimension: domain.DimStructure,
		Scoring:   Categorical,
		Categories: []string{
			"One-liner", "Setup–punchline", "Question–answer", "Short story",
			"Dialogue/conversation", "List/build-up", CatchAll,
		},
		HasIdeal: true,
	},
	domain.DimWordplay: {
		Dimension:  domain.DimWordplay,
		Scoring:    Ordinal,
		Categories: []string{"None", "Light", "Moderate", "Heavy"},
		HasIdeal:   true,
	},
	domain.DimFreshness: {
		Dimension: domain.DimFreshness,
		Scoring:   Ordinal,
		Categories: []string{
			"Timeless", "Slightly current", "Current", "Very topical", "Time-sensitive",
		},
		HasIdeal: true,
	},
	domain.DimSetupPayoff: {
		Dimension: domain.DimSetupPayoff,
		Scoring:   Ordinal,
		Categories: []string{
			"Immediate", "Quick", "Balanced", "Long", "Very long build",
		},
		HasIdeal: true,
	},
	domain.DimClarity: {
		Dimension: domain.DimClarity,
		Scoring:   Ordinal,
		Categories: []string{
			"Crystal clear", "Mostly clear", "Slightly ambiguous", "Ambiguous", "Reinterpretation",
		},
		HasIdeal: true,
	},
	domain.DimEnergy: {
		Dimension: domain.DimEnergy,
		Scoring:   Ordinal,
		Categories: []string{
			"Deadpan", "Low", "Conversational", "Animated", "High-energy", CatchAll,
		},
		HasIdeal: true,
	},
	domain.DimTitleFit: {
		Dimension:  domain.DimTitleFit,
		Scoring:    Graded,
		Categories: []string{"Perfect", "Strong", "Moderate", "Weak", "Mismatch"},
		HasIdeal:   false,
	},
}

// IdealDimensions are the 11 dimensions that have an instructor ideal selector
// (excludes Title Fit).
func IdealDimensions() []domain.Dimension {
	out := make([]domain.Dimension, 0, len(AllDimensions)-1)
	for _, d := range AllDimensions {
		if Specs[d].HasIdeal {
			out = append(out, d)
		}
	}
	return out
}

// LLMDimensions are the 11 dimensions classified by the LLM (excludes Length).
func LLMDimensions() []domain.Dimension {
	out := make([]domain.Dimension, 0, len(AllDimensions)-1)
	for _, d := range AllDimensions {
		if d != domain.DimLength {
			out = append(out, d)
		}
	}
	return out
}

// IsValidCategory reports whether category is in the allowed list for dim.
func IsValidCategory(dim domain.Dimension, category string) bool {
	spec, ok := Specs[dim]
	if !ok {
		return false
	}
	for _, c := range spec.Categories {
		if c == category {
			return true
		}
	}
	return false
}

// IsCatchAll reports whether category is the catch-all sentinel.
func IsCatchAll(category string) bool {
	return category == CatchAll
}

// ValidateIdealProfile checks that profile covers all 11 ideal dimensions
// with categories allowed by Specs. Catch-all is rejected as an ideal.
func ValidateIdealProfile(profile domain.IdealProfile) error {
	if len(profile) == 0 {
		return domain.NewValidationError("ideal_profile", "ideal_profile is required")
	}
	for _, dim := range IdealDimensions() {
		cat, ok := profile[dim]
		if !ok || cat == "" {
			return domain.NewValidationError("ideal_profile", "missing category for "+string(dim))
		}
		if !IsValidCategory(dim, cat) {
			return domain.NewValidationError("ideal_profile", "invalid category for "+string(dim))
		}
		if IsCatchAll(cat) {
			return domain.NewValidationError("ideal_profile", "catch-all is not a valid ideal for "+string(dim))
		}
	}
	for dim := range profile {
		spec, ok := Specs[dim]
		if !ok || !spec.HasIdeal {
			return domain.NewValidationError("ideal_profile", "dimension has no ideal selector: "+string(dim))
		}
	}
	return nil
}

// CategoryIndex returns the ordinal position of category in dim's list, or -1.
func CategoryIndex(dim domain.Dimension, category string) int {
	spec, ok := Specs[dim]
	if !ok {
		return -1
	}
	for i, c := range spec.Categories {
		if c == category {
			return i
		}
	}
	return -1
}

// titleFitGrades maps Title Fit categories to their intrinsic dim_fit.
var titleFitGrades = map[string]float64{
	"Perfect":  1.0,
	"Strong":   0.75,
	"Moderate": 0.5,
	"Weak":     0.25,
	"Mismatch": 0.0,
}
