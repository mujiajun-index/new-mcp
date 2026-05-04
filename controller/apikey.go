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
	items, err := apiKeyService.List(userID)
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
		common.Error(c, http.StatusInternalServerError, "创建 API Key 失败")
		return
	}
	common.Created(c, resp)
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
