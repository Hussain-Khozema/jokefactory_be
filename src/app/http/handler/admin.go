package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/usecase"
)

// AdminHandler handles instructor/admin login.
type AdminHandler struct {
	adminService *usecase.AdminAuthService
}

func NewAdminHandler(adminService *usecase.AdminAuthService) *AdminHandler {
	return &AdminHandler{adminService: adminService}
}

func (h *AdminHandler) Login(c *gin.Context) {
	var req dto.AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	res, err := h.adminService.Login(c.Request.Context(), req.DisplayName, req.Password)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	var roundID interface{}
	if res.Round != nil {
		roundID = res.Round.ID
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"user_id":      res.User.ID,
			"display_name": res.User.DisplayName,
			"role":         res.User.Role,
		},
		"round_id": roundID,
	})
}

// ResetGame clears all game data. Protected by the admin password.
func (h *AdminHandler) ResetGame(c *gin.Context) {
	if err := h.adminService.ResetGame(c.Request.Context()); err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{
		"status":  "reset",
		"message": "all game data cleared",
	})
}
