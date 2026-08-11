package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var inviteService = &service.InviteService{}

// GetInviteOverview 我的邀请概览(邀请码/链接/已邀请人数/奖励余额/当前奖励配置)。
func GetInviteOverview(c *gin.Context) {
	userID := c.GetInt64("user_id")
	resp, err := inviteService.Overview(userID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取邀请信息失败")
		return
	}
	common.Success(c, resp)
}

// TransferAffQuota 邀请奖励待提取余额转入钱包(对齐 new-api /api/user/aff_transfer)。
func TransferAffQuota(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.TransferAffQuotaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := inviteService.Transfer(userID, &req, operatorFromContext(c).IP)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTransferTooSmall):
			common.Error(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, model.ErrInsufficientAffQuota):
			common.Error(c, http.StatusBadRequest, "邀请额度不足")
		default:
			common.Error(c, http.StatusInternalServerError, "转入失败")
		}
		return
	}
	common.Success(c, resp)
}
