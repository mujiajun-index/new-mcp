package dto

// WalletOverview 我的额度概览。
type WalletOverview struct {
	Quota        int64  `json:"quota"`         // 可用余额(quota)
	UsedQuota    int64  `json:"used_quota"`    // 累计已用(quota)
	RequestCount int64  `json:"request_count"` // 累计请求数
	TotalTopup   int64  `json:"total_topup"`   // 累计充值(quota)
	Group        string `json:"group"`         // 用户套餐分组
}

// WalletBillingItem 一条消费明细(基于 mcp_call_logs 计费列)。
type WalletBillingItem struct {
	ID                int64   `json:"id"`
	ToolName          string  `json:"tool_name"`
	Method            string  `json:"method"`
	ServiceName       string  `json:"service_name"`
	GroupName         string  `json:"group_name"`
	BillingStatus     string  `json:"billing_status"`
	BillingType       string  `json:"billing_type"`
	UnitPrice         float64 `json:"unit_price"`
	QuotaConsumed     int64   `json:"quota_consumed"`
	PriceScope        string  `json:"price_scope"`
	MarketplaceItemID *int64  `json:"marketplace_item_id"`
	CreatedAt         string  `json:"created_at"`
}

// WalletUsageStats 用量统计(今日/本周消费 quota)。
type WalletUsageStats struct {
	ConsumedToday int64 `json:"consumed_today"`
	ConsumedWeek  int64 `json:"consumed_week"`
	ConsumedTotal int64 `json:"consumed_total"`
}
