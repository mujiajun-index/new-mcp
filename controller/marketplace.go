package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var marketplaceService = &service.MarketplaceService{}

// --- Admin endpoints ---

func AdminListMarketplaceItems(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	items, total, err := marketplaceService.ListItemsAdmin(page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取市场列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func AdminCreateMarketplaceItem(c *gin.Context) {
	adminID := c.GetInt64("user_id")
	var req dto.CreateMarketplaceItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceService.CreateItem(adminID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func AdminGetMarketplaceItem(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := marketplaceService.GetItemByID(id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "市场项不存在")
		return
	}
	common.Success(c, resp)
}

func AdminUpdateMarketplaceItem(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateMarketplaceItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceService.UpdateItem(id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func AdminDeleteMarketplaceItem(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := marketplaceService.DeleteItem(id); err != nil {
		common.Error(c, http.StatusNotFound, "市场项不存在")
		return
	}
	common.Success(c, nil)
}

// --- Public/User browsing ---

func BrowseMarketplace(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	category := c.Query("category")
	keyword := c.Query("keyword")
	items, total, err := marketplaceService.ListPublished(page, pageSize, category, keyword)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取市场失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func GetMarketplaceItem(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := marketplaceService.GetPublished(id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "市场项不存在")
		return
	}
	common.Success(c, resp)
}

// --- User actions ---

func InstallFromMarketplace(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.InstallFromMarketplaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceService.Install(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func CreateMarketplaceReview(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateReviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceService.CreateReview(userID, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}
