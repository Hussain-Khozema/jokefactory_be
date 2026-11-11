package usecase

import (
	"reflect"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/domain/scoring"
)

func TestSelectFeedbackDimensionsPreferImprove(t *testing.T) {
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 1.0
	}
	fits[domain.DimLength] = 0
	fits[domain.DimTopic] = 0
	fits[domain.DimHumorStyle] = 0
	fits[domain.DimComplexity] = 0

	good, improve := SelectFeedbackDimensions(fits, 0.75)
	wantImprove := []string{"LENGTH", "TOPIC", "HUMOR_STYLE"}
	wantGood := []string{"EDGINESS", "STRUCTURE"}
	if !reflect.DeepEqual(improve, wantImprove) {
		t.Fatalf("improve = %v, want %v", improve, wantImprove)
	}
	if !reflect.DeepEqual(good, wantGood) {
		t.Fatalf("good = %v, want %v", good, wantGood)
	}
}

func TestSelectFeedbackDimensionsBackfillPassWhenFewFails(t *testing.T) {
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 1.0
	}
	fits[domain.DimLength] = 0

	good, improve := SelectFeedbackDimensions(fits, 0.75)
	if !reflect.DeepEqual(improve, []string{"LENGTH"}) {
		t.Fatalf("improve = %v", improve)
	}
	wantGood := []string{"TOPIC", "HUMOR_STYLE", "COMPLEXITY", "EDGINESS"}
	if !reflect.DeepEqual(good, wantGood) {
		t.Fatalf("good = %v, want %v", good, wantGood)
	}
}

func TestSelectFeedbackDimensionsBackfillFailWhenFewPasses(t *testing.T) {
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 0
	}
	fits[domain.DimTitleFit] = 1.0

	good, improve := SelectFeedbackDimensions(fits, 0.75)
	if !reflect.DeepEqual(good, []string{"TITLE_FIT"}) {
		t.Fatalf("good = %v", good)
	}
	wantImprove := []string{"LENGTH", "TOPIC", "HUMOR_STYLE", "COMPLEXITY"}
	if !reflect.DeepEqual(improve, wantImprove) {
		t.Fatalf("improve = %v, want %v", improve, wantImprove)
	}
}

func TestSelectFeedbackDimensionsEmpty(t *testing.T) {
	good, improve := SelectFeedbackDimensions(nil, 0.75)
	if len(good) != 0 || len(improve) != 0 {
		t.Fatalf("got good=%v improve=%v", good, improve)
	}
}

func TestFeedbackStartVariesByJokeAndWraps(t *testing.T) {
	if FeedbackStart(0) != 0 || FeedbackStart(13) != 1 || FeedbackStart(24) != 0 || FeedbackStart(-1) != 11 {
		t.Fatalf("starts = %d %d %d %d", FeedbackStart(0), FeedbackStart(13), FeedbackStart(24), FeedbackStart(-1))
	}
}

func TestSelectFeedbackDimensionsFromShowsDifferentDimensionsPerJoke(t *testing.T) {
	// The reported bug: three jokes that all miss Topic, Humor Style and
	// Complexity showed the same card. Starting each joke at a different
	// dimension must surface later dimensions too.
	fits := map[domain.Dimension]float64{}
	for _, dim := range scoring.AllDimensions {
		fits[dim] = 0
	}
	fits[domain.DimLength] = 1
	fits[domain.DimEdginess] = 1
	fits[domain.DimClarity] = 1
	fits[domain.DimTitleFit] = 1

	g0, i0 := SelectFeedbackDimensionsFrom(fits, 0.75, 0)
	g6, i6 := SelectFeedbackDimensionsFrom(fits, 0.75, 6)
	if reflect.DeepEqual(i0, i6) {
		t.Fatalf("same improve list for different starts: %v", i0)
	}
	if !reflect.DeepEqual(i0, []string{"TOPIC", "HUMOR_STYLE", "COMPLEXITY"}) || !reflect.DeepEqual(g0, []string{"LENGTH", "EDGINESS"}) {
		t.Fatalf("start 0: good=%v improve=%v", g0, i0)
	}
	if !reflect.DeepEqual(i6, []string{"WORDPLAY", "FRESHNESS", "SETUP_PAYOFF"}) || !reflect.DeepEqual(g6, []string{"CLARITY", "TITLE_FIT"}) {
		t.Fatalf("start 6: good=%v improve=%v", g6, i6)
	}
	// Wrapping: start 11 begins at Title Fit and continues with Length.
	g11, _ := SelectFeedbackDimensionsFrom(fits, 0.75, 11)
	if !reflect.DeepEqual(g11, []string{"TITLE_FIT", "LENGTH"}) {
		t.Fatalf("start 11 good = %v", g11)
	}
}

func TestDirectionHint(t *testing.T) {
	cases := []struct {
		dim         domain.Dimension
		ideal, joke string
		want        string
	}{
		{domain.DimLength, "Short", "Long", HintLower},
		{domain.DimLength, "Long", "Medium", HintHigher},
		{domain.DimLength, "Medium", "Medium", ""},
		{domain.DimComplexity, "Very simple", "Simple", HintLower},
		{domain.DimEnergy, "Deadpan", scoring.CatchAll, ""},
		{domain.DimTopic, "Work", "Food", ""},
		{domain.DimTitleFit, "", "Weak", ""},
		{domain.DimWordplay, "Heavy", "", ""},
		{domain.DimWordplay, "Heavy", "Not a category", ""},
	}
	for _, c := range cases {
		if got := DirectionHint(c.dim, c.ideal, c.joke); got != c.want {
			t.Errorf("DirectionHint(%s, %q, %q) = %q, want %q", c.dim, c.ideal, c.joke, got, c.want)
		}
	}
}
