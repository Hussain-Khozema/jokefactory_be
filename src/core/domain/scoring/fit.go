package scoring

import "jokefactory/src/core/domain"

// Classification maps each of the 12 dimensions to the joke's category.
type Classification map[domain.Dimension]string

// IdealProfile maps the 11 selectable dimensions to the instructor's ideal
// category. Title Fit must not appear — it is scored intrinsically.
type IdealProfile map[domain.Dimension]string

// DimFit returns the per-dimension fit in [0, 1] using the 3-tier model:
//
//   - Ordinal:     1.0 exact, 0.5 adjacent (±1 in ordered list), else 0.
//     Catch-all ("None of the above") is binary vs any normal ideal (0 unless
//     the ideal is also the catch-all).
//   - Categorical: 1.0 exact match, else 0.
//   - Graded (Title Fit): Perfect=1, Strong=0.75, Moderate=0.5, Weak=0.25, Mismatch=0.
//
// Unknown / invalid categories yield 0.
func DimFit(dim domain.Dimension, ideal, joke string) float64 {
	spec, ok := Specs[dim]
	if !ok {
		return 0
	}

	switch spec.Scoring {
	case Graded:
		return gradedFit(joke)
	case Categorical:
		return categoricalFit(ideal, joke)
	case Ordinal:
		return ordinalFit(dim, ideal, joke)
	default:
		return 0
	}
}

// TrueFit sums dim_fit across all 12 dimensions. Range is [0, 12] when every
// dimension has a valid classification entry.
func TrueFit(classification Classification, profile IdealProfile) float64 {
	var sum float64
	for _, dim := range AllDimensions {
		jokeCat := classification[dim]
		idealCat := profile[dim] // empty for Title Fit; ignored by DimFit graded path
		sum += DimFit(dim, idealCat, jokeCat)
	}
	return sum
}

func gradedFit(joke string) float64 {
	if score, ok := titleFitGrades[joke]; ok {
		return score
	}
	return 0
}

func categoricalFit(ideal, joke string) float64 {
	if joke == "" || ideal == "" {
		return 0
	}
	if joke == ideal {
		return 1
	}
	return 0
}

func ordinalFit(dim domain.Dimension, ideal, joke string) float64 {
	if joke == "" || ideal == "" {
		return 0
	}

	// Catch-all is binary: only scores 1 when both sides are the catch-all.
	if IsCatchAll(joke) || IsCatchAll(ideal) {
		if joke == ideal {
			return 1
		}
		return 0
	}

	ji := CategoryIndex(dim, joke)
	ii := CategoryIndex(dim, ideal)
	if ji < 0 || ii < 0 {
		return 0
	}
	diff := ji - ii
	if diff < 0 {
		diff = -diff
	}
	switch diff {
	case 0:
		return 1.0
	case 1:
		return 0.5
	default:
		return 0
	}
}
