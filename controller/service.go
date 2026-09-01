package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/service"
)

var mcpServiceService = &service.McpServiceService{}

func ListServices(c *gin.Context) {
	userID := c.GetInt64("user_id")
	page, pageSize := common.GetPagination(c)

	filters := map[string]string{
		"transport_type": c.Query("transport_type"),
		"status":         c.Query("status"),
		"keyword":        c.Query("keyword"),
	}

	items, total, err := mcpServiceService.List(userID, page, pageSize, filters)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取服务列表失败")
		return
	}
	common.PageOf(c, items, page, pageSize, total)
}

func CreateService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	role := c.GetString("role")
	var req dto.CreateServiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	// stdio 服务在服务器本地执行命令行子进程，属特权操作，仅管理员可创建。
	if req.TransportType == "stdio" && !common.IsAdminRole(role) {
		common.Error(c, http.StatusForbidden, "仅管理员可创建 stdio 服务")
		return
	}
	resp, err := mcpServiceService.Create(userID, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Created(c, resp)
}

func GetService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.GetByID(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func UpdateService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateServiceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := mcpServiceService.Update(userID, id, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

func DeleteService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := mcpServiceService.Delete(userID, id); err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, nil)
}

func TestService(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.Test(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func TestConnection(c *gin.Context) {
	role := c.GetString("role")
	var req dto.TestConnectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	// 测试 stdio 同样会在服务器本地拉起子进程，仅管理员可用。
	if req.TransportType == "stdio" && !common.IsAdminRole(role) {
		common.Error(c, http.StatusForbidden, "仅管理员可测试 stdio 服务")
		return
	}
	resp, err := mcpServiceService.TestConnection(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// PrepareStdio runs the pre-flight detect/install for a stdio service before creation.
func PrepareStdio(c *gin.Context) {
	role := c.GetString("role")
	// prepare-stdio 会在服务器本地执行 npx/uvx 安装与包预拉取，属特权操作，仅管理员可用。
	if !common.IsAdminRole(role) {
		common.Error(c, http.StatusForbidden, "仅管理员可执行 stdio 安装")
		return
	}
	var req dto.PrepareStdioReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.PrepareStdio(&req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

func RefreshTools(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.RefreshTools(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func GetServiceTools(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	tools, err := mcpServiceService.GetTools(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, tools)
}

// CallServiceTool 服务详情页工具测试:对服务的单个工具执行 tools/call,返回结果与耗时。
func CallServiceTool(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.CallToolReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.CallTool(userID, id, &req)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

// ReadServiceResource 服务详情页资源测试:对指定 URI 执行 resources/read,返回内容与耗时。
func ReadServiceResource(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.ReadResourceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.ReadResource(userID, id, &req)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

// CallServicePrompt 服务详情页提示测试:按参数渲染提示(prompts/get),返回消息与耗时。
func CallServicePrompt(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.GetPromptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.GetPrompt(userID, id, &req)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

func GetServiceResources(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resources, err := mcpServiceService.GetResources(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resources)
}

func GetServicePrompts(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	prompts, err := mcpServiceService.GetPrompts(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, prompts)
}

func GetServiceHealth(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.GetHealth(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

// GetServiceProcess 返回 stdio 服务子进程的资源占用快照(详情页 5s 轮询)。
func GetServiceProcess(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.GetProcessStat(userID, id)
	if err != nil {
		common.Error(c, http.StatusNotFound, "服务不存在")
		return
	}
	common.Success(c, resp)
}

// ControlServiceProcess 启动/停止/重启 stdio 服务子进程(总览卡片/详情页进程信息)。
func ControlServiceProcess(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.ProcessControlReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.ControlProcess(userID, id, req.Action)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// GetServicesOverview 服务总览页:统计摘要 + 全部服务的运行/资源快照(5s 轮询)。
// 所有登录用户可用(按 user_id 只看自己的服务);普通用户额外排除 stdio 服务
// (健康状态条视角)。
func GetServicesOverview(c *gin.Context) {
	userID := c.GetInt64("user_id")
	isAdmin := common.IsAdminRole(c.GetString("role"))
	resp, err := mcpServiceService.GetServicesOverview(userID, isAdmin)
	if err != nil {
		common.Error(c, http.StatusInternalServerError, "获取服务总览失败")
		return
	}
	common.Success(c, resp)
}

// --- 多秘钥管理(/services/:id/keys) ---

// GetServiceKeys 秘钥池视图(掩码值 + 模式 + 统计)。
func GetServiceKeys(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	resp, err := mcpServiceService.ListKeys(userID, id)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// UpdateServiceKeys 更新秘钥:追加(去重保状态)/ 替换全部。
func UpdateServiceKeys(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateServiceKeysReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.UpdateKeys(userID, id, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}

// SetServiceKeyStatus 启用/禁用单把秘钥。
func SetServiceKeyStatus(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	keyID, _ := strconv.ParseInt(c.Param("keyID"), 10, 64)
	var req dto.SetServiceKeyStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := mcpServiceService.SetKeyStatus(userID, id, keyID, &req); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// DeleteServiceKey 删除单把秘钥。
func DeleteServiceKey(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	keyID, _ := strconv.ParseInt(c.Param("keyID"), 10, 64)
	if err := mcpServiceService.DeleteKey(userID, id, keyID); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// BatchServiceKeys 批量操作:全部启用 / 删除已禁用。
func BatchServiceKeys(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.BatchServiceKeysReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := mcpServiceService.BatchKeys(userID, id, req.Action); err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, nil)
}

// UpdateServiceKeyConfig 模式切换:单↔多、随机↔轮询。
func UpdateServiceKeyConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req dto.UpdateServiceKeyConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Error(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := mcpServiceService.UpdateKeyConfig(userID, id, &req)
	if err != nil {
		common.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	common.Success(c, resp)
}
