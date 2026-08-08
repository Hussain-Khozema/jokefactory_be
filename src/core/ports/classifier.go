package ports

import (
	"context"

	"jokefactory/src/core/domain"
)

// JokeInput is one published joke handed to the LLM classifier.
// Length is classified in code and must not be returned by Classifier.
type JokeInput struct {
	JokeID int64
	Text   string
	Title  string
}

// JokeClassification is the LLM categories for the 11 non-Length dimensions.
type JokeClassification struct {
	JokeID     int64
	Categories map[domain.Dimension]string
}

// Classifier classifies published jokes on the 11 LLM dimensions (excludes Length).
type Classifier interface {
	Classify(ctx context.Context, jokes []JokeInput) ([]JokeClassification, error)
}
