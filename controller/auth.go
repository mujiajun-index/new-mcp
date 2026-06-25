package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var authService = &service.AuthService{}

func Register(c *gin.Context) {
	var req dto.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := authService.Register(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func Login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := authService.Login(&req)
	if err != nil {
		common.Error(c, http.StatusUnauthorized, err.Error())
		return
	}
	common.Success(c, resp)
}

func GetProfile(c *gin.Context) {
	userID := c.GetInt64("user_id")
	resp, err := authService.GetProfile(userID)
	if err != nil {
		common.Error(c, http.StatusNotFound, "用户不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateProfile(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.UpdateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := authService.UpdateProfile(userID, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func ChangePassword(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req dto.ChangePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := authService.ChangePassword(userID, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}
