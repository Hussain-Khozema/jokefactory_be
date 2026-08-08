package domain

// RoundConfig holds instructor-tunable parameters for a round.
type RoundConfig struct {
	CustomerBudget        float64
	BatchSize             int
	MarketPrice           float64
	CostOfPublishing      float64
	CostOfDiscard         float64
	CustomerCount         int
	BuyThreshold          float64
	Jitter                float64
	SwapMargin            float64
	FeedbackJokeCount     int
	FeedbackPassThreshold float64
}

// IdealProfile maps each selectable dimension to its ideal category.
// Excludes Title Fit (intrinsic / graded).
type IdealProfile map[Dimension]string

// DefaultRoundConfig returns the pilot defaults from §2.4.
func DefaultRoundConfig() RoundConfig {
	return RoundConfig{
		CustomerBudget:        DefaultCustomerBudget,
		BatchSize:             DefaultInstructorBatchSize,
		MarketPrice:           DefaultMarketPrice,
		CostOfPublishing:      DefaultCostOfPublishing,
		CostOfDiscard:         DefaultCostOfDiscard,
		CustomerCount:         DefaultCustomerCount,
		BuyThreshold:          DefaultBuyThreshold,
		Jitter:                DefaultJitter,
		SwapMargin:            DefaultSwapMargin,
		FeedbackJokeCount:     DefaultFeedbackJokeCount,
		FeedbackPassThreshold: DefaultFeedbackPassThreshold,
	}
}

// ConfigFromRound copies persisted knobs off a Round.
func ConfigFromRound(r *Round) RoundConfig {
	if r == nil {
		return DefaultRoundConfig()
	}
	return RoundConfig{
		CustomerBudget:        r.CustomerBudget,
		BatchSize:             r.BatchSize,
		MarketPrice:           r.MarketPrice,
		CostOfPublishing:      r.CostOfPublishing,
		CostOfDiscard:         r.CostOfDiscard,
		CustomerCount:         r.CustomerCount,
		BuyThreshold:          r.BuyThreshold,
		Jitter:                r.Jitter,
		SwapMargin:            r.SwapMargin,
		FeedbackJokeCount:     r.FeedbackJokeCount,
		FeedbackPassThreshold: r.FeedbackPassThreshold,
	}
}
