package dto

import (
	"jokefactory/src/core/domain"
)

// PublicRound is the student-safe round projection.
// Hides buy_threshold, jitter, swap_margin, and ideal_profile.
type PublicRound struct {
	ID                int64   `json:"id"`
	RoundNumber       int     `json:"round_number"`
	Status            string  `json:"status"`
	BatchSize         int     `json:"batch_size"`
	MaxBatchSize      int     `json:"max_batch_size"`
	CustomerBudget    float64 `json:"customer_budget"`
	MarketPrice       float64 `json:"market_price"`
	CostOfPublishing  float64 `json:"cost_of_publishing"`
	CostOfDiscard     float64 `json:"cost_of_discard"`
	CustomerCount     int     `json:"customer_count"`
	FeedbackJokeCount int     `json:"feedback_joke_count"`
	StartedAt         any     `json:"started_at"`
	EndedAt           any     `json:"ended_at"`
	IsPoppedActive    bool    `json:"is_popped_active"`
}

// InstructorRound includes engine knobs instructors may tune.
type InstructorRound struct {
	PublicRound
	BuyThreshold          float64           `json:"buy_threshold"`
	Jitter                float64           `json:"jitter"`
	SwapMargin            float64           `json:"swap_margin"`
	FeedbackPassThreshold float64           `json:"feedback_pass_threshold"`
	IdealProfile          map[string]string `json:"ideal_profile,omitempty"`
}

// ToPublicRound maps a domain round for student endpoints.
func ToPublicRound(rd *domain.Round) PublicRound {
	maxBatch := rd.BatchSize
	if rd.RoundNumber >= 2 {
		maxBatch = rd.BatchSize
	}
	return PublicRound{
		ID:                rd.ID,
		RoundNumber:       rd.RoundNumber,
		Status:            string(rd.Status),
		BatchSize:         rd.BatchSize,
		MaxBatchSize:      maxBatch,
		CustomerBudget:    rd.CustomerBudget,
		MarketPrice:       rd.MarketPrice,
		CostOfPublishing:  rd.CostOfPublishing,
		CostOfDiscard:     rd.CostOfDiscard,
		CustomerCount:     rd.CustomerCount,
		FeedbackJokeCount: rd.FeedbackJokeCount,
		StartedAt:         rd.StartedAt,
		EndedAt:           rd.EndedAt,
		IsPoppedActive:    rd.IsPoppedActive,
	}
}

// ToInstructorRound maps a domain round plus optional ideal profile.
func ToInstructorRound(rd *domain.Round, profile domain.IdealProfile) InstructorRound {
	return InstructorRound{
		PublicRound:           ToPublicRound(rd),
		BuyThreshold:          rd.BuyThreshold,
		Jitter:                rd.Jitter,
		SwapMargin:            rd.SwapMargin,
		FeedbackPassThreshold: rd.FeedbackPassThreshold,
		IdealProfile:          IdealProfileFromDomain(profile),
	}
}
