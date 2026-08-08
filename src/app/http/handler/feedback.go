package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
)

// FeedbackHandler serves curated Good/Improve feedback for a team.
type FeedbackHandler struct {
	feedbackService *usecase.FeedbackService
}

func NewFeedbackHandler(feedbackService *usecase.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{feedbackService: feedbackService}
}

func (h *FeedbackHandler) Get(c *gin.Context) {
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

	items, err := h.feedbackService.Get(c.Request.Context(), roundID, teamID, userID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	jokes := make([]dto.FeedbackJoke, 0, len(items))
	for _, item := range items {
		jokes = append(jokes, dto.FeedbackJoke{
			JokeID:            item.JokeID,
			JokeTitle:         item.JokeTitle,
			WasBought:         item.WasBought,
			GoodDimensions:    item.GoodDimensions,
			ImproveDimensions: item.ImproveDimensions,
		})
	}
	response.OK(c, gin.H{"jokes": jokes})
}
