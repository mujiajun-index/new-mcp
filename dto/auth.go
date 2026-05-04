package dto

type RegisterReq struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=128"`
	Email    string `json:"email" binding:"omitempty,email"`
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
	Email     string `json:"email" binding:"omitempty,email"`
	AvatarURL string `json:"avatar_url" binding:"omitempty,max=512"`
}

type ChangePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=128"`
}

type ProfileResp struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	AvatarURL string `json:"avatar_url"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}
