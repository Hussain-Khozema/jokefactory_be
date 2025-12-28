package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
)

// SessionHandler handles session endpoints.
type SessionHandler struct {
	sessionService *usecase.SessionService
}

func NewSessionHandler(sessionService *usecase.SessionService) *SessionHandler {
	return &SessionHandler{sessionService: sessionService}
}

func (h *SessionHandler) Join(c *gin.Context) {
	var req dto.SessionJoinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	res, err := h.sessionService.Join(c.Request.Context(), req.DisplayName)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"user_id":      res.User.ID,
			"display_name": res.User.DisplayName,
		},
		"round_id": res.Round.ID,
		"participant": gin.H{
			"status":      res.Participant.Status,
			"joined_at":   res.Participant.JoinedAt,
			"assigned_at": res.Participant.AssignedAt,
		},
	})
}

func (h *SessionHandler) Me(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	res, err := h.sessionService.Me(c.Request.Context(), userID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	var role interface{}
	var team interface{}
	if res.User.Role != nil {
		role = *res.User.Role
	}
	if res.User.TeamID != nil {
		team = *res.User.TeamID
	}

	var participant interface{}
	if res.Participant != nil {
		participant = gin.H{
			"status":      res.Participant.Status,
			"joined_at":   res.Participant.JoinedAt,
			"assigned_at": res.Participant.AssignedAt,
		}
	} else {
		participant = nil
	}

	var round interface{}
	if res.Round != nil {
		round = gin.H{
			"id":              res.Round.ID,
			"round_number":    res.Round.RoundNumber,
			"status":          res.Round.Status,
			"customer_budget": res.Round.CustomerBudget,
			"batch_size":      res.Round.BatchSize,
			"started_at":      res.Round.StartedAt,
			"ended_at":        res.Round.EndedAt,
		}
	} else {
		round = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"user_id":      res.User.ID,
			"display_name": res.User.DisplayName,
		},
		"round_id": res.Round.ID,
		"round":    round,
		"participant": participant,
		"assignment": gin.H{
			"role":    role,
			"team_id": team,
		},
	})
}

func parseUserID(c *gin.Context) (int64, bool) {
	raw := c.GetHeader("X-User-Id")
	if raw == "" {
		response.BadRequest(c, "missing X-User-Id header", middleware.GetRequestID(c))
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid X-User-Id header", middleware.GetRequestID(c))
		return 0, false
	}
	return id, true
}

