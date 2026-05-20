package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var visionService = &service.VisionService{}

func ListVisionConfigs(c *gin.Context) {
	userID := c.GetInt64("user_id")
	items, err := visionService.List(userID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取视觉配置列表失败")
		return
	}
	common.Success(c, items)
}

func CreateVisionConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateVisionConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := visionService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func GetVisionConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := visionService.GetByID(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "视觉配置不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateVisionConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateVisionConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := visionService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteVisionConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := visionService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "视觉配置不存在")
		return
	}
	common.Success(c, nil)
}

func TestVisionConfig(c *gin.Context) {
	var req dto.TestVisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result := visionService.TestVision(&req)
	common.Success(c, result)
}

func ListVisionModels(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.ListModelsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	models, err := visionService.ListModels(userID, &req)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取模型列表失败: "+err.Error())
		return
	}
	common.Success(c, models)
}

func EnableVisionConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := visionService.Enable(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DisableVisionConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := visionService.Disable(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}
