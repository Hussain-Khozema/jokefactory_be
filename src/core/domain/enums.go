package domain

// Role represents a user's role in the game.
type Role string

const (
	// RoleInstructor is the class facilitator.
	RoleInstructor Role = "INSTRUCTOR"
	// RoleJM is a Joke Maker (student writer).
	RoleJM Role = "JM"
	// RoleMarketing titles jokes and chooses publish vs discard.
	RoleMarketing Role = "MARKETING"
)

// ParticipantStatus indicates lobby assignment state.
type ParticipantStatus string

const (
	// ParticipantWaiting means the user has joined but is not yet assigned.
	ParticipantWaiting ParticipantStatus = "WAITING"
	// ParticipantAssigned means the user has a role (and team, if applicable).
	ParticipantAssigned ParticipantStatus = "ASSIGNED"
)

// RoundStatus represents lifecycle of a round.
type RoundStatus string

const (
	// RoundConfigured means the round exists but has not started.
	RoundConfigured RoundStatus = "CONFIGURED"
	// RoundActive means the round is in progress.
	RoundActive RoundStatus = "ACTIVE"
	// RoundEnded means the round is finished.
	RoundEnded RoundStatus = "ENDED"
)

// BatchStatus represents lifecycle of a batch.
type BatchStatus string

const (
	// BatchDraft is an unsubmitted batch (unused in the current JM flow).
	BatchDraft BatchStatus = "DRAFT"
	// BatchSubmitted means JM submitted; waiting for Marketing.
	BatchSubmitted BatchStatus = "SUBMITTED"
	// BatchProcessed means Marketing has published/discarded the jokes.
	BatchProcessed BatchStatus = "PROCESSED"
)

// PublishStatus is the Marketing decision for a joke.
type PublishStatus string

const (
	// PublishPending means Marketing has not decided yet.
	PublishPending PublishStatus = "PENDING"
	// PublishPublished means the joke is live on the market.
	PublishPublished PublishStatus = "PUBLISHED"
	// PublishDiscarded means Marketing chose not to publish.
	PublishDiscarded PublishStatus = "DISCARDED"
)

// ClassificationStatus tracks async LLM classification of a published batch.
type ClassificationStatus string

const (
	// ClassificationPending means the batch is queued for classification.
	ClassificationPending ClassificationStatus = "PENDING"
	// ClassificationProcessing means a worker is classifying the batch.
	ClassificationProcessing ClassificationStatus = "PROCESSING"
	// ClassificationDone means fit scores are materialized.
	ClassificationDone ClassificationStatus = "DONE"
	// ClassificationFailed means classification exhausted retries.
	ClassificationFailed ClassificationStatus = "FAILED"
)

// Dimension identifies one of the 12 judging dimensions.
type Dimension string

const (
	// DimLength is word-count length (code-classified).
	DimLength Dimension = "LENGTH"
	// DimTopic is the joke's subject matter.
	DimTopic Dimension = "TOPIC"
	// DimHumorStyle is the primary humor technique.
	DimHumorStyle Dimension = "HUMOR_STYLE"
	// DimComplexity is how much thought the joke requires.
	DimComplexity Dimension = "COMPLEXITY"
	// DimEdginess is how clean vs edgy the joke is.
	DimEdginess Dimension = "EDGINESS"
	// DimStructure is the joke's structural form.
	DimStructure Dimension = "STRUCTURE"
	// DimWordplay is how much wordplay the joke uses.
	DimWordplay Dimension = "WORDPLAY"
	// DimFreshness is how topical vs timeless the joke is.
	DimFreshness Dimension = "FRESHNESS"
	// DimSetupPayoff is the setup-to-punchline pacing.
	DimSetupPayoff Dimension = "SETUP_PAYOFF"
	// DimClarity is how clear vs ambiguous the joke is.
	DimClarity Dimension = "CLARITY"
	// DimEnergy is the delivery energy level.
	DimEnergy Dimension = "ENERGY"
	// DimTitleFit is how well the title matches the joke (intrinsic, graded).
	DimTitleFit Dimension = "TITLE_FIT"
)
