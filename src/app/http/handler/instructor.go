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

	var req dto.ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}

	base := domain.DefaultRoundConfig()
	existing, err := h.instructorService.GetRound(c.Request.Context(), roundID)
	if err == nil {
		base = domain.ConfigFromRound(existing)
	} else if !domain.IsNotFound(err) {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	cfg := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{
		CustomerBudget:        req.CustomerBudget,
		BatchSize:             req.BatchSize,
		MarketPrice:           req.MarketPrice,
		CostOfPublishing:      req.CostOfPublishing,
		CostOfDiscard:         req.CostOfDiscard,
		CustomerCount:         req.CustomerCount,
		BuyThreshold:          req.BuyThreshold,
		Jitter:                req.Jitter,
		SwapMargin:            req.SwapMargin,
		FeedbackJokeCount:     req.FeedbackJokeCount,
		FeedbackPassThreshold: req.FeedbackPassThreshold,
	})

	result, err := h.instructorService.Config(
		c.Request.Context(),
		roundID,
		&cfg,
		dto.IdealProfileToDomain(req.IdealProfile),
	)
	if err != nil {
		_ = c.Error(err)
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}

	response.OK(c, gin.H{
		"round": dto.ToInstructorRound(result.Round, result.IdealProfile),
	})
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
	lobby, err := h.instructorService.Assign(c.Request.Context(), roundID, req.TeamCount)
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

	// Optional body: if present with fields, merge into config before starting (FE compat).
	var req dto.ConfigRequest
	bindErr := c.ShouldBindJSON(&req)
	if bindErr != nil && c.Request.ContentLength > 0 {
		response.BadRequest(c, "invalid payload", middleware.GetRequestID(c))
		return
	}
	if bindErr == nil && hasConfigFields(&req) {
		base := domain.DefaultRoundConfig()
		existing, err := h.instructorService.GetRound(c.Request.Context(), roundID)
		if err == nil {
			base = domain.ConfigFromRound(existing)
		} else if !domain.IsNotFound(err) {
			response.FromDomainError(c, err, middleware.GetRequestID(c))
			return
		}
		cfg := usecase.MergeConfig(&base, &usecase.PartialRoundConfig{
			CustomerBudget:        req.CustomerBudget,
			BatchSize:             req.BatchSize,
			MarketPrice:           req.MarketPrice,
			CostOfPublishing:      req.CostOfPublishing,
			CostOfDiscard:         req.CostOfDiscard,
			CustomerCount:         req.CustomerCount,
			BuyThreshold:          req.BuyThreshold,
			Jitter:                req.Jitter,
			SwapMargin:            req.SwapMargin,
			FeedbackJokeCount:     req.FeedbackJokeCount,
			FeedbackPassThreshold: req.FeedbackPassThreshold,
		})
		if _, err := h.instructorService.Config(
			c.Request.Context(), roundID, &cfg, dto.IdealProfileToDomain(req.IdealProfile),
		); err != nil {
			response.FromDomainError(c, err, middleware.GetRequestID(c))
			return
		}
	}

	round, err := h.instructorService.StartRound(c.Request.Context(), roundID)
	if err != nil {
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	profile, _ := h.instructorService.GetIdealProfile(c.Request.Context(), roundID)
	response.OK(c, gin.H{"round": dto.ToInstructorRound(round, profile)})
}

func hasConfigFields(req *dto.ConfigRequest) bool {
	return req.CustomerBudget != nil ||
		req.BatchSize != nil ||
		req.MarketPrice != nil ||
		req.CostOfPublishing != nil ||
		req.CostOfDiscard != nil ||
		req.CustomerCount != nil ||
		req.BuyThreshold != nil ||
		req.Jitter != nil ||
		req.SwapMargin != nil ||
		req.FeedbackJokeCount != nil ||
		req.FeedbackPassThreshold != nil ||
		req.IdealProfile != nil
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
	response.OK(c, gin.H{"round": dto.ToPublicRound(round)})
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

	response.OK(c, gin.H{"round": dto.ToPublicRound(round)})
}

func (h *InstructorHandler) Stats(c *gin.Context) {
	roundID, err := strconv.ParseInt(c.Param("round_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid round id", middleware.GetRequestID(c))
		return
	}
	stats, err := h.instructorService.Stats(c.Request.Context(), roundID)
	if err != nil {
		_ = c.Error(err)
		response.FromDomainError(c, err, middleware.GetRequestID(c))
		return
	}
	response.OK(c, gin.H{
		"round_id":    stats.RoundID,
		"leaderboard": stats.Leaderboard,
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
