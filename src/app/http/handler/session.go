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
		// Attach error for middleware logging
		c.Error(err)
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"user_id":      res.User.ID,
			"display_name": res.User.DisplayName,
		},
		"participant": gin.H{
			"status":      res.User.Status,
			"joined_at":   res.User.JoinedAt,
			"assigned_at": res.User.AssignedAt,
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

	participant := gin.H{
		"status":      res.User.Status,
		"joined_at":   res.User.JoinedAt,
		"assigned_at": res.User.AssignedAt,
	}

	teammates := make([]gin.H, 0, len(res.Teammates))
	for _, tm := range res.Teammates {
		teammates = append(teammates, gin.H{
			"user_id":      tm.UserID,
			"display_name": tm.DisplayName,
			"role":         tm.Role,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"user_id":      res.User.ID,
			"display_name": res.User.DisplayName,
		},
		"participant": participant,
		"assignment": gin.H{
			"role":    role,
			"team_id": team,
		},
		"teammates": teammates,
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

