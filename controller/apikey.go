package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var apiKeyService = &service.ApiKeyService{}

func ListApiKeys(c *gin.Context) {
	userID := c.GetInt64("user_id")
	keyword := c.Query("keyword")
	items, err := apiKeyService.List(userID, keyword)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取 API Key 列表失败")
		return
	}
	common.Success(c, items)
}

func CreateApiKey(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateApiKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := apiKeyService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func UpdateApiKey(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateApiKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := apiKeyService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteApiKey(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := apiKeyService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "API Key 不存在")
		return
	}
	common.Success(c, nil)
}

func GetApiKeyFullKey(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	key, err := apiKeyService.GetKey(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, err.Error())
		return
	}
	common.Success(c, gin.H{"key": key})
}

func BatchDeleteApiKeys(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req struct {
		IDs []int64 `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := apiKeyService.BatchDelete(userID, req.IDs); err != nil {
		common.Error(c, http.StatusInternalServerError, "批量删除失败")
		return
	}
	common.Success(c, nil)
}

func BatchUpdateApiKeyStatus(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req struct {
		IDs    []int64 `json:"ids" binding:"required"`
		Status int     `json:"status" binding:"required,oneof=1 2"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := apiKeyService.BatchUpdateStatus(userID, req.IDs, req.Status); err != nil {
		common.Error(c, http.StatusInternalServerError, "批量更新失败")
		return
	}
	common.Success(c, nil)
}
