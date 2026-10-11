package ports

import (
	"context"
	"time"

	"jokefactory/src/core/domain"
)

type Repository interface {
	Health(ctx context.Context) error
}

type BatchWithJokes struct {
	Batch domain.Batch
	Jokes []domain.Joke
}

type TeamMember struct {
	UserID      int64
	DisplayName string
	Role        domain.Role
}

type LobbySnapshot struct {
	RoundID    int64
	Summary    LobbySummary
	Teams      []LobbyTeam
	Unassigned []LobbyUnassigned
}

type LobbySummary struct {
	Waiting   int
	Assigned  int
	Dropped   int
	TeamCount int
}

type LobbyTeam struct {
	Team    domain.Team
	Members []TeamMember
}

type LobbyUnassigned struct {
	UserID      int64
	DisplayName string
	Status      domain.ParticipantStatus
}

type TeamSummary struct {
	Team               domain.Team
	RoundID            int64
	Rank               int
	Points             int
	Profit             float64
	TotalSales         int
	Performance        string
	BatchesCreated     int
	BatchesProcessed   int
	PublishedJokes     int
	DiscardedJokes     int
	UnsoldJokes        int
	SoldJokesCount     int
	UnprocessedBatches int
	// JokesCreated is every joke the team's marketer decided on
	// (published + discarded). The frontend cannot derive it, because round 2
	// batch sizes vary; Marketing's Content Waste tile is
	// JokesCreated - JokesPublished.
	JokesCreated int
	// JokesPublished mirrors PublishedJokes under the name the spec uses.
	JokesPublished int
	// Lean is set only for LEAN rounds; Profit already includes its costs.
	Lean *LeanTeamStats
}

type TeamStats struct {
	Rank             int         `json:"rank"`
	Team             domain.Team `json:"team"`
	BatchesProcessed int         `json:"batches_processed"`
	TotalSales       int         `json:"total_sales"`
	PublishedJokes   int         `json:"published_jokes"`
	DiscardedJokes   int         `json:"discarded_jokes"`
	TotalJokes       int         `json:"total_jokes"`
	UnsoldJokes      int         `json:"unsold_jokes"`
	Profit           float64     `json:"profit"`
	// Lean is set only for LEAN rounds (nil → key omitted, so CLASSIC JSON is
	// unchanged); Profit already includes its costs.
	Lean *LeanTeamStats `json:"lean,omitempty"`
}

// LeanTeamStats is present only for LEAN rounds (nil → key omitted in JSON).
type LeanTeamStats struct {
	// BatchesSubmitted counts submits (+1 batch_submission_events), not the
	// -1 events that record processing.
	BatchesSubmitted int `json:"batches_submitted"`
	// ShippingCost is the sum over sent batches of the shipping fee for the
	// batch's size (domain.ShippingFeeFor; an unsplit batch ships as 5).
	ShippingCost float64 `json:"shipping_cost"`
	// StorageCost is storage_fee_per_joke_minute × WIP joke-minutes (jokes
	// waiting on Marketing's desk; an unsplit batch counts 5 jokes).
	StorageCost float64 `json:"storage_cost"`
	// HostingCost is hosting_fee_per_joke_minute × inventory joke-minutes
	// (released jokes waiting for their first sale).
	HostingCost float64 `json:"hosting_cost"`
	// SetupCost is an alias of ShippingCost, kept for older frontends.
	SetupCost float64 `json:"setup_cost"`
	// CarryingCost is an alias of StorageCost + HostingCost, kept for older
	// frontends.
	CarryingCost           float64      `json:"carrying_cost"`
	AvgSubmitToPublishS    float64      `json:"avg_submit_to_publish_s"`
	AvgPublishToFirstSaleS float64      `json:"avg_publish_to_first_sale_s"`
	BatchSeries            []BatchPoint `json:"batch_series"`
}

// BatchPoint is one submitted batch, in submission order, for the learning curve.
type BatchPoint struct {
	Seq         int       `json:"seq"`
	SubmittedAt time.Time `json:"submitted_at"`
	Jokes       int       `json:"jokes"`
	Published   int       `json:"published"`
	Sold        int       `json:"sold"`
}

