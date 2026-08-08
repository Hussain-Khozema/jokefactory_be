package domain

import "time"

// ClassificationJob tracks async classification of a published batch.
type ClassificationJob struct {
	BatchID      int64
	RoundID      int64
	Status       ClassificationStatus
	Attempts     int
	LastError    *string
	Model        *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ClassifiedAt *time.Time
}

// JokeFit is the materialized true_fit for one published joke.
type JokeFit struct {
	JokeID     int64
	RoundID    int64
	TrueFit    float64
	ComputedAt time.Time
}

// JokeDimensionValue is one classified category for a joke.
type JokeDimensionValue struct {
	JokeID    int64
	Dimension Dimension
	Category  string
}

// JokeDimFit is one per-dimension fit score for a joke.
type JokeDimFit struct {
	JokeID    int64
	Dimension Dimension
	DimFit    float64
}
