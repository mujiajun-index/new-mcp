package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/service"
)

var systemService = &service.SystemService{}

// AdminGetSystemInfo 返回当前版本与启动时间。
// GET /api/v1/admin/system/info
func AdminGetSystemInfo(c *gin.Context) {
	common.Success(c, systemService.GetSystemInfo())
}

// AdminCheckUpdate 代理请求 GitHub 最新 release 并返回。
// 仓库无 release 时返回 success + has_release=false，由前端给提示而非报错；
// 网络/上游失败返回 502，axios 拦截器会自动 toast 后端的中文 message。
// GET /api/v1/admin/system/check-update
func AdminCheckUpdate(c *gin.Context) {
	result, err := systemService.CheckUpdate()
	if err != nil {
		common.Error(c, http.StatusBadGateway, "检查更新失败: "+err.Error())
		return
	}
	common.Success(c, result)
}
