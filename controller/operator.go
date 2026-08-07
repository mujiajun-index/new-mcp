package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
)

// operatorFromContext 从 gin context 构造操作者(管理员/用户),用于写入审计日志。
// user_id/username/role 由 middleware/auth.go 注入。
func operatorFromContext(c *gin.Context) model.Operator {
	return model.Operator{
		ID:       c.GetInt64("user_id"),
		Username: c.GetString("username"),
		Role:     c.GetString("role"),
		IP:       middleware.GetRequestIP(c),
	}
}
