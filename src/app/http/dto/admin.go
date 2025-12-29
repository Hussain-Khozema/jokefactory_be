package dto

// AdminLoginRequest is used for instructor/admin login.
type AdminLoginRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
	Password    string `json:"password" binding:"required"`
}
