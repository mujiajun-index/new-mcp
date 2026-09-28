package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var smartSearchService = &service.SmartSearchService{}

func AdminGetSmartSearch(c *gin.Context) {
	config, err := smartSearchService.Get()
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "读取智能搜索配置失败")
		return
	}
	common.Success(c, config)
}

func AdminUpdateSmartSearch(c *gin.Context) {
	var in dto.SmartSearchConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	config, err := smartSearchService.Update(operatorFromContext(c), &in)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, config)
}

func AdminSetSmartSearchEnabled(c *gin.Context) {
	var in struct {
		Enabled *bool `json:"enabled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	config, err := smartSearchService.SetEnabled(operatorFromContext(c), *in.Enabled)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, config)
}

func AdminTestSmartSearch(c *gin.Context) {
	var in dto.SmartSearchConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := smartSearchService.Test(&in)
	if err != nil {
		common.Error(c, http.StatusBadRequest, "测试失败: "+err.Error())
		return
	}
	common.Success(c, gin.H{"result": string(result)})
}