type SalesPoint struct {
	EventIndex       int       `json:"event_index"`
	TeamEventIndex   int       `json:"team_event_index"`
	Timestamp        time.Time `json:"timestamp"`
	TeamID           int64     `json:"team_id"`
	TeamName         string    `json:"team_name"`
	CumulativePoints int       `json:"cumulative_points"`
}

type RoundStats struct {
	RoundID     int64       `json:"round_id"`
	Leaderboard []TeamStats `json:"leaderboard"`
}

// LeanChartTeamRow is one team of a round (a team_rounds_state row) with the
// per-team totals the lean charts read from leanTeamCTE. ShippingCost is the
// leaderboard's shipping_cost; WIPJokeMinutes and InventoryJokeMinutes price
// the storage and hosting fees.
type LeanChartTeamRow struct {
	TeamID               int64
	TeamName             string
	BatchesSubmitted     int
	DiscardedJokes       int
	ShippingCost         float64
	WIPJokeMinutes       float64
	InventoryJokeMinutes float64
}

// LeanChartBatchRow is one submitted batch of the round. JokeRows is 0 for a
// batch Marketing has not split yet. Sold counts its jokes with any +1
// purchase event; FirstSoldAt is the earliest of those events (nil: none).
type LeanChartBatchRow struct {
	BatchID     int64
	TeamID      int64
	SubmittedAt time.Time
	ProcessedAt *time.Time
	JokeRows    int
	Published   int
	Sold        int
	FirstSoldAt *time.Time
}

// LeanChartJokeRow is one PUBLISHED joke of the round with its first sale.
type LeanChartJokeRow struct {
	TeamID      int64
	PublishedAt time.Time
	FirstSoldAt *time.Time
}

// LeanChartRows is everything the lean charts are built from, read in one
// snapshot. ClockEnd is the round's ended_at, or the database's now() while it
// runs — the instant the Lean costs stop at.
type LeanChartRows struct {
	ClockEnd time.Time
	Teams    []LeanChartTeamRow
	Batches  []LeanChartBatchRow
	Jokes    []LeanChartJokeRow
}

