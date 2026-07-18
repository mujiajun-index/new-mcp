package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var marketplaceTagService = &service.MarketplaceTagService{}

// --- Admin: marketplace tags (标签字典) ---

func AdminListMarketplaceTags(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	status, _ := strconv.Atoi(c.DefaultQuery("status", "0"))
	items, total, err := marketplaceTagService.List(status, page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取标签失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func AdminCreateMarketplaceTag(c *gin.Context) {
	var req dto.CreateMarketplaceTagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceTagService.Create(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func AdminUpdateMarketplaceTag(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateMarketplaceTagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceTagService.Update(id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func AdminDeleteMarketplaceTag(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := marketplaceTagService.Delete(id); err != nil {
		common.Error(c, http.StatusNotFound, "标签不存在")
		return
	}
	common.Success(c, nil)
}
