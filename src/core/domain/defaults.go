package domain

const (
	DefaultCustomerBudget        = 3.0
	DefaultInstructorBatchSize   = 5
	DefaultMarketPrice           = 1.0
	DefaultCostOfPublishing      = 0.1
	DefaultCostOfDiscard         = 0.01
	DefaultCustomerCount         = 150
	DefaultBuyThreshold          = 7.0
	DefaultJitter                = 0.3
	DefaultSwapMargin            = 0.5
	DefaultFeedbackJokeCount     = 3
	DefaultFeedbackPassThreshold = 0.75

	// DefaultRound2BatchSize is Round 2's batch_size: a cap there (1 to 10
	// jokes), where Round 1's DefaultInstructorBatchSize is exact.
	DefaultRound2BatchSize = 10

	// Lean is the only mode this branch plays: one rule set for both rounds.
	DefaultMarketMode                = MarketLean
	DefaultArrivalWindowSeconds      = 900
	DefaultSetupCostPerBatch         = 0.0
	DefaultCarryingCostPerJokeMinute = 0.0
	DefaultWIPLimit                  = 0
	DefaultPreferenceBase            = 3.0

	// Lean fees (see lean_fees.go).
	DefaultShippingFeeSmall        = 1.0
	DefaultShippingFeeStandard     = 2.0
	DefaultShippingFeeLarge        = 3.0
	DefaultStorageFeePerJokeMinute = 0.10
	DefaultHostingFeePerJokeMinute = 0.05
)
