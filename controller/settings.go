package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var settingsService = &service.SettingsService{}

func AdminGetSettings(c *gin.Context) {
	items := settingsService.GetAllSettings()
	common.Success(c, items)
}

func AdminUpdateSetting(c *gin.Context) {
	var req dto.SettingUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := settingsService.UpdateSetting(req.Key, req.Value); err != nil {
		common.Error(c, http.StatusInternalServerError, "更新失败: "+err.Error())
		return
	}
	common.Success(c, nil)
}

func GetPublicSettings(c *gin.Context) {
	settings := settingsService.GetPublicSettings()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    settings,
	})
}
