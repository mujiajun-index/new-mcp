package dto

// WalletOverview 我的额度概览。
type WalletOverview struct {
	Quota        int64  `json:"quota"`         // 可用余额(quota)
	UsedQuota    int64  `json:"used_quota"`    // 累计已用(quota)
	RequestCount int64  `json:"request_count"` // 累计请求数
	TotalTopup   int64  `json:"total_topup"`   // 累计充值(quota)
	Group        string `json:"group"`         // 用户套餐分组
}

// WalletUsageStats 用量统计(今日/本周消费 quota)。
type WalletUsageStats struct {
	ConsumedToday int64 `json:"consumed_today"`
	ConsumedWeek  int64 `json:"consumed_week"`
	ConsumedTotal int64 `json:"consumed_total"`
}
