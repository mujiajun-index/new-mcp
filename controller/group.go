package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var groupService = &service.GroupService{}

func ListGroups(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, pageSize := common.GetPagination(c)
	items, total, err := groupService.List(userID, page, pageSize)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取分组列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func CreateGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := groupService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func CheckGroupName(c *gin.Context) {
	userID := c.GetInt64("user_id")
	name := c.Query("name")
	if name == "" {
		common.Error(c, http.StatusBadRequest, "name 参数不能为空")
		return
	}
	excludeID, _ := strconv.ParseInt(c.Query("exclude_id"), 10, 64)
	exists, err := groupService.CheckName(userID, name, excludeID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "检查失败")
		return
	}
	common.Success(c, map[string]bool{"exists": exists})
}

func GetGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := groupService.GetByID(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "分组不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := groupService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := groupService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "分组不存在")
		return
	}
	common.Success(c, nil)
}

func AddGroupServices(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.AddGroupServicesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := groupService.AddServices(userID, id, req.ServiceIDs); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func RemoveGroupService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	groupID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	serviceID, _ := strconv.ParseInt(c.Param("serviceId"), 10, 64)
	if err := groupService.RemoveService(userID, groupID, serviceID); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func GetGroupTools(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	tools, err := groupService.GetTools(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "分组不存在")
		return
	}
	common.Success(c, tools)
}

func UpdateGroupTool(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	toolName := c.Param("toolName")
	var req dto.UpdateToolReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := groupService.UpdateTool(userID, id, toolName, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func BatchUpdateGroupTools(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.BatchUpdateToolsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := groupService.BatchUpdateTools(userID, id, req.Tools); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func RefreshGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := groupService.RefreshAll(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func GetGroupEndpoint(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := groupService.GetEndpoint(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "分组不存在")
		return
	}
	common.Success(c, resp)
}
