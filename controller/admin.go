package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var adminService = &service.AdminService{}

func AdminListUsers(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	keyword := c.Query("keyword")
	items, total, err := adminService.ListUsers(page, pageSize, keyword)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取用户列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func AdminUpdateUser(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.AdminUpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := adminService.UpdateUser(id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func AdminCreateUser(c *gin.Context) {
	var req dto.AdminCreateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	user, err := adminService.CreateUser(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, user)
}

func AdminGetStats(c *gin.Context) {
	stats, err := adminService.GetStats()
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取统计数据失败")
		return
	}
	common.Success(c, stats)
}

func AdminGetLogs(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	var filter dto.LogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		common.Error(c, http.StatusBadRequest, "筛选参数错误")
		return
	}
	items, total, err := adminService.GetLogs(&filter, page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取日志失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func AdminGetLogStats(c *gin.Context) {
	var filter dto.LogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		common.Error(c, http.StatusBadRequest, "筛选参数错误")
		return
	}
	stats, err := adminService.GetLogStats(&filter)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取日志统计失败")
		return
	}
	common.Success(c, stats)
}

// --- User log endpoints ---

func GetUserLogs(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, pageSize := common.GetPagination(c)
	var filter dto.LogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		common.Error(c, http.StatusBadRequest, "筛选参数错误")
		return
	}
	items, total, err := adminService.GetUserLogs(userID, &filter, page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取日志失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func GetUserLogStats(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var filter dto.LogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		common.Error(c, http.StatusBadRequest, "筛选参数错误")
		return
	}
	stats, err := adminService.GetUserLogStats(userID, &filter)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取日志统计失败")
		return
	}
	common.Success(c, stats)
}

// --- Admin service management ---

func AdminListServices(c *gin.Context) {
	page, pageSize := common.GetPagination(c)
	items, total, err := mcpServiceService.ListAdminServices(page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取服务列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func AdminCreateService(c *gin.Context) {
	adminID := c.GetInt64("user_id")
	var req dto.CreateServiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.CreateAdminService(adminID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}
