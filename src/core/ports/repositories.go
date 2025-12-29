// Package ports defines interfaces (ports) that connect core domain to infrastructure.
// These interfaces follow the ports and adapters (hexagonal) architecture pattern.
//
// Ports are defined here in the core layer, while implementations (adapters)
// live in src/infra/repo. This ensures the core has no dependency on infrastructure.
package ports

import (
	"context"
	"time"

	"jokefactory/src/core/domain"
)

// Repository is the base interface for all repositories.
// Concrete repositories should embed this and add entity-specific methods.
type Repository interface {
	// Health checks if the underlying storage is reachable.
	Health(ctx context.Context) error
}

// BatchWithJokes bundles a batch with its jokes.
type BatchWithJokes struct {
	Batch domain.Batch
	Jokes []domain.Joke
}

// MarketItem represents a published joke in the marketplace.
type MarketItem struct {
	JokeID       int64
	JokeText     string
	TeamID       int64
	TeamName     string
	IsBoughtByMe bool
}

// TeamMember is a user assigned to a team with a role.
type TeamMember struct {
	UserID      int64
	DisplayName string
	Role        domain.Role
}

// LobbySnapshot captures lobby state for instructor.
type LobbySnapshot struct {
	RoundID    int64
	Summary    LobbySummary
	Teams      []LobbyTeam
	Customers  []LobbyCustomer
	Unassigned []LobbyUnassigned
}

// LobbySummary aggregates counts for lobby view.
type LobbySummary struct {
	Waiting       int
	Assigned      int
	Dropped       int
	TeamCount     int
	CustomerCount int
}

// LobbyTeam lists team members.
type LobbyTeam struct {
	Team    domain.Team
	Members []TeamMember
}

// LobbyCustomer represents a customer in the lobby.
type LobbyCustomer struct {
	UserID      int64
	DisplayName string
	Role        domain.Role
}

// LobbyUnassigned represents a waiting participant.
type LobbyUnassigned struct {
	UserID      int64
	DisplayName string
	Status      domain.ParticipantStatus
}

// TeamSummary aggregates stats for a team in a round.
type TeamSummary struct {
	Team            domain.Team
	RoundID         int64
	Rank            int
	Points          int
	TotalSales      int
	BatchesCreated  int
	BatchesRated    int
	AcceptedJokes   int
	AvgScoreOverall float64
	UnratedBatches  int
}

// TeamStats is used for instructor round stats.
type TeamStats struct {
	Rank            int
	Team            domain.Team
	Points          int
	TotalSales      int
	BatchesRated    int
	AvgScoreOverall float64
	AcceptedJokes   int
}

// CumulativeSalePoint represents sales growth over time.
type CumulativeSalePoint struct {
	EventIndex int       `json:"event_index"`
	Timestamp  time.Time `json:"timestamp"`
	TeamID     int64     `json:"team_id"`
	TeamName   string    `json:"team_name"`
	TotalSales int       `json:"total_sales"`
}

// BatchQualityPoint maps batch size to its average score.
type BatchQualityPoint struct {
	BatchID     int64      `json:"batch_id"`
	TeamID      int64      `json:"team_id"`
	TeamName    string     `json:"team_name"`
	SubmittedAt *time.Time `json:"submitted_at"`
	BatchSize   int        `json:"batch_size"`
	AvgScore    float64    `json:"avg_score"`
}

// LearningCurvePoint shows quality by submission order for each team.
type LearningCurvePoint struct {
	TeamID     int64   `json:"team_id"`
	TeamName   string  `json:"team_name"`
	BatchOrder int     `json:"batch_order"`
	AvgScore   float64 `json:"avg_score"`
}

// OutputRejectionPoint compares output volume vs rejection.
type OutputRejectionPoint struct {
	TeamID        int64   `json:"team_id"`
	TeamName      string  `json:"team_name"`
	TotalJokes    int     `json:"total_jokes"`
	RatedJokes    int     `json:"rated_jokes"`
	AcceptedJokes int     `json:"accepted_jokes"`
	RejectionRate float64 `json:"rejection_rate"`
}

// RevenueAcceptancePoint maps revenue to acceptance rate per team.
type RevenueAcceptancePoint struct {
	TeamID         int64   `json:"team_id"`
	TeamName       string  `json:"team_name"`
	TotalSales     int     `json:"total_sales"`
	AcceptedJokes  int     `json:"accepted_jokes"`
	AcceptanceRate float64 `json:"acceptance_rate"`
}

