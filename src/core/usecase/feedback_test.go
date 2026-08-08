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
