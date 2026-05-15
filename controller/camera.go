package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var cameraService = &service.CameraService{}

func ListCameras(c *gin.Context) {
	userID := c.GetInt64("user_id")
	items, err := cameraService.List(userID)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取摄像头列表失败")
		return
	}
	common.Success(c, items)
}

func CreateCamera(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.CreateCameraReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := cameraService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func GetCamera(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := cameraService.GetByID(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "摄像头不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateCamera(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateCameraReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := cameraService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteCamera(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := cameraService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "摄像头不存在")
		return
	}
	common.Success(c, nil)
}

func EnableCamera(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := cameraService.Enable(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DisableCamera(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := cameraService.Disable(userID, id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}
