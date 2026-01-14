package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
)

// CustomerHandler handles customer endpoints.
type CustomerHandler struct {
	customerService *usecase.CustomerService
}

func NewCustomerHandler(customerService *usecase.CustomerService) *CustomerHandler {
	return &CustomerHandler{customerService: customerService}
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
	items, err := h.customerService.Market(c.Request.Context(), userID, roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	var out []gin.H
	for _, item := range items {
		out = append(out, gin.H{
			"joke_id":         item.JokeID,
			"joke_text":       item.JokeText,
			"joke_title":      item.JokeTitle,
			"team": gin.H{
				"id":                item.TeamID,
				"name":              item.TeamName,
				"performance_label": item.TeamLabel,
				"accepted_jokes":    item.TeamAccepted,
				"sold_jokes_count":  item.TeamSold,
				"profit":            item.TeamProfit,
			},
			"bought_count":    item.BoughtCount,
			"is_bought_by_me": item.IsBoughtByMe,
		})
	}
	response.OK(c, gin.H{"items": out})
}

func (h *CustomerHandler) Budget(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	budget, err := h.customerService.Budget(c.Request.Context(), userID, roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{
		"round_id":         budget.RoundID,
		"starting_budget":  budget.StartingBudget,
		"remaining_budget": budget.RemainingBudget,
	})
}

func (h *CustomerHandler) Buy(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	jokeID, err := strconv.ParseInt(c.Param("joke_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid joke id", middleware.GetRequestID(c))
		return
	}
	purchase, budget, teamID, err := h.customerService.Buy(c.Request.Context(), userID, roundID, jokeID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{
		"purchase": gin.H{
			"purchase_id": purchase.ID,
			"joke_id":     purchase.JokeID,
		},
		"budget": gin.H{
			"starting_budget":  budget.StartingBudget,
			"remaining_budget": budget.RemainingBudget,
		},
		"team_points_awarded": gin.H{
			"team_id":      teamID,
			"points_delta": 1,
		},
	})
}

func (h *CustomerHandler) Return(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	jokeID, err := strconv.ParseInt(c.Param("joke_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid joke id", middleware.GetRequestID(c))
		return
	}
	purchase, budget, teamID, err := h.customerService.Return(c.Request.Context(), userID, roundID, jokeID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{
		"purchase": gin.H{
			"purchase_id": purchase.ID,
			"joke_id":     purchase.JokeID,
		},
		"budget": gin.H{
			"starting_budget":  budget.StartingBudget,
			"remaining_budget": budget.RemainingBudget,
		},
		"team_points_awarded": gin.H{
			"team_id":      teamID,
			"points_delta": -1,
		},
	})
}
