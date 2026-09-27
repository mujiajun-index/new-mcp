package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var systemOneService = &service.SystemOneService{}

func systemOneID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(c, http.StatusBadRequest, "无效的配置 ID")
		return 0, false
	}
	return id, true
}

func ListSystemOneConfigs(c *gin.Context) {
	rows, err := systemOneService.List(c.GetInt64("user_id"))
	if err != nil {
		common.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	common.Success(c, rows)
}

func CreateSystemOneConfig(c *gin.Context) {
	var req dto.SystemOneConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	row, err := systemOneService.Create(c.GetInt64("user_id"), &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, row)
}

func GetSystemOneConfig(c *gin.Context) {
	id, ok := systemOneID(c)
	if !ok {
		return
	}
	row, err := systemOneService.Get(c.GetInt64("user_id"), id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "配置不存在")
		return
	}
	common.Success(c, row)
}

func UpdateSystemOneConfig(c *gin.Context) {
	id, ok := systemOneID(c)
	if !ok {
		return
	}
	var req dto.SystemOneConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := systemOneService.Update(c.GetInt64("user_id"), id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteSystemOneConfig(c *gin.Context) {
	id, ok := systemOneID(c)
	if !ok {
		return
	}
	if err := systemOneService.Delete(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func EnableSystemOneConfig(c *gin.Context) {
	id, ok := systemOneID(c)
	if !ok {
		return
	}
	if err := systemOneService.Enable(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DisableSystemOneConfig(c *gin.Context) {
	id, ok := systemOneID(c)
	if !ok {
		return
	}
	if err := systemOneService.Disable(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func TestSystemOneConfig(c *gin.Context) {
	var req dto.TestSystemOneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, systemOneService.Test(c.GetInt64("user_id"), &req))
}
