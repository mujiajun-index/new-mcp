package service

import (
	"time"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type WalletService struct{}

// Overview 返回用户额度概览。
func (s *WalletService) Overview(userID int64) (*dto.WalletOverview, error) {
	u, err := model.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	return &dto.WalletOverview{
		Quota:        u.Quota,
		UsedQuota:    u.UsedQuota,
		RequestCount: u.RequestCount,
		TotalTopup:   u.TotalTopup,
		Group:        u.Group,
	}, nil
}

// BillingLogs 返回用户消费明细分页(仅计费相关行)。
func (s *WalletService) BillingLogs(userID int64, offset, limit int) ([]dto.WalletBillingItem, int64, error) {
	logs, total, err := model.GetBillingLogsByUser(userID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]dto.WalletBillingItem, len(logs))
	for i, l := range logs {
		items[i] = dto.WalletBillingItem{
			ID:                l.ID,
			ToolName:          l.ToolName,
			Method:            l.Method,
			ServiceName:       l.ServiceName,
			GroupName:         l.GroupName,
			BillingStatus:     l.BillingStatus,
			BillingType:       l.BillingType,
			UnitPrice:         l.UnitPrice,
			QuotaConsumed:     l.QuotaConsumed,
			PriceScope:        l.PriceScope,
			MarketplaceItemID: l.MarketplaceItemID,
			CreatedAt:         l.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}

// UsageStats 返回用户用量统计(今日/本周/累计消费 quota)。
func (s *WalletService) UsageStats(userID int64) (*dto.WalletUsageStats, error) {
	now := time.Now()
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startWeek := startToday.AddDate(0, 0, -int(startToday.Weekday())) // 本周日(按周日为周首)

	today, err := model.SumQuotaConsumedByUser(userID, startToday)
	if err != nil {
		return nil, err
	}
	week, err := model.SumQuotaConsumedByUser(userID, startWeek)
	if err != nil {
		return nil, err
	}
	total, err := model.SumQuotaConsumedByUser(userID, time.Time{})
	if err != nil {
		return nil, err
	}
	return &dto.WalletUsageStats{
		ConsumedToday: today,
		ConsumedWeek:  week,
		ConsumedTotal: total,
	}, nil
}
