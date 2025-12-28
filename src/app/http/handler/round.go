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
	round, err := h.roundService.Active(c.Request.Context())
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	if round == nil {
		response.OK(c, gin.H{"round": nil})
		return
	}
	response.OK(c, gin.H{"round": gin.H{
		"id":             round.ID,
		"round_number":   round.RoundNumber,
		"status":         round.Status,
		"batch_size":     round.BatchSize,
		"customer_budget": round.CustomerBudget,
		"started_at":     round.StartedAt,
	}})
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
		"total_sales":       summary.TotalSales,
		"batches_created":   summary.BatchesCreated,
		"batches_rated":     summary.BatchesRated,
		"accepted_jokes":    summary.AcceptedJokes,
		"avg_score_overall": summary.AvgScoreOverall,
		"unrated_batches":   summary.UnratedBatches,
	})
}

