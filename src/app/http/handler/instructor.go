package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"jokefactory/src/app/http/dto"
	"jokefactory/src/app/http/response"
	"jokefactory/src/app/middleware"
	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
)

// InstructorHandler handles instructor endpoints.
type InstructorHandler struct {
	instructorService *usecase.InstructorService
}

func NewInstructorHandler(instructorService *usecase.InstructorService) *InstructorHandler {
	return &InstructorHandler{instructorService: instructorService}
}

func (h *InstructorHandler) Lobby(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	lobby, err := h.instructorService.Lobby(c.Request.Context(), roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, lobby)
}

func (h *InstructorHandler) Config(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	// We ignore budget/batch here; they will be provided when starting the round.
	round, err := h.instructorService.InsertConfig(c.Request.Context(), roundID, 0, 1)
	if err != nil {
		// Inline log to help diagnose server errors in lower layers
		c.Error(err) // recorded in Gin context; already gets logged by middleware
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{"round": round})
}

func (h *InstructorHandler) Assign(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	var req dto.AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}
	lobby, err := h.instructorService.Assign(c.Request.Context(), roundID, req.CustomerCount, req.TeamCount)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, lobby)
}

func (h *InstructorHandler) PatchUser(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid user id", middleware.GetRequestID(c))
		return
	}
	var req dto.PatchUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	var rolePtr *domain.Role
	if req.Role != nil {
		role := domain.Role(*req.Role)
		rolePtr = &role
	}
	status := domain.ParticipantStatus(req.Status)

	lobby, err := h.instructorService.PatchUser(c.Request.Context(), roundID, userID, status, rolePtr, req.TeamID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, lobby)
}

func (h *InstructorHandler) StartRound(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	var req dto.ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}
	batchSize := 0
	switch {
	case req.BatchSize != nil:
		batchSize = *req.BatchSize
	case roundID == 2:
		existingRound, err := h.instructorService.GetRound(c.Request.Context(), roundID)
		if err != nil {
			response.FromDomainError(c, err, middleware.GetRequestID(c))
			return
		}
		batchSize = existingRound.BatchSize
	default:
		response.BadRequest(c, "batch_size is required for this round", middleware.GetRequestID(c))
		return
	}

	round, err := h.instructorService.StartRoundWithConfig(c.Request.Context(), roundID, req.CustomerBudget, batchSize)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{"round": round})
}

func (h *InstructorHandler) EndRound(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	round, err := h.instructorService.EndRound(c.Request.Context(), roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{"round": round})
}

func (h *InstructorHandler) SetPopupState(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}

	var req dto.PopupStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	if req.IsPoppedActive == nil {
		response.BadRequest(c, "is_popped_active is required", middleware.GetRequestID(c))
		return
	}

	round, err := h.instructorService.SetPopupState(c.Request.Context(), roundID, *req.IsPoppedActive)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{"round": gin.H{
		"id":               round.ID,
		"round_number":     round.RoundNumber,
		"status":           round.Status,
		"customer_budget":  round.CustomerBudget,
		"batch_size":       round.BatchSize,
		"started_at":       round.StartedAt,
		"ended_at":         round.EndedAt,
		"is_popped_active": round.IsPoppedActive,
	}})
}

func (h *InstructorHandler) Stats(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	stats, err := h.instructorService.Stats(c.Request.Context(), roundID)
	if err != nil {
		// Attach error for logging middleware; response keeps user-safe message.
		c.Error(err)
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{
		"round_id":                stats.RoundID,
		"leaderboard":             stats.Leaderboard,
		"sales_over_time":         stats.SalesOverTime,
		"batch_sequence_quality":  stats.BatchSequenceQuality,
		"batch_size_quality":      stats.BatchSizeQuality,
	})
}

// DeleteUser removes a non-instructor user from the round and database.
func (h *InstructorHandler) DeleteUser(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid user id", middleware.GetRequestID(c))
		return
	}

	if err := h.instructorService.DeleteUser(c.Request.Context(), roundID, userID); err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{"deleted_user_id": userID})
}
