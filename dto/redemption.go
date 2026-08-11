package dto

// RedemptionCreateReq 管理员批量生成兑换码。Amount 为面值(货币单位,如 CNY 元)。
type RedemptionCreateReq struct {
	Name      string  `json:"name"`
	Amount    float64 `json:"amount" binding:"required,gt=0"`
	Count     int     `json:"count" binding:"min=1,max=100"`
	ExpiredAt int64   `json:"expired_at"` // Unix 秒,0=永不过期
}

type RedemptionItem struct {
	ID         int64   `json:"id"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Amount     float64 `json:"amount"` // 面值(货币单位)
	Status     int     `json:"status"` // 1=可用 2=已兑换 3=已禁用
	UserID     *int64  `json:"user_id"`
	Username   string  `json:"username"` // 兑换者用户名(未兑换时为空)
	ExpiredAt  int64   `json:"expired_at"`
	CreatedAt  string  `json:"created_at"`
	RedeemedAt string  `json:"redeemed_at"`
}

type RedemptionUpdateStatusReq struct {
	Status int `json:"status" binding:"oneof=1 3"` // 仅允许 可用/禁用 切换;已兑换不可改
}

type RedeemReq struct {
	Code string `json:"code" binding:"required"`
}

type RedeemResp struct {
	Amount float64 `json:"amount"` // 本次充值面值(货币)
}
