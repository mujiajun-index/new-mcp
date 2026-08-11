package dto

// InviteOverview 我的邀请概览(对齐 new-api /api/user/aff + 钱包邀请区块)。
type InviteOverview struct {
	AffCode         string `json:"aff_code"`                   // 我的邀请码
	InviteURL       string `json:"invite_url"`                 // 邀请链接 ServerAddress + /sign-up?aff=<code>
	AffCount        int    `json:"aff_count"`                  // 已邀请人数
	AffQuota        int64  `json:"aff_quota"`                  // 邀请奖励待提取余额(quota)
	AffHistoryQuota int64  `json:"aff_history_quota"`          // 邀请奖励累计(quota)
	QuotaForInviter int64  `json:"quota_for_inviter"`          // 当前邀请者奖励配置(展示用)
	QuotaForInvitee int64  `json:"quota_for_invitee"`          // 当前受邀者奖励配置(展示用)
}

// TransferAffQuotaReq 邀请奖励转入钱包请求。
type TransferAffQuotaReq struct {
	Quota int64 `json:"quota" binding:"required,min=1"` // 转入额度(quota),最小 1 货币单位由 service 校验
}

// TransferAffQuotaResp 转账后最新余额。
type TransferAffQuotaResp struct {
	Quota    int64 `json:"quota"`     // 转账后钱包可用余额
	AffQuota int64 `json:"aff_quota"` // 转账后邀请奖励待提取余额
}
