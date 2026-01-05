package dto

// SessionJoinRequest is the payload for /v1/session/join.
type SessionJoinRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
}

// BatchSubmitRequest is the payload for submitting a batch.
type BatchSubmitRequest struct {
	TeamID int64    `json:"team_id" binding:"required"`
	Jokes  []string `json:"jokes" binding:"required"`
}

// RatingsRequest captures QC ratings submission.
type RatingsRequest struct {
	Ratings  []RatingEntry `json:"ratings" binding:"required"`
	Feedback *string       `json:"feedback"`
}

// RatingEntry holds a single joke rating.
type RatingEntry struct {
	JokeID int64  `json:"joke_id" binding:"required"`
	Rating int    `json:"rating" binding:"required"`
	Tag    string `json:"tag" binding:"required"`
}

// AssignRequest is used for instructor assign endpoint.
type AssignRequest struct {
	CustomerCount int `json:"customer_count" binding:"required"`
	TeamCount     int `json:"team_count" binding:"required"`
}

// PatchUserRequest is used for instructor patch user endpoint.
type PatchUserRequest struct {
	Status string  `json:"status" binding:"required"`
	Role   *string `json:"role"`
	TeamID *int64  `json:"team_id"`
}

// ConfigRequest updates round configuration.
type ConfigRequest struct {
	CustomerBudget     int     `json:"customer_budget" binding:"required"`
	BatchSize          *int    `json:"batch_size"`
	UnsoldJokesPenalty float64 `json:"unsold_jokes_penalty" binding:"required"`
}
