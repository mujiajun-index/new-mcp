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
	status, _ := strconv.Atoi(c.Query("status"))
	groupID, _ := strconv.ParseInt(c.Query("group_id"), 10, 64)
	items, total, err := marketplaceService.ListItemsAdmin(page, pageSize, status,
		c.Query("category"), c.Query("keyword"), groupID, c.Query("tag"))
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取市场列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

// AdminGetMarketplaceHealth 市场管理页:全部条目的平台级健康(同条目下全部
// 用户引用行的真实调用聚合,30s 缓存)。
func AdminGetMarketplaceHealth(c *gin.Context) {
	health := service.GetMarketplaceItemHealth()
	if health == nil {
		health = map[int64]*dto.MarketplaceItemHealth{}
	}
	common.Success(c, health)
}

// AdminGetMarketplaceItemProcess 市场详情(stdio 条目):进程视图——共享=平台唯一
// 进程;独占=安装引用行分页枚举(?page=&page_size=&username=,默认每页 18 条,
// username 匹配用户名/服务名),另附全量运行实例的资源概述(详情页 5s 轮询)。
func AdminGetMarketplaceItemProcess(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	resp, err := marketplaceService.GetProcessStat(id, page, pageSize, c.Query("username"))
	if err != nil {
		common.Error(c, http.StatusNotFound, "市场项不存在")
		return
	}
	common.Success(c, resp)
}

// AdminControlMarketplaceItemProcess 条目进程启停:共享模式操作平台唯一进程
// (start=预热);独占模式 body.service_id 指定目标安装引用行。
func AdminControlMarketplaceItemProcess(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.MarketplaceProcessControlReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceService.ControlProcess(id, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
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

// AdminRefreshMarketplaceItem 手动刷新市场项 tools/resources/prompts 快照(仅平台托管项)。
func AdminRefreshMarketplaceItem(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := marketplaceService.RefreshItemSnapshots(id)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// --- 条目级多秘钥管理(/admin/marketplace/:id/keys) ---
// 一份池对全部安装用户全局轮换;交互与 DTO 同服务级(/services/:id/keys)。

// AdminGetMarketplaceKeys 条目秘钥池视图(掩码值 + 模式 + 统计)。
func AdminGetMarketplaceKeys(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := marketplaceService.ListKeys(id)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// AdminUpdateMarketplaceKeys 更新条目秘钥:追加(去重保状态)/ 替换全部。
func AdminUpdateMarketplaceKeys(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateServiceKeysReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceService.UpdateKeys(id, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// AdminSetMarketplaceKeyStatus 启用/禁用单把条目秘钥。
func AdminSetMarketplaceKeyStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	keyID, _ := strconv.ParseInt(c.Param("keyID"), 10, 64)
	var req dto.SetServiceKeyStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceService.SetKeyStatus(id, keyID, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// AdminDeleteMarketplaceKey 删除单把条目秘钥。
func AdminDeleteMarketplaceKey(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	keyID, _ := strconv.ParseInt(c.Param("keyID"), 10, 64)
	if err := marketplaceService.DeleteKey(id, keyID); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// AdminBatchMarketplaceKeys 批量操作:全部启用 / 删除已禁用。
func AdminBatchMarketplaceKeys(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.BatchServiceKeysReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceService.BatchKeys(id, req.Action); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// AdminUpdateMarketplaceKeyConfig 条目模式切换:单↔多、随机↔轮询。
func AdminUpdateMarketplaceKeyConfig(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateServiceKeyConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceService.UpdateKeyConfig(id, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// AdminBatchUpdateMarketplacePricing 批量设置已上架市场服务价格(§5.5)。
func AdminBatchUpdateMarketplacePricing(c *gin.Context) {
	var req dto.BatchPricingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	affected, err := marketplaceService.BatchUpdatePricing(req.Items)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, gin.H{"affected": affected})
}

// AdminBatchSetMarketplaceGroupsTags 批量设置市场项分组/标签(替换语义:
// 字段缺省=不动,空数组=清空;两字段独立开关)。
func AdminBatchSetMarketplaceGroupsTags(c *gin.Context) {
	var req dto.BatchGroupsTagsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	affected, err := marketplaceService.BatchSetGroupsTags(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, gin.H{"affected": affected})
}

// AdminSetMarketplaceEntryPrices 全量替换市场项条目级定价(工具/资源/提示单独设价,
// §5.2)。不在载荷中的条目回退:工具→服务统一价,资源/提示→免费。
func AdminSetMarketplaceEntryPrices(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.EntryPricingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := marketplaceService.SetItemEntryPrices(id, req.Prices); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// AdminCloneMarketplaceItem 从自有服务克隆上架(D14)。
func AdminCloneMarketplaceItem(c *gin.Context) {
	adminID := c.GetInt64("user_id")
	var req dto.CloneMarketplaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := marketplaceService.CloneFromService(adminID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

// AdminListCloneSources 列出当前管理员自己账户下可克隆上架的来源服务(其账户下 source=user/admin,
// 自动排除虚拟服务与市场引用),供"从自有服务克隆"下拉使用(§11)。
func AdminListCloneSources(c *gin.Context) {
	adminID := c.GetInt64("user_id")
	page, pageSize := common.GetPagination(c)
	items, total, err := mcpServiceService.ListClonableServices(adminID, page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取可克隆服务列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

// --- Public/User browsing ---

func BrowseMarketplace(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	category := c.Query("category")
	keyword := c.Query("keyword")
	groupID, _ := strconv.ParseInt(c.Query("group_id"), 10, 64)
	tag := c.Query("tag")
	items, total, err := marketplaceService.ListPublished(page, pageSize, category, keyword, groupID, tag)
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

// AddMarketplaceItem 引用式安装:把市场项添加为用户的引用服务(source=marketplace,空 config)。
func AddMarketplaceItem(c *gin.Context) {
	userID := c.GetInt64("user_id")
	itemID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if itemID <= 0 {
		common.Error(c, http.StatusBadRequest, "无效的市场项 ID")
		return
	}
	resp, err := marketplaceService.AddToMyServices(userID, itemID)
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
