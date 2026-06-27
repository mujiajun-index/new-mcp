package controller

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
)

type SetupStatusResponse struct {
	Status       bool   `json:"status"`
	AdminInit    bool   `json:"admin_init"`
	DatabaseType string `json:"database_type,omitempty"`
}

type SetupRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

func GetSetup(c *gin.Context) {
	if common.SystemInitialized {
		common.Error(c, http.StatusForbidden, "系统已经初始化完成")
		return
	}

	resp := SetupStatusResponse{
		Status:    false,
		AdminInit: model.AdminUserExists(),
	}

	switch common.DbType {
	case "mysql":
		resp.DatabaseType = "mysql"
	case "postgres":
		resp.DatabaseType = "postgres"
	default:
		resp.DatabaseType = "sqlite"
	}

	common.Success(c, resp)
}

func PostSetup(c *gin.Context) {
	if common.SystemInitialized {
		common.Error(c, http.StatusBadRequest, "系统已经初始化完成")
		return
	}

	adminExists := model.AdminUserExists()

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	if !adminExists {
		if req.Username == "" || len(req.Username) > 64 {
			common.Error(c, http.StatusBadRequest, "用户名长度应在1-64个字符之间")
			return
		}
		if len(req.Password) < 8 {
			common.Error(c, http.StatusBadRequest, "密码长度至少为8个字符")
			return
		}
		if req.Password != req.ConfirmPassword {
			common.Error(c, http.StatusBadRequest, "两次输入的密码不一致")
			return
		}

		hash, err := common.Password2Hash(req.Password)
		if err != nil {
			common.Error(c, http.StatusInternalServerError, "系统错误")
			return
		}

		user := &model.User{
			Username:   req.Username,
			Password:   hash,
			// 系统初始化的首个账号即超级管理员（固定为 id=1）。
			Role:       common.RoleSuperAdmin,
			Status:     common.StatusEnabled,
			Group:      "default",
			RegisterIP: middleware.GetRequestIP(c),
		}
		if err := user.Insert(); err != nil {
			common.Error(c, http.StatusInternalServerError, "创建管理员账号失败")
			return
		}
	}

	setup := model.Setup{
		Version:       common.Version,
		InitializedAt: time.Now().Unix(),
	}
	if err := model.DB.Create(&setup).Error; err != nil {
		common.Error(c, http.StatusInternalServerError, "系统初始化失败")
		return
	}

	common.SystemInitialized = true
	common.Success(c, nil)
}
