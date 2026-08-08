package dto

import "jokefactory/src/core/domain"

// SessionJoinRequest is the payload for /v1/session/join.
type SessionJoinRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
}

// BatchSubmitRequest is the payload for submitting a batch.
type BatchSubmitRequest struct {
	TeamID int64    `json:"team_id" binding:"required"`
	Jokes  []string `json:"jokes" binding:"required"`
}

// AssignRequest is used for instructor assign endpoint.
type AssignRequest struct {
	TeamCount int `json:"team_count" binding:"required"`
}

// PatchUserRequest is used for instructor patch user endpoint.
type PatchUserRequest struct {
	Status string  `json:"status" binding:"required"`
	Role   *string `json:"role"`
	TeamID *int64  `json:"team_id"`
}

// PublishJokeDecision is one joke's Marketing publish/discard choice.
type PublishJokeDecision struct {
	JokeID      int64  `json:"joke_id" binding:"required"`
	JokeTitle   string `json:"joke_title"`
	IsPublished bool   `json:"is_published"`
}

// PublishRequest is the payload for POST /marketing/batches/{id}/publish.
type PublishRequest struct {
	Jokes []PublishJokeDecision `json:"jokes" binding:"required"`
}

// FeedbackJoke is one curated joke entry on GET .../feedback.
type FeedbackJoke struct {
	JokeID            int64    `json:"joke_id"`
	JokeTitle         string   `json:"joke_title"`
	WasBought         bool     `json:"was_bought"`
	GoodDimensions    []string `json:"good_dimensions"`
	ImproveDimensions []string `json:"improve_dimensions"`
}

// ConfigRequest updates round configuration (instructor).
// All fields are optional; omitted values keep the existing round / defaults.
type ConfigRequest struct {
	CustomerBudget        *float64          `json:"customer_budget"`
	BatchSize             *int              `json:"batch_size"`
	MarketPrice           *float64          `json:"market_price"`
	CostOfPublishing      *float64          `json:"cost_of_publishing"`
	CostOfDiscard         *float64          `json:"cost_of_discard"`
	CustomerCount         *int              `json:"customer_count"`
	BuyThreshold          *float64          `json:"buy_threshold"`
	Jitter                *float64          `json:"jitter"`
	SwapMargin            *float64          `json:"swap_margin"`
	FeedbackJokeCount     *int              `json:"feedback_joke_count"`
	FeedbackPassThreshold *float64          `json:"feedback_pass_threshold"`
	IdealProfile          map[string]string `json:"ideal_profile"`
}

// IdealProfileToDomain converts a string-keyed map to domain.IdealProfile.
func IdealProfileToDomain(raw map[string]string) domain.IdealProfile {
	if raw == nil {
		return nil
	}
	out := make(domain.IdealProfile, len(raw))
	for k, v := range raw {
		out[domain.Dimension(k)] = v
	}
	return out
}

// IdealProfileFromDomain converts domain.IdealProfile to a JSON-friendly map.
func IdealProfileFromDomain(profile domain.IdealProfile) map[string]string {
	if profile == nil {
		return nil
	}
	out := make(map[string]string, len(profile))
	for k, v := range profile {
		out[string(k)] = v
	}
	return out
}
