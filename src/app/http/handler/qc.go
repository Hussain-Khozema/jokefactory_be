package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
)

// QCHandler handles quality control endpoints.
type QCHandler struct {
	qcService *usecase.QCService
}

func NewQCHandler(qcService *usecase.QCService) *QCHandler {
	return &QCHandler{qcService: qcService}
}

func (h *QCHandler) QueueNext(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundIDStr := c.Query("round_id")
	roundID, err := strconv.ParseInt(roundIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	item, err := h.qcService.Next(c.Request.Context(), userID, roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	var jokes []gin.H
	for _, j := range item.Jokes {
		jokes = append(jokes, gin.H{
			"joke_id":   j.ID,
			"joke_text": j.Text,
		})
	}
	response.OK(c, gin.H{
		"batch": gin.H{
			"batch_id":     item.Batch.ID,
			"round_id":     item.Batch.RoundID,
			"team_id":      item.Batch.TeamID,
			"submitted_at": item.Batch.SubmittedAt,
		},
		"jokes":      jokes,
		"queue_size": item.QueueSize,
	})
}

func (h *QCHandler) SubmitRatings(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	batchID, err := strconv.ParseInt(c.Param("batch_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid batch id", middleware.GetRequestID(c))
		return
	}

	var req dto.RatingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}
	var ratings []domain.JokeRating
	for _, r := range req.Ratings {
		var title *string
		if r.JokeTitle != nil {
			t := strings.TrimSpace(*r.JokeTitle)
			if t != "" {
				title = &t
			}
		}
		ratings = append(ratings, domain.JokeRating{
			JokeID:   r.JokeID,
			QCUserID: userID,
			Rating:   r.Rating,
			Tag:      domain.QCTag(r.Tag),
			JokeTitle: title,
		})
	}

	batch, published, err := h.qcService.Rate(c.Request.Context(), userID, batchID, ratings, req.Feedback)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{
		"batch": gin.H{
			"batch_id":    batch.ID,
			"status":      batch.Status,
			"rated_at":    batch.RatedAt,
			"avg_score":   batch.AvgScore,
			"passes_count": batch.PassesCount,
			"feedback":    batch.Feedback,
		},
		"published": gin.H{
			"count":    len(published),
			"joke_ids": published,
		},
	})
}

func (h *QCHandler) QueueCount(c *gin.Context) {
	roundIDStr := c.Query("round_id")
	roundID, err := strconv.ParseInt(roundIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	count, err := h.qcService.QueueCount(c.Request.Context(), roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{"queue_size": count})
}

