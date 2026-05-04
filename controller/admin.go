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
	items, total, err := adminService.ListUsers(page, pageSize)
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
	items, total, err := adminService.GetLogs(page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取日志失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}
