package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/ports"
	"jokefactory/src/core/usecase"
)

// MarketingHandler handles Marketing queue and publish endpoints.
type MarketingHandler struct {
	marketingService *usecase.MarketingService
}

func NewMarketingHandler(marketingService *usecase.MarketingService) *MarketingHandler {
	return &MarketingHandler{marketingService: marketingService}
}

func (h *MarketingHandler) QueueNext(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Query("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}

	item, err := h.marketingService.QueueNext(c.Request.Context(), userID, roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	if item.Batch.ID == 0 {
		response.OK(c, gin.H{
			"batch":      nil,
			"jokes":      []gin.H{},
			"queue_size": item.QueueSize,
		})
		return
	}

	jokes := make([]gin.H, 0, len(item.Jokes))
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
			"status":       item.Batch.Status,
			"submitted_at": item.Batch.SubmittedAt,
			"locked_at":    item.Batch.LockedAt,
			"locked_by":    item.Batch.LockedBy,
		},
		"jokes":      jokes,
		"queue_size": item.QueueSize,
	})
}

func (h *MarketingHandler) Publish(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	batchID, err := strconv.ParseInt(c.Param("batch_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid batch id", middleware.GetRequestID(c))
		return
	}

	var req dto.PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	decisions := make([]ports.JokePublishDecision, 0, len(req.Jokes))
	for _, j := range req.Jokes {
		decisions = append(decisions, ports.JokePublishDecision{
			JokeID:      j.JokeID,
			Title:       j.JokeTitle,
			IsPublished: j.IsPublished,
		})
	}

	result, err := h.marketingService.Publish(c.Request.Context(), userID, batchID, decisions)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{
		"batch": gin.H{
			"batch_id":     result.Batch.ID,
			"status":       result.Batch.Status,
			"processed_at": result.Batch.ProcessedAt,
		},
		"published": gin.H{
			"count":    len(result.PublishedIDs),
			"joke_ids": result.PublishedIDs,
		},
		"discarded": gin.H{
			"count":    len(result.DiscardedIDs),
			"joke_ids": result.DiscardedIDs,
		},
	})
}

func (h *MarketingHandler) QueueCount(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Query("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	count, err := h.marketingService.QueueCount(c.Request.Context(), userID, roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{"queue_size": count})
}
