package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
)

// BatchHandler handles JM batch endpoints.
type BatchHandler struct {
	batchService *usecase.BatchService
}

func NewBatchHandler(batchService *usecase.BatchService) *BatchHandler {
	return &BatchHandler{batchService: batchService}
}

func (h *BatchHandler) Submit(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}

	var req dto.BatchSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	batch, err := h.batchService.Submit(c.Request.Context(), userID, roundID, req.TeamID, req.Jokes)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{
		"batch": gin.H{
			"batch_id":     batch.ID,
			"round_id":     batch.RoundID,
			"team_id":      batch.TeamID,
			"status":       batch.Status,
			"submitted_at": batch.SubmittedAt,
			"jokes_count":  len(req.Jokes),
		},
	})
}

func (h *BatchHandler) List(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
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

	batches, err := h.batchService.List(c.Request.Context(), roundID, teamID, userID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	var out []gin.H
	for _, b := range batches {
		var tagSummary []gin.H
		for _, ts := range b.TagSummary {
			tagSummary = append(tagSummary, gin.H{
				"tag":   ts.Tag,
				"count": ts.Count,
			})
		}
		var jokes []gin.H
		for _, j := range b.Jokes {
			jokes = append(jokes, gin.H{
				"joke_id":      j.ID,
				"joke_text":    j.Text,
				"is_published": j.IsPublished,
				"sold_count":   j.SoldCount,
			})
		}
		out = append(out, gin.H{
			"batch_id":     b.ID,
			"status":       b.Status,
			"submitted_at": b.SubmittedAt,
			"rated_at":     b.RatedAt,
			"avg_score":    b.AvgScore,
			"passes_count": b.PassesCount,
			"feedback":     b.Feedback,
			"tag_summary":  tagSummary,
			"jokes":        jokes,
		})
	}
	response.OK(c, gin.H{"batches": out})
}
