package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var mcpServiceService = &service.McpServiceService{}

func ListServices(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, pageSize := common.GetPagination(c)

	filters := map[string]string{
		"transport_type": c.Query("transport_type"),
		"status":         c.Query("status"),
		"keyword":        c.Query("keyword"),
	}

	items, total, err := mcpServiceService.List(userID, page, pageSize, filters)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取服务列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func CreateService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateServiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func GetService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.GetByID(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateServiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := mcpServiceService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := mcpServiceService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, nil)
}

func TestService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.Test(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func TestConnection(c *gin.Context) {
	var req dto.TestConnectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.TestConnection(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

func RefreshTools(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.RefreshTools(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func GetServiceTools(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	tools, err := mcpServiceService.GetTools(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, tools)
}

func GetServiceHealth(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.GetHealth(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}
