package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var redemptionService = &service.RedemptionService{}

// --- Admin: 兑换码管理 ---

func AdminCreateRedemptions(c *gin.Context) {
	var req dto.RedemptionCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	items, err := redemptionService.Generate(&req)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "生成兑换码失败: "+err.Error())
		return
	}
	common.Created(c, items)
}

func AdminListRedemptions(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	keyword := c.Query("keyword")
	status, _ := strconv.Atoi(c.Query("status"))
	items, total, err := redemptionService.List(page, pageSize, keyword, status)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取兑换码列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func AdminUpdateRedemptionStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.RedemptionUpdateStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := redemptionService.UpdateStatus(id, req.Status); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func AdminDeleteRedemption(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := redemptionService.Delete(id); err != nil {
		common.Error(c, http.StatusNotFound, err.Error())
		return
	}
	common.Success(c, nil)
}

// --- User: 兑换 ---

func RedeemCode(c *gin.Context) {
	// 开关:未开启兑换时拒绝
	if !model.GetOptionBool("RedemptionEnabled") {
		common.Error(c, http.StatusForbidden, "兑换功能未开放")
		return
	}
	userID := c.GetInt64("user_id")
	var req dto.RedeemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	quota, err := redemptionService.Redeem(userID, req.Code)
	if err != nil {
		// 兑换码业务错误(已用/过期/禁用/无效)统一 400
		switch {
		case errors.Is(err, model.ErrRedemptionUsed):
			common.Error(c, http.StatusBadRequest, "兑换码已被使用")
		case errors.Is(err, model.ErrRedemptionExpired):
			common.Error(c, http.StatusBadRequest, "兑换码已过期")
		case errors.Is(err, model.ErrRedemptionDisabled):
			common.Error(c, http.StatusBadRequest, "兑换码已禁用")
		default:
			common.Error(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	common.Success(c, dto.RedeemResp{Quota: quota})
}
