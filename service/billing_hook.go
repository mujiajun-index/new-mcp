package service

import (
	"fmt"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/model"
)

// 注入低额度提醒钩子(打破 billing → service 的依赖环:billing 经钩子回调,
// 由能访问 SendEmail 的 service 包实际发信)。包初始化时注册一次。
func init() {
	billing.LowQuotaNotifier = func(userID, currentQuota int64) {
		if !model.IsSMTPConfigured() {
			return
		}
		user, err := model.GetUserByID(userID)
		if err != nil || user.Email == "" {
			return
		}
		systemName := model.GetOptionString("SystemName")
		currency := model.GetOptionString("DisplayCurrency")
		quotaPerUnit := model.GetOptionInt64("QuotaPerUnit")
		if quotaPerUnit <= 0 {
			quotaPerUnit = 500000
		}
		content := fmt.Sprintf(
			"<p>您在 %s 的账户额度已低于提醒阈值,当前剩余:<b>%.2f %s</b>。请及时充值或兑换,以免影响市场服务调用。</p>",
			systemName, float64(currentQuota)/float64(quotaPerUnit), currency)
		_ = SendEmail("额度即将用尽提醒", user.Email, content)
	}
}
