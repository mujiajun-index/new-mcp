package service

import (
	"encoding/json"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// ManualTestTokenName 手动测试日志的令牌名占位:手动测试走会话鉴权、没有真实
// API 令牌,统一记为 "tool-test"(与前端日志页提示文案及令牌名称查询约定对齐,
// 见 web/src/features/logs/components/user-logs-page.tsx)。
const ManualTestTokenName = "tool-test"

// recordManualTestLog 服务详情页手动测试(工具/资源/提示)落调用日志:与网关调用
// 同口径写入 mcp_call_logs(type=Consume、Method/ToolName 对齐网关字段),使日志页
// 可见、健康状态条与健康回写(model.ApplyHealthWriteBack)同样吃到手动测试数据。
// Extra 标记 manual_test 便于区分真实网关流量。计费恒为 skipped(调试用途不计费),
// 不递增用户请求次数(非网关请求);同步写入,手动测试非热路径。
func recordManualTestLog(svc *model.McpService, userID int64, method, target string, res *dto.CallToolResult) {
	if res == nil {
		return
	}
	status := "success"
	errMsg := ""
	if res.IsError || res.Error != "" {
		status = "error"
		errMsg = res.Error
	}
	// tool_name 列宽 255,与网关 truncate(target, 255) 口径一致
	if len(target) > 255 {
		target = target[:255]
	}
	username := ""
	if userID > 0 {
		if u, err := model.GetUserByID(userID); err == nil && u != nil {
			username = u.Username
		}
	}
	extra, _ := json.Marshal(map[string]any{"manual_test": true})

	l := &model.McpCallLog{
		Type:           model.LogTypeConsume,
		UserID:         userID,
		Username:       username,
		ApiKeyName:     ManualTestTokenName, // 无真实令牌,占位标识手动测试来源
		ServiceID:      svc.ID,
		ServiceName:    svc.Name,
		ToolName:       target,
		Method:         method,
		ResponseStatus: status,
		DurationMs:     int(res.DurationMs),
		ErrorMessage:   errMsg,
		BillingStatus:  "skipped",
		Extra:          string(extra),
	}
	_ = l.Insert()
	model.ApplyHealthWriteBack(l)
}
