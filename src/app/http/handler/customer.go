package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
)

// CustomerHandler serves the market board (AI-customer driven).
type CustomerHandler struct {
	ai *usecase.AICustomerService
}

func NewCustomerHandler(ai *usecase.AICustomerService) *CustomerHandler {
	return &CustomerHandler{ai: ai}
}

func (h *CustomerHandler) Market(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	if h.ai == nil {
		response.FromDomainError(c, domain.NewConflictError("AI customer engine not configured"), middleware.GetRequestID(c))
		return
	}
	items, err := h.ai.Market(c.Request.Context(), userID, roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		title := ""
		if it.JokeTitle != nil {
			title = *it.JokeTitle
		}
		item := gin.H{
			"joke_id":    it.JokeID,
			"joke_text":  it.JokeText,
			"joke_title": title,
			"team_id":    it.TeamID,
			"team_name":  it.TeamName,
			"sold_count": it.SoldCount,
		}
		if it.PublishedAt != nil {
			item["published_at"] = it.PublishedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	response.OK(c, gin.H{"items": out})
}
