package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
)

// RoundHandler handles round-related endpoints.
type RoundHandler struct {
	roundService *usecase.RoundService
}

func NewRoundHandler(roundService *usecase.RoundService) *RoundHandler {
	return &RoundHandler{roundService: roundService}
}

func (h *RoundHandler) Active(c *gin.Context) {
	rounds, err := h.roundService.List(c.Request.Context())
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	resp := make([]gin.H, 0, len(rounds))
	for _, rd := range rounds {
		maxBatchSize := rd.BatchSize
		if rd.RoundNumber == 2 {
			maxBatchSize = 10
		}
		resp = append(resp, gin.H{
			"id":                   rd.ID,
			"round_number":         rd.RoundNumber,
			"status":               rd.Status,
			"batch_size":           rd.BatchSize,
			"max_batch_size":       maxBatchSize,
			"customer_budget":      rd.CustomerBudget,
			"market_price":         rd.MarketPrice,
			"cost_of_publishing":   rd.CostOfPublishing,
			"started_at":           rd.StartedAt,
			"ended_at":             rd.EndedAt,
			"is_popped_active":     rd.IsPoppedActive,
		})
	}
	response.OK(c, gin.H{"rounds": resp})
}

func (h *RoundHandler) TeamSummary(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	teamID, err := strconv.ParseInt(c.Param("team_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid team id", middleware.GetRequestID(c))
		return
	}

	summary, err := h.roundService.TeamSummary(c.Request.Context(), roundID, teamID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{
		"team": gin.H{
			"id":   summary.Team.ID,
			"name": summary.Team.Name,
		},
		"round_id":          summary.RoundID,
		"rank":              summary.Rank,
		"points":            summary.Points,
		"profit":            summary.Profit,
		"total_sales":       summary.TotalSales,
		"performance_label": summary.Performance,
		"unsold_jokes":      summary.UnsoldJokes,
		"sold_jokes_count":  summary.SoldJokesCount,
		"batches_created":   summary.BatchesCreated,
		"batches_rated":     summary.BatchesRated,
		"accepted_jokes":    summary.AcceptedJokes,
		"avg_score_overall": summary.AvgScoreOverall,
		"unrated_batches":   summary.UnratedBatches,
	})
}
