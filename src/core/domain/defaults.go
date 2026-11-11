package domain

const (
	DefaultCustomerBudget        = 3.0
	DefaultInstructorBatchSize   = 5
	DefaultMarketPrice           = 1.0
	DefaultCostOfPublishing      = 0.1
	DefaultCostOfDiscard         = 0.0
	DefaultCustomerCount         = 150
	DefaultBuyThreshold          = 8.0
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
	DefaultPreferenceBase            = 5.0

	// Two costs, both Marketing's: listing (DefaultCostOfPublishing, per
	// released joke) and hosting (per joke-minute unsold). Shipping, storage
	// and discard default to 0; the columns and the API keep them as knobs
	// (docs/superpowers/specs/2026-11-10-two-costs-feedback-bots-design.md §2).
	DefaultShippingFeeSmall        = 0.0
	DefaultShippingFeeStandard     = 0.0
	DefaultShippingFeeLarge        = 0.0
	DefaultStorageFeePerJokeMinute = 0.0
	DefaultHostingFeePerJokeMinute = 0.05

	// DefaultCustomersPerTeam sizes a LEAN round's market to the class: at
	// StartRound customer_count becomes this × the teams staffed at that
	// moment. 0 keeps the fixed DefaultCustomerCount. Buy threshold 8 and
	// preference base 5 are the values the 20-team sweep validated
	// (docs/superpowers/specs/2026-10-11-tuned-defaults-design.md).
	DefaultCustomersPerTeam = 18
)
