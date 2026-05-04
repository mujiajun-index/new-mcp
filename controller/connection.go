package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var connectionService = &service.ConnectionService{}

func ListConnections(c *gin.Context) {
	userID := c.GetInt64("user_id")
	items, err := connectionService.List(userID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取连接列表失败")
		return
	}
	common.Success(c, items)
}

func CreateConnection(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateConnectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := connectionService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func GetConnection(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := connectionService.GetByID(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "连接不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateConnection(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateConnectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := connectionService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteConnection(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := connectionService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "连接不存在")
		return
	}
	common.Success(c, nil)
}

func ConnectConnection(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := connectionService.Connect(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, map[string]string{"connection_status": "connected"})
}

func DisconnectConnection(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := connectionService.Disconnect(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, map[string]string{"connection_status": "disconnected"})
}

func BindConnectionApiKey(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		ApiKeyID int64 `json:"api_key_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := connectionService.BindApiKey(userID, id, req.ApiKeyID); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}
