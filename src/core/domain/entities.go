package domain

import "time"

// Role represents a user's role in the game.
type Role string

const (
	RoleInstructor Role = "INSTRUCTOR"
	RoleJM         Role = "JM"
	RoleQC         Role = "QC"
	RoleCustomer   Role = "CUSTOMER"
)

// QCTag represents the single tag assigned per joke by QC.
type QCTag string

const (
	QCTagExcellentStandout QCTag = "EXCELLENT_STANDOUT"
	QCTagGenuinelyFunny    QCTag = "GENUINELY_FUNNY"
	QCTagMadeMeSmile       QCTag = "MADE_ME_SMILE"
	QCTagOriginalIdea      QCTag = "ORIGINAL_IDEA"
	QCTagPoliteSmile       QCTag = "POLITE_SMILE"
	QCTagDidntLand         QCTag = "DIDNT_LAND"
	QCTagNotAcceptable     QCTag = "NOT_ACCEPTABLE"
	QCTagOther             QCTag = "OTHER"
)

// ParticipantStatus indicates lobby assignment state.
type ParticipantStatus string

const (
	ParticipantWaiting  ParticipantStatus = "WAITING"
	ParticipantAssigned ParticipantStatus = "ASSIGNED"
)

// RoundStatus represents lifecycle of a round.
type RoundStatus string

const (
	RoundConfigured RoundStatus = "CONFIGURED"
	RoundActive     RoundStatus = "ACTIVE"
	RoundEnded      RoundStatus = "ENDED"
)

// BatchStatus represents lifecycle of a batch.
type BatchStatus string

const (
	BatchDraft     BatchStatus = "DRAFT"
	BatchSubmitted BatchStatus = "SUBMITTED"
	BatchRated     BatchStatus = "RATED"
)

// Team represents a team.
type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// User represents a player.
type User struct {
	ID          int64
	DisplayName string
	Role        *Role
	TeamID      *int64
	Status      ParticipantStatus
	AssignedAt  *time.Time
	JoinedAt    time.Time
	CreatedAt   time.Time
}

// Round represents a game session.
type Round struct {
	ID                 int64
	RoundNumber        int
	Status             RoundStatus
	CustomerBudget     int
	BatchSize          int
	UnsoldJokesPenalty float64
	StartedAt          *time.Time
	EndedAt            *time.Time
	CreatedAt          time.Time
	IsPoppedActive     bool
}

// TeamRoundState tracks per-team stats for a round.
type TeamRoundState struct {
	RoundID        int64
	TeamID         int64
	PointsEarned   int
	BatchesCreated int
	BatchesRated   int
	AcceptedJokes  int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Batch represents a submission from a team.
type Batch struct {
	ID          int64
	RoundID     int64
	TeamID      int64
	Status      BatchStatus
	SubmittedAt *time.Time
	RatedAt     *time.Time
	AvgScore    *float64
	PassesCount *int
	Feedback    *string
	LockedAt    *time.Time
	CreatedAt   time.Time
	TagSummary  []TagCount
	Jokes       []Joke
}

// Joke represents a joke in a batch.
type Joke struct {
	ID        int64
	BatchID   int64
	Text      string
	CreatedAt time.Time
}

// JokeRating represents QC rating.
type JokeRating struct {
	JokeID   int64
	QCUserID int64
	Rating   int
	Tag      QCTag
	RatedAt  time.Time
}

// TagCount aggregates tag counts per batch.
type TagCount struct {
	Tag   QCTag
	Count int
}

// PublishedJoke represents a joke published to market.
type PublishedJoke struct {
	JokeID    int64
	RoundID   int64
	TeamID    int64
	CreatedAt time.Time
}

// CustomerRoundBudget tracks a customer's budget for a round.
type CustomerRoundBudget struct {
	RoundID         int64
	CustomerUserID  int64
	StartingBudget  int
	RemainingBudget int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Purchase represents a purchase of a joke.
type Purchase struct {
	ID             int64
	RoundID        int64
	CustomerUserID int64
	JokeID         int64
	CreatedAt      time.Time
}
