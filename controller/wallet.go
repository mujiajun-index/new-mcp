package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/service"
)

var walletService = &service.WalletService{}

// GetWallet 我的额度概览(quota/used_quota/请求次数/累计充值/分组)。
func GetWallet(c *gin.Context) {
	userID := c.GetInt64("user_id")
	resp, err := walletService.Overview(userID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取额度概览失败")
		return
	}
	common.Success(c, resp)
}

// GetWalletBilling 消费明细分页(基于 mcp_call_logs 计费列,仅计费相关行)。
func GetWalletBilling(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, pageSize := common.GetPagination(c)
	offset := common.GetOffset(page, pageSize)
	items, total, err := walletService.BillingLogs(userID, offset, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取消费明细失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

// GetWalletUsageStats 用量统计(今日/本周/累计消费 quota)。
func GetWalletUsageStats(c *gin.Context) {
	userID := c.GetInt64("user_id")
	resp, err := walletService.UsageStats(userID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取用量统计失败")
		return
	}
	common.Success(c, resp)
}
