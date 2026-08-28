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

// manualTestBilling 手动测试的计费结算结果(仅市场服务工具测试产生;其余测试恒为
// skipped=nil)。字段与网关计费列(§4.5)一一对应,由 recordManualTestLog 写入日志。
type manualTestBilling struct {
	Status      string  // skipped / charged / refunded / blocked / debt / pending
	BillingType string  // free / per_call
	UnitPrice   float64 // 单价快照(展示货币)
	Quota       int64   // 实扣额度
	PriceScope  string  // tool / service / global / free
	ItemID      *int64  // 关联市场项
}

// recordManualTestLog 服务详情页手动测试(工具/资源/提示)落调用日志:与网关调用
// 同口径写入 mcp_call_logs(type=Consume、Method/ToolName 对齐网关字段),使日志页
// 可见、健康状态条与健康回写(model.ApplyHealthWriteBack)同样吃到手动测试数据。
// Extra 标记 manual_test 便于区分真实网关流量。bill 为 nil(自有服务/资源/提示测试)
// 时计费恒为 skipped;市场服务工具测试传入实际结算结果(charged/refunded/blocked...)。
// 不递增用户请求次数(非网关请求);同步写入,手动测试非热路径。
func recordManualTestLog(svc *model.McpService, userID int64, method, target string, res *dto.CallToolResult, bill *manualTestBilling) {
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
	if bill != nil {
		if bill.Status != "" {
			l.BillingStatus = bill.Status
		}
		l.BillingType = bill.BillingType
		l.UnitPrice = bill.UnitPrice
		l.QuotaConsumed = bill.Quota
		l.PriceScope = bill.PriceScope
		l.MarketplaceItemID = bill.ItemID
	} else {
		// 资源/提示测试与自有服务工具测试不走计费(bill=nil),市场归属直接取
		// 服务行——条目健康按日志 marketplace_item_id 聚合,漏写则市场服务的
		// 这几类手动测试不计入平台健康(自有服务该值为 nil,行为不变)。
		l.MarketplaceItemID = svc.MarketplaceItemID
	}
	_ = l.Insert()
	model.ApplyHealthWriteBack(l)
}
