package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var marketplaceGroupService = &service.MarketplaceGroupService{}

// --- Admin: marketplace groups (业务分类) ---

func AdminListMarketplaceGroups(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	status, _ := strconv.Atoi(c.DefaultQuery("status", "0"))
	items, total, err := marketplaceGroupService.List(status, page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取分组失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

// AdminListAllMarketplaceGroups 返回所有分组(不分页,管理 UI 用)。
func AdminListAllMarketplaceGroups(c *gin.Context) {
	items, err := marketplaceGroupService.ListAllForAdmin()
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取分组失败")
		return
	}
	common.Success(c, items)
}

func AdminCreateMarketplaceGroup(c *gin.Context) {
	var req dto.CreateMarketplaceGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceGroupService.Create(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func AdminUpdateMarketplaceGroup(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateMarketplaceGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceGroupService.Update(id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func AdminDeleteMarketplaceGroup(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := marketplaceGroupService.Delete(id); err != nil {
		common.Error(c, http.StatusNotFound, "分组不存在")
		return
	}
	common.Success(c, nil)
}

// --- Public ---

// BrowseMarketplaceGroups 公开:返回启用分组(供广场左侧筛选)。
func BrowseMarketplaceGroups(c *gin.Context) {
	items, err := marketplaceGroupService.ListEnabled()
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取分组失败")
		return
	}
	common.Success(c, items)
}
