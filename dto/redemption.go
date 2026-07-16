package dto

// RedemptionCreateReq 管理员批量生成兑换码。Quota 单位为 quota(面值)。
type RedemptionCreateReq struct {
	Name      string `json:"name"`
	Quota     int64  `json:"quota" binding:"required,min=1"`
	Count     int    `json:"count" binding:"min=1,max=100"`
	ExpiredAt int64  `json:"expired_at"` // Unix 秒,0=永不过期
}

type RedemptionItem struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Quota      int64  `json:"quota"`
	Status     int    `json:"status"` // 1=可用 2=已兑换 3=已禁用
	UserID     *int64 `json:"user_id"`
	ExpiredAt  int64  `json:"expired_at"`
	CreatedAt  string `json:"created_at"`
	RedeemedAt string `json:"redeemed_at"`
}

type RedemptionUpdateStatusReq struct {
	Status int `json:"status" binding:"oneof=1 3"` // 仅允许 可用/禁用 切换;已兑换不可改
}

type RedeemReq struct {
	Code string `json:"code" binding:"required"`
}

type RedeemResp struct {
	Quota int64 `json:"quota"` // 本次入账额度(quota)
}
