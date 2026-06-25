package dto

type RegisterReq struct {
	Username         string `json:"username" binding:"required,min=3,max=64"`
	Password         string `json:"password" binding:"required,min=6,max=128"`
	Email            string `json:"email" binding:"omitempty,email"`
	VerificationCode string `json:"verification_code" binding:"omitempty"`
}

type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResp struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Token    string `json:"token"`
}

type UpdateProfileReq struct {
	DisplayName           *string `json:"display_name"`
	Email                 *string `json:"email" binding:"omitempty,email"`
	AvatarURL             *string `json:"avatar_url" binding:"omitempty,max=512"`
	EmailVerificationCode string  `json:"email_verification_code" binding:"omitempty"`
}

type ChangePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=128"`
}

type ProfileResp struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	AvatarURL    string `json:"avatar_url"`
	Status       int    `json:"status"`
	Quota        int64  `json:"quota"`
	UsedQuota    int64  `json:"used_quota"`
	RequestCount int64  `json:"request_count"`
	Group        string `json:"group"`
	CreatedAt    string `json:"created_at"`
}
