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
	CreateBatch(ctx context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error)
	ListBatchesByTeam(ctx context.Context, roundID, teamID int64) ([]domain.Batch, error)
	GetBatchWithJokes(ctx context.Context, batchID int64) (*BatchWithJokes, error)
	CountSubmittedBatches(ctx context.Context, roundID int64) (int, error)
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
	PublishBatch(ctx context.Context, batchID, marketerID, teamID int64, decisions []JokePublishDecision) (*PublishResult, error)
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
