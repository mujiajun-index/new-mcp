package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var imageGenService = &service.ImageGenService{}

func imageGenID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(c, http.StatusBadRequest, "Invalid image configuration ID")
		return 0, false
	}
	return id, true
}
func ListImageGenConfigs(c *gin.Context) {
	items, err := imageGenService.List(c.GetInt64("user_id"))
	if err != nil {
		common.Error(c, 500, "Failed to list image configurations")
		return
	}
	common.Success(c, items)
}
func CreateImageGenConfig(c *gin.Context) {
	var r dto.ImageGenConfigReq
	if err := c.ShouldBindJSON(&r); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	item, err := imageGenService.Create(c.GetInt64("user_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Created(c, item)
}
func GetImageGenConfig(c *gin.Context) {
	id, ok := imageGenID(c)
	if !ok {
		return
	}
	item, err := imageGenService.Get(c.GetInt64("user_id"), id)
	if err != nil {
		common.Error(c, 404, "Image configuration not found")
		return
	}
	common.Success(c, item)
}
func UpdateImageGenConfig(c *gin.Context) {
	id, ok := imageGenID(c)
	if !ok {
		return
	}
	var r dto.ImageGenConfigReq
	if err := c.ShouldBindJSON(&r); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	if err := imageGenService.Update(c.GetInt64("user_id"), id, &r); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, nil)
}
func DeleteImageGenConfig(c *gin.Context) {
	id, ok := imageGenID(c)
	if !ok {
		return
	}
	if err := imageGenService.Delete(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, nil)
}
func EnableImageGenConfig(c *gin.Context) {
	id, ok := imageGenID(c)
	if !ok {
		return
	}
	if err := imageGenService.Enable(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, nil)
}
func DisableImageGenConfig(c *gin.Context) {
	id, ok := imageGenID(c)
	if !ok {
		return
	}
	if err := imageGenService.Disable(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, nil)
}
func ListImageGenModels(c *gin.Context) {
	var r dto.ImageGenProbeReq
	if err := c.ShouldBindJSON(&r); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	items, err := imageGenService.Models(c.Request.Context(), c.GetInt64("user_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, items)
}
func TestImageGenConfig(c *gin.Context) {
	var r dto.ImageGenProbeReq
	if err := c.ShouldBindJSON(&r); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	item, err := imageGenService.Test(c.Request.Context(), c.GetInt64("user_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, item)
}
