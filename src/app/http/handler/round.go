package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
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

	out := make([]dto.PublicRound, 0, len(rounds))
	for i := range rounds {
		out = append(out, dto.ToPublicRound(&rounds[i]))
	}

	response.OK(c, gin.H{"rounds": out})
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
		"round_id":            summary.RoundID,
		"rank":                summary.Rank,
		"points":              summary.Points,
		"profit":              summary.Profit,
		"total_sales":         summary.TotalSales,
		"performance_label":   summary.Performance,
		"unsold_jokes":        summary.UnsoldJokes,
		"sold_jokes_count":    summary.SoldJokesCount,
		"batches_created":     summary.BatchesCreated,
		"batches_processed":   summary.BatchesProcessed,
		"published_jokes":     summary.PublishedJokes,
		"discarded_jokes":     summary.DiscardedJokes,
		"unprocessed_batches": summary.UnprocessedBatches,
	})
}