type UserRepository interface {
	CreateUser(ctx context.Context, displayName string) (*domain.User, error)
	GetUserByDisplayName(ctx context.Context, displayName string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	UpdateUserAssignment(ctx context.Context, userID int64, role *domain.Role, teamID *int64) error
	UpdateUserStatus(ctx context.Context, userID int64, status domain.ParticipantStatus) error
	PatchUserInRound(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) error
	MarkUserAssigned(ctx context.Context, userID int64) error
	ListUsersByStatus(ctx context.Context, status domain.ParticipantStatus) ([]domain.User, error)
	ListTeamMembers(ctx context.Context, teamID int64) ([]TeamMember, error)
	DeleteUser(ctx context.Context, userID int64) error
}

type TeamRepository interface {
	EnsureTeamCount(ctx context.Context, teamCount int) ([]domain.Team, error)
	GetTeam(ctx context.Context, teamID int64) (*domain.Team, error)
}

type RoundRepository interface {
	GetActiveRound(ctx context.Context) (*domain.Round, error)
	GetRoundByID(ctx context.Context, roundID int64) (*domain.Round, error)
	GetLatestRound(ctx context.Context) (*domain.Round, error)
	ListRounds(ctx context.Context) ([]domain.Round, error)
	UpdateRoundConfig(ctx context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error)
	InsertRoundConfig(ctx context.Context, roundID int64, cfg *domain.RoundConfig) (*domain.Round, error)
	UpsertIdealProfile(ctx context.Context, roundID int64, profile domain.IdealProfile) error
	GetIdealProfile(ctx context.Context, roundID int64) (domain.IdealProfile, error)
	StartRound(ctx context.Context, roundID int64) (*domain.Round, error)
	EndRound(ctx context.Context, roundID int64) (*domain.Round, error)
	SetRoundPopupState(ctx context.Context, roundID int64, isActive bool) (*domain.Round, error)
	EnsureTeamRoundState(ctx context.Context, roundID, teamID int64) error
	ResetGame(ctx context.Context) error
}

type BatchRepository interface {
	// CreateBatch stores a batch from exactly one of jokes (already split) or
	// rawText (the JM's unsplit blob, stored in both raw_text columns).
	CreateBatch(ctx context.Context, roundID, teamID int64, jokes []string, rawText string) (*domain.Batch, error)
	ListBatchesByTeam(ctx context.Context, roundID, teamID int64) ([]domain.Batch, error)
	GetBatchWithJokes(ctx context.Context, batchID int64) (*BatchWithJokes, error)
}

// JokePublishDecision is Marketing's per-joke publish/discard choice.
type JokePublishDecision struct {
	JokeID      int64
	Title       string
	IsPublished bool
}

// PublishResult is the outcome of Marketing publishing a batch.
type PublishResult struct {
	Batch        domain.Batch
	PublishedIDs []int64
	DiscardedIDs []int64
}

type MarketingRepository interface {
	// ClaimNextBatch locks the next SUBMITTED batch for the marketer's team
	// (FOR UPDATE SKIP LOCKED). Returns nil,nil when the queue is empty.
	ClaimNextBatch(ctx context.Context, roundID, teamID, marketerID int64) (*BatchWithJokes, error)
	CountSubmittedBatchesForTeam(ctx context.Context, roundID, teamID int64) (int, error)
	// PublishBatch applies the marketer's publish/discard decisions. Whether an
	// all-discard batch is allowed is the caller's call: the ">=1 published"
	// rule is game pedagogy (it holds in round 1 only), so the repository takes
	// it as a flag instead of looking the round number up itself. note is
	// Marketing's already-validated note to the Joke Maker (nil stores NULL); it
	// is written in the same transaction that marks the batch PROCESSED.
	PublishBatch(ctx context.Context, batchID, marketerID, teamID int64, decisions []JokePublishDecision, requireAtLeastOnePublished bool, note *string) (*PublishResult, error)
	// SplitBatch replaces a batch's jokes with the supplied texts, clears
	// raw_text (raw_text_original is left intact) and refreshes the lock.
	// Conflicts when any existing joke has already been decided.
	SplitBatch(ctx context.Context, batchID, marketerID, teamID int64, jokes []string) (*BatchWithJokes, error)
	// UnsplitBatch drops a batch's jokes and puts it back in the unsplit state,
	// restoring raw_text from the immutable raw_text_original (falling back to
	// re-joining the joke texts for legacy jokes-array submissions, which have
	// no original blob). Refreshes the lock.
	UnsplitBatch(ctx context.Context, batchID, marketerID, teamID int64) (*BatchWithJokes, error)
}

// FeedbackJokeRow is one published joke plus its materialized dim_fits for feedback.
type FeedbackJokeRow struct {
	JokeID    int64
	JokeTitle string
	WasBought bool
	DimFits   map[domain.Dimension]float64
}

type StatsRepository interface {
	GetTeamSummary(ctx context.Context, roundID, teamID int64) (*TeamSummary, error)
	GetLobby(ctx context.Context, roundID int64) (*LobbySnapshot, error)
	GetRoundStatsV2(ctx context.Context, roundID int64) (*RoundStats, error)
	// ListTeamFeedbackJokes returns the latest limit published jokes for a team
	// (newest first), each with dim_fits and whether any AI customer bought it.
	ListTeamFeedbackJokes(ctx context.Context, roundID, teamID int64, limit int) ([]FeedbackJokeRow, error)
	// Lean variants compute shipping, storage and hosting fees at read time and fold
	// them into profit. The Classic queries above are not touched.
	GetRoundStatsLean(ctx context.Context, roundID int64) (*RoundStats, error)
	GetTeamSummaryLean(ctx context.Context, roundID, teamID int64) (*TeamSummary, error)
	// GetLeanChartRows reads the raw rows the lean charts are built from, in
	// one read-only snapshot. An unknown round is a not-found error.
	GetLeanChartRows(ctx context.Context, roundID int64) (*LeanChartRows, error)
}

// MaxClassificationAttempts is the job-level retry ceiling before FAILED sticks.
const MaxClassificationAttempts = 5

// StaleClassificationAfter is how long a PROCESSING job may sit before the
// reconciler treats it as abandoned and re-enqueues.
const StaleClassificationAfter = 5 * time.Minute

// JokeFitMaterialization is one joke's persisted classification + fit scores.
type JokeFitMaterialization struct {
	JokeID     int64
	RoundID    int64
	Categories map[domain.Dimension]string
	DimFits    map[domain.Dimension]float64
	TrueFit    float64
}

type ClassificationRepository interface {
	EnsureClassificationJob(ctx context.Context, batchID, roundID int64) error
	// ClaimClassificationJob marks the job PROCESSING and increments attempts.
	// Returns nil,nil when the job is DONE, missing, or not claimable.
	ClaimClassificationJob(ctx context.Context, batchID int64) (*domain.ClassificationJob, error)
	MarkClassificationDone(ctx context.Context, batchID int64, model string) error
	MarkClassificationFailed(ctx context.Context, batchID int64, errMsg string) error
	PersistJokeFits(ctx context.Context, fits []JokeFitMaterialization) error
	GetJokeFit(ctx context.Context, jokeID int64) (*domain.JokeFit, error)
	ListJokeDimFits(ctx context.Context, jokeID int64) ([]domain.JokeDimFit, error)
	ListJokeDimensionValues(ctx context.Context, jokeID int64) ([]domain.JokeDimensionValue, error)
	// ListOrphanClassificationBatchIDs returns PROCESSED batches that still need
	// classification (missing/pending/failed/stale job).
	ListOrphanClassificationBatchIDs(ctx context.Context, limit int) ([]int64, error)
}

// HeldJoke is one AI customer's current holding with its materialized fit.
type HeldJoke struct {
	JokeID  int64
	TeamID  int64
	TrueFit float64
	Price   float64
}

// CandidateJoke is a classified published joke available for purchase evaluation.
type CandidateJoke struct {
	JokeID  int64
	TeamID  int64
	RoundID int64
	TrueFit float64
	// Text is the joke text. Lean uses it to avoid selling a customer the
	// same joke twice when a team publishes the same text more than once;
	// Classic ignores it (ListCandidateJokes leaves it empty).
	Text string
}

// MarketJoke is a published joke as shown on the market board.
type MarketJoke struct {
	JokeID      int64
	JokeText    string
	JokeTitle   *string
	TeamID      int64
	TeamName    string
	SoldCount   int
	PublishedAt *time.Time
}

type AICustomerRepository interface {
	ReplaceAICustomers(ctx context.Context, roundID int64, customers []domain.AICustomer) error
	ListAICustomers(ctx context.Context, roundID int64) ([]domain.AICustomer, error)
	ListCandidateJokes(ctx context.Context, jokeIDs []int64) ([]CandidateJoke, error)
	ListHoldings(ctx context.Context, roundID, aiCustomerID int64) ([]HeldJoke, error)
	// BuyJoke locks the customer row, deducts budget, inserts purchase + event, +1 points.
	BuyJoke(ctx context.Context, roundID, aiCustomerID, jokeID, teamID int64, price float64) error
	// SwapJoke returns weakestHeld and buys jokeID in one transaction.
	SwapJoke(ctx context.Context, roundID, aiCustomerID, buyJokeID, buyTeamID, returnJokeID, returnTeamID int64, price float64) error
	ListMarket(ctx context.Context, roundID int64) ([]MarketJoke, error)
	// ClaimDueCustomers atomically marks every customer of the round whose
	// arrives_at <= now and shopped_at IS NULL as shopped (shopped_at = now)
	// and returns them. Two concurrent callers can never claim the same
	// customer. Lean mode only. now is the reference time for in-memory
	// stores only; Postgres ignores it and uses the database clock (now()),
	// the same clock that stamped rounds.started_at and so arrives_at.
	ClaimDueCustomers(ctx context.Context, roundID int64, now time.Time) ([]domain.AICustomer, error)
	// ListPublishedCandidates returns every published, classified joke in the
	// round with its team and true fit — the shelf a Lean customer shops from.
	ListPublishedCandidates(ctx context.Context, roundID int64) ([]CandidateJoke, error)
}

// Store is the composed surface used by usecases during the refactor.
// Narrower per-aggregate deps land as services are rewritten in later phases.
type Store interface {
	Repository
	UserRepository
	TeamRepository
	RoundRepository
	BatchRepository
	MarketingRepository
	ClassificationRepository
	AICustomerRepository
	StatsRepository
}
