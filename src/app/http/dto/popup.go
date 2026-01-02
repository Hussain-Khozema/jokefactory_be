package dto

// PopupStateRequest toggles popups for a round.
type PopupStateRequest struct {
	IsPoppedActive *bool `json:"is_popped_active" binding:"required"`
}