// RoundStats aggregates leaderboard plus chart data for instructor dashboard.
type RoundStats struct {
	RoundID             int64                    `json:"round_id"`
	Leaderboard         []TeamStats              `json:"leaderboard"`
	CumulativeSales     []CumulativeSalePoint    `json:"cumulative_sales"`
	BatchQualityBySize  []BatchQualityPoint      `json:"batch_quality_by_size"`
	LearningCurve       []LearningCurvePoint     `json:"learning_curve"`
	OutputVsRejection   []OutputRejectionPoint   `json:"output_vs_rejection"`
	RevenueVsAcceptance []RevenueAcceptancePoint `json:"revenue_vs_acceptance"`
}

// GameRepository is a composite repository covering all domain operations.
// The API surface mirrors the BE Schema v2 contract.
type GameRepository interface {
	Repository

	// Users & participants
	CreateUser(ctx context.Context, displayName string) (*domain.User, error)
	GetUserByDisplayName(ctx context.Context, displayName string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	EnsureParticipant(ctx context.Context, roundID, userID int64) (*domain.RoundParticipant, error)
	GetParticipant(ctx context.Context, roundID, userID int64) (*domain.RoundParticipant, error)
	UpdateUserAssignment(ctx context.Context, userID int64, role *domain.Role, teamID *int64) error
	UpdateParticipantStatus(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus) error
	MarkParticipantAssigned(ctx context.Context, roundID, userID int64) error
	ListParticipantsByStatus(ctx context.Context, roundID int64, status domain.ParticipantStatus) ([]domain.User, error)
	ListTeamMembers(ctx context.Context, teamID int64) ([]TeamMember, error)
	ListCustomers(ctx context.Context, roundID int64) ([]LobbyCustomer, error)

	// Teams
	EnsureTeamCount(ctx context.Context, teamCount int) ([]domain.Team, error)
	GetTeam(ctx context.Context, teamID int64) (*domain.Team, error)

	// Rounds
	GetActiveRound(ctx context.Context) (*domain.Round, error)
	GetRoundByID(ctx context.Context, roundID int64) (*domain.Round, error)
	GetLatestRound(ctx context.Context) (*domain.Round, error)
	UpdateRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error)
	InsertRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error)
	StartRound(ctx context.Context, roundID int64, customerBudget, batchSize int) (*domain.Round, error)
	EndRound(ctx context.Context, roundID int64) (*domain.Round, error)

	// Team round state
	EnsureTeamRoundState(ctx context.Context, roundID, teamID int64) error
	IncrementBatchCreated(ctx context.Context, roundID, teamID int64) error
	IncrementRatedStats(ctx context.Context, roundID, teamID int64, passesCount, pointsDelta int) error

	// Batches and jokes
	CreateBatch(ctx context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error)
	ListBatchesByTeam(ctx context.Context, roundID, teamID int64) ([]domain.Batch, error)
	GetBatchWithJokes(ctx context.Context, batchID int64) (*BatchWithJokes, error)
	GetNextBatchForQC(ctx context.Context, roundID, qcUserID int64) (*BatchWithJokes, int, error)
	RateBatch(ctx context.Context, batchID int64, qcUserID int64, ratings []domain.JokeRating, feedback *string) (*domain.Batch, []int64, error)
	CountSubmittedBatches(ctx context.Context, roundID int64) (int, error)

	// Market and budget
	EnsureCustomerBudget(ctx context.Context, roundID, customerID int64, starting int) (*domain.CustomerRoundBudget, error)
	ListMarket(ctx context.Context, roundID, customerID int64) ([]MarketItem, error)
	BuyJoke(ctx context.Context, roundID, customerID, jokeID int64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error)
	ReturnJoke(ctx context.Context, roundID, customerID, jokeID int64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error)

	// Stats
	GetTeamSummary(ctx context.Context, roundID, teamID int64) (*TeamSummary, error)
	GetLobby(ctx context.Context, roundID int64) (*LobbySnapshot, error)
	GetRoundStats(ctx context.Context, roundID int64) ([]TeamStats, error)
	GetRoundStatsV2(ctx context.Context, roundID int64) (*RoundStats, error)

	// Admin utilities
	ResetGame(ctx context.Context) error
}
