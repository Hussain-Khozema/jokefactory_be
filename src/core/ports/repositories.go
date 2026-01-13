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
	TeamLabel    string
	TeamProfit   float64
	TeamAccepted int
	TeamSold     int
	BoughtCount  int
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
	Profit          float64
	TotalSales      int
	Performance     string
	BatchesCreated  int
	BatchesRated    int
	AcceptedJokes   int
	UnsoldJokes     int
	SoldJokesCount  int
	AvgScoreOverall float64
	UnratedBatches  int
}

// TeamStats is used for instructor round stats.
type TeamStats struct {
	Rank            int         `json:"rank"`
	Team            domain.Team `json:"team"`
	BatchesRated    int         `json:"batches_rated"`
	TotalSales      int         `json:"total_sales"`
	AcceptedJokes   int         `json:"accepted_jokes"`
	UnacceptedJokes int         `json:"unaccepted_jokes"`
	AvgScoreOverall float64     `json:"avg_score_overall"`
	TotalJokes      int         `json:"total_jokes"`
	UnsoldJokes     int         `json:"unsold_jokes"`
	Profit          float64     `json:"profit"`
}

// SalesPoint represents cumulative points (sales) growth over time per team.
type SalesPoint struct {
	EventIndex       int       `json:"event_index"`
	TeamEventIndex   int       `json:"team_event_index"`
	Timestamp        time.Time `json:"timestamp"`
	TeamID           int64     `json:"team_id"`
	TeamName         string    `json:"team_name"`
	CumulativePoints int       `json:"cumulative_points"`
}

// BatchSequencePoint shows average score by batch submission order for a team.
type BatchSequencePoint struct {
	RoundID     int64   `json:"round_id"`
	RoundNumber int     `json:"round_number"`
	TeamID      int64   `json:"team_id"`
	TeamName    string  `json:"team_name"`
	BatchOrder  int     `json:"batch_order"`
	AvgScore    float64 `json:"avg_score"`
}

// BatchSizeQualityPoint maps batch size to average score, used for round comparison.
type BatchSizeQualityPoint struct {
	RoundID     int64   `json:"round_id"`
	RoundNumber int     `json:"round_number"`
	TeamID      int64   `json:"team_id"`
	TeamName    string  `json:"team_name"`
	BatchSize   int     `json:"batch_size"`
	AvgScore    float64 `json:"avg_score"`
}

// TeamRejectionPoint represents rejection-related metrics per team for charts.
type TeamRejectionPoint struct {
	TeamID          int64   `json:"team_id"`
	TeamName        string  `json:"team_name"`
	UnacceptedJokes int     `json:"unaccepted_jokes"`
	RejectionRate   float64 `json:"rejection_rate"`
}

// RoundStats aggregates leaderboard plus chart data for instructor dashboard.
type RoundStats struct {
	RoundID              int64                   `json:"round_id"`
	Leaderboard          []TeamStats             `json:"leaderboard"`
	RejectionByTeam      []TeamRejectionPoint    `json:"rejection_by_team"`
	SalesOverTime        []SalesPoint            `json:"sales_over_time"`
	BatchSequenceQuality []BatchSequencePoint    `json:"batch_sequence_quality"`
	BatchSizeQuality     []BatchSizeQualityPoint `json:"batch_size_quality"`
}

// GameRepository is a composite repository covering all domain operations.
// The API surface mirrors the BE Schema v2 contract.
type GameRepository interface {
	Repository

	// Users & participants
	CreateUser(ctx context.Context, displayName string) (*domain.User, error)
	GetUserByDisplayName(ctx context.Context, displayName string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	UpdateUserAssignment(ctx context.Context, userID int64, role *domain.Role, teamID *int64) error
	UpdateUserStatus(ctx context.Context, userID int64, status domain.ParticipantStatus) error
	// PatchUserInRound updates user role/team/status and keeps customer budget consistent
	// for the given round (create budget row when becoming CUSTOMER; delete when leaving CUSTOMER).
	//
	// Implementation detail: should be atomic (single DB transaction).
	PatchUserInRound(ctx context.Context, roundID, userID int64, status domain.ParticipantStatus, role *domain.Role, teamID *int64) error
	MarkUserAssigned(ctx context.Context, userID int64) error
	ListUsersByStatus(ctx context.Context, status domain.ParticipantStatus) ([]domain.User, error)
	ListTeamMembers(ctx context.Context, teamID int64) ([]TeamMember, error)
	ListCustomers(ctx context.Context) ([]LobbyCustomer, error)
	DeleteUser(ctx context.Context, userID int64) error

	// Teams
	EnsureTeamCount(ctx context.Context, teamCount int) ([]domain.Team, error)
	GetTeam(ctx context.Context, teamID int64) (*domain.Team, error)

	// Rounds
	GetActiveRound(ctx context.Context) (*domain.Round, error)
	GetRoundByID(ctx context.Context, roundID int64) (*domain.Round, error)
	GetLatestRound(ctx context.Context) (*domain.Round, error)
	ListRounds(ctx context.Context) ([]domain.Round, error)
	UpdateRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int, marketPrice, costOfPublishing float64) (*domain.Round, error)
	InsertRoundConfig(ctx context.Context, roundID int64, customerBudget, batchSize int, marketPrice, costOfPublishing float64) (*domain.Round, error)
	StartRound(ctx context.Context, roundID int64, customerBudget, batchSize int, marketPrice, costOfPublishing float64) (*domain.Round, error)
	EndRound(ctx context.Context, roundID int64) (*domain.Round, error)
	SetRoundPopupState(ctx context.Context, roundID int64, isActive bool) (*domain.Round, error)

	// Team round state
	EnsureTeamRoundState(ctx context.Context, roundID, teamID int64) error
	IncrementBatchCreated(ctx context.Context, roundID, teamID int64) error
	IncrementRatedStats(ctx context.Context, roundID, teamID int64, passesCount, pointsDelta int) error

	// Batches and jokes
	CreateBatch(ctx context.Context, roundID, teamID int64, jokes []string) (*domain.Batch, error)
	ListBatchesByTeam(ctx context.Context, roundID, teamID int64) ([]domain.Batch, error)
	GetBatchWithJokes(ctx context.Context, batchID int64) (*BatchWithJokes, error)
	GetNextBatchForQC(ctx context.Context, roundID, qcUserID, teamID int64) (*BatchWithJokes, int, error)
	RateBatch(ctx context.Context, batchID int64, qcUserID int64, ratings []domain.JokeRating, feedback *string) (*domain.Batch, []int64, error)
	CountSubmittedBatches(ctx context.Context, roundID int64) (int, error)

	// Market and budget
	EnsureCustomerBudget(ctx context.Context, roundID, customerID int64, starting int) (*domain.CustomerRoundBudget, error)
	ListMarket(ctx context.Context, roundID, customerID int64) ([]MarketItem, error)
	BuyJoke(ctx context.Context, roundID, customerID, jokeID int64, marketPrice float64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error)
	ReturnJoke(ctx context.Context, roundID, customerID, jokeID int64, marketPrice float64) (*domain.Purchase, *domain.CustomerRoundBudget, int64, error)

	// Stats
	GetTeamSummary(ctx context.Context, roundID, teamID int64) (*TeamSummary, error)
	GetLobby(ctx context.Context, roundID int64) (*LobbySnapshot, error)
	GetRoundStats(ctx context.Context, roundID int64) ([]TeamStats, error)
	GetRoundStatsV2(ctx context.Context, roundID int64) (*RoundStats, error)

	// Admin utilities
	ResetGame(ctx context.Context) error
}
