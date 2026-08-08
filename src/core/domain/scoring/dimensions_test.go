package scoring

import (
	"testing"

	"jokefactory/src/core/domain"
)

func TestIdealDimensions_ExcludesTitleFit(t *testing.T) {
	t.Parallel()
	dims := IdealDimensions()
	if len(dims) != 11 {
		t.Fatalf("IdealDimensions len = %d, want 11", len(dims))
	}
	for _, d := range dims {
		if d == domain.DimTitleFit {
			t.Fatal("Title Fit must not appear in IdealDimensions")
		}
		if !Specs[d].HasIdeal {
			t.Fatalf("%s HasIdeal=false but listed in IdealDimensions", d)
		}
	}
}

func TestIsValidCategory(t *testing.T) {
	t.Parallel()
	if !IsValidCategory(domain.DimTopic, "Work") {
		t.Fatal("Work should be valid for Topic")
	}
	if IsValidCategory(domain.DimTopic, "Workplace") {
		t.Fatal("Workplace is not in the locked Topic list")
	}
	if !IsValidCategory(domain.DimStructure, "Setup–punchline") {
		t.Fatal("Setup–punchline should be valid (en-dash)")
	}
	if !IsValidCategory(domain.DimHumorStyle, CatchAll) {
		t.Fatal("None of the above should be valid for Humor Style")
	}
}

func TestValidateIdealProfile(t *testing.T) {
	t.Parallel()
	good := domain.IdealProfile{
		domain.DimLength:      LengthMedium,
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
	if err := ValidateIdealProfile(good); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}

	missing := domain.IdealProfile{domain.DimLength: LengthShort}
	if err := ValidateIdealProfile(missing); err == nil {
		t.Fatal("expected error for incomplete profile")
	}

	withTitleFit := domain.IdealProfile{}
	for k, v := range good {
		withTitleFit[k] = v
	}
	withTitleFit[domain.DimTitleFit] = "Perfect"
	if err := ValidateIdealProfile(withTitleFit); err == nil {
		t.Fatal("expected error when Title Fit is included")
	}

	catchAll := domain.IdealProfile{}
	for k, v := range good {
		catchAll[k] = v
	}
	catchAll[domain.DimHumorStyle] = CatchAll
	if err := ValidateIdealProfile(catchAll); err == nil {
		t.Fatal("expected error for catch-all ideal")
	}
}

func TestAllDimensionsHaveSpecs(t *testing.T) {
	t.Parallel()
	if len(AllDimensions) != 12 {
		t.Fatalf("AllDimensions len = %d, want 12", len(AllDimensions))
	}
	for _, d := range AllDimensions {
		spec, ok := Specs[d]
		if !ok {
			t.Fatalf("missing Specs entry for %s", d)
		}
		if len(spec.Categories) == 0 {
			t.Fatalf("%s has empty Categories", d)
		}
		if spec.Dimension != d {
			t.Fatalf("%s Specs.Dimension mismatch: %s", d, spec.Dimension)
		}
	}
}
