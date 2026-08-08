package domain

import "time"

type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

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

type Round struct {
	ID                    int64
	RoundNumber           int
	Status                RoundStatus
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
	StartedAt             *time.Time
	EndedAt               *time.Time
	CreatedAt             time.Time
	IsPoppedActive        bool
}

type TeamRoundState struct {
	RoundID          int64
	TeamID           int64
	PointsEarned     int
	BatchesCreated   int
	BatchesProcessed int
	PublishedJokes   int
	DiscardedJokes   int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Batch struct {
	ID          int64
	RoundID     int64
	TeamID      int64
	Status      BatchStatus
	SubmittedAt *time.Time
	ProcessedAt *time.Time
	LockedAt    *time.Time
	LockedBy    *int64
	CreatedAt   time.Time
	Jokes       []Joke
}

type Joke struct {
	ID            int64
	BatchID       int64
	Text          string
	Title         *string
	PublishStatus PublishStatus
	PublishedAt   *time.Time
	CreatedAt     time.Time
	SoldCount     int
}

// AICustomer is a simulated buyer for a round.
type AICustomer struct {
	ID                int64
	RoundID           int64
	PersonalThreshold float64
	StartingBudget    float64
	RemainingBudget   float64
	CreatedAt         time.Time
}

// Purchase is one AI customer's holding of a published joke.
type Purchase struct {
	ID           int64
	RoundID      int64
	AICustomerID int64
	JokeID       int64
	TeamID       int64
	Price        float64
	CreatedAt    time.Time
}
