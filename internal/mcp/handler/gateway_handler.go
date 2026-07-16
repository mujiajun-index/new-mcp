package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/smart"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type LogContext struct {
	ApiKeyID   int64
	UserID     int64
	Username   string
	ApiKeyName string
	GroupSlug  string
	ClientIP   string
	UserAgent  string
	ExposeMode string // "direct" or "smart"
}

type executeResult struct {
	Resp        *JSONRPCResponse
	ToolName    string
	ServiceID   int64
	ServiceName string
	GroupID     int64
	GroupName   string
	Billing     *billingOutcome // 市场来源服务计费结果(供日志记录);nil=自有/免费
}

// billingOutcome 一次调用的计费结算结果,写入 mcp_call_logs 计费列。
//   Status: skipped(自有/免费) / pending(已预扣待结算) / charged / refunded / blocked(余额不足) / debt(FailOpen 欠账)
type billingOutcome struct {
	sess        *billing.BillingSession
	Status      string
	Quota       int64   // quota_consumed
	UnitPrice   float64 // 单价快照(展示货币)
	BillingType string  // free / per_call
	PriceScope  string  // tool/service/global/free
	ItemID      *int64  // marketplace_item_id
	BlockMsg    string  // blocked 时的错误信息
}

type GatewayHandler struct {
	pool            *bridge.SessionPool
	toolRouter      *bridge.ToolRouter
	searchEngine    *smart.SearchEngine
	virtualRegistry *virtual.VirtualToolRegistry
	billing         *billing.BillingService
}

func NewGatewayHandler(pool *bridge.SessionPool, toolRouter *bridge.ToolRouter, vr *virtual.VirtualToolRegistry) *GatewayHandler {
	return &GatewayHandler{
		pool:            pool,
		toolRouter:      toolRouter,
		searchEngine:    smart.NewSearchEngine(),
		virtualRegistry: vr,
		billing:         billing.NewBillingService(),
	}
}

func (h *GatewayHandler) HandleRequest(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return h.handleInitialize(req)
	case "notifications/initialized":
		return nil
	case "tools/list":
		return h.handleToolsList(ctx, req, logCtx)
	case "tools/call":
		return h.handleToolsCall(ctx, req, logCtx)
	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

func (h *GatewayHandler) handleInitialize(req *JSONRPCRequest) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    "newmcp",
				"version": "1.0.0",
			},
		},
	}
}

func (h *GatewayHandler) handleToolsList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if logCtx.GroupSlug == "" {
		// /mcp or /smart/mcp — mode driven by route, not group config
		if logCtx.ExposeMode == "direct" {
			tools, err := h.getDirectToolsForApiKey(logCtx.ApiKeyID)
			if err != nil {
				return h.errorResponse(req.ID, -32603, "Failed to get tools")
			}
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]interface{}{"tools": tools},
			}
		}
		return h.smartToolsResponse(req.ID)
	}

	// /mcp/group/:slug — mode from group config
	group, err := model.GetGroupBySlug(logCtx.GroupSlug)
	if err != nil {
		return h.errorResponse(req.ID, -32602, "Group not found: "+logCtx.GroupSlug)
	}

	info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
	if err != nil {
		return h.errorResponse(req.ID, -32602, "Invalid API key")
	}
	if !bridge.HasGroupAccess(info, group.Name) {
		return h.errorResponse(req.ID, -32602, "API key does not have access to group: "+logCtx.GroupSlug)
	}

	switch group.ExposeMode {
	case "direct":
		entries, err := bridge.CollectToolsForGroups([]model.McpGroup{*group}, false)
		if err != nil {
			return h.errorResponse(req.ID, -32603, "Failed to get tools")
		}
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"tools": bridge.ToolsToMaps(entries)},
		}
	default:
		return h.smartToolsResponse(req.ID)
	}
}

func (h *GatewayHandler) handleToolsCall(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return h.errorResponse(req.ID, -32602, "Invalid params")
	}

	start := time.Now()
	var resp *JSONRPCResponse
	var groupID int64
	var groupName string
	var serviceID int64
	var serviceName string
	var billing *billingOutcome
	originalToolName := "" // records meta-tool name for smart mode
	// request_id:JSON-RPC id 作为计费幂等键(MCP 客户端重试时稳定)
	requestID := fmt.Sprintf("%v", req.ID)

	// Resolve group info
	if logCtx.GroupSlug != "" {
		if group, err := model.GetGroupBySlug(logCtx.GroupSlug); err == nil {
			groupID = group.ID
			groupName = group.Name
		}
	}

	// Smart mode meta-tools
	switch params.Name {
	case "mcp.search":
		originalToolName = "mcp.search"
		resp = h.handleSearch(ctx, req.ID, logCtx, params.Arguments)
	case "mcp.describe":
		originalToolName = "mcp.describe"
		resp = h.handleDescribe(ctx, req.ID, logCtx, params.Arguments)
	case "mcp.execute":
		originalToolName = "mcp.execute"
		result := h.handleExecute(ctx, req.ID, logCtx, params.Arguments, requestID)
		resp = result.Resp
		billing = result.Billing
		if result.ToolName != "" {
			params.Name = result.ToolName
		}
		if result.ServiceID != 0 {
			serviceID = result.ServiceID
			serviceName = result.ServiceName
		}
		if result.GroupID != 0 {
			groupID = result.GroupID
			groupName = result.GroupName
		}
	default:
		// Verify group access for group-scoped requests
		if logCtx.GroupSlug != "" {
			info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
			if err != nil {
				resp = h.errorResponse(req.ID, -32602, "Invalid API key")
			} else {
				group, gErr := model.GetGroupBySlug(logCtx.GroupSlug)
				if gErr != nil {
					resp = h.errorResponse(req.ID, -32602, "Group not found: "+logCtx.GroupSlug)
				} else if !bridge.HasGroupAccess(info, group.Name) {
					resp = h.errorResponse(req.ID, -32602, "API key does not have access to group: "+logCtx.GroupSlug)
				}
			}
		}

		if resp == nil {
			billing = &billingOutcome{}
			resp = h.routeAndCall(ctx, req.ID, logCtx, params.Name, params.Arguments, &serviceID, &serviceName, requestID, billing)
		}
	}

	duration := time.Since(start)
	status := "success"
	var errMsg string
	if resp.Error != nil {
		status = "error"
		errMsg = resp.Error.Message
	}

	method := "tools/call"
	if originalToolName != "" {
		method = originalToolName
	}

	callLog := &model.McpCallLog{
		UserID:         logCtx.UserID,
		Username:       logCtx.Username,
		ApiKeyID:       logCtx.ApiKeyID,
		ApiKeyName:     logCtx.ApiKeyName,
		GroupID:        groupID,
		GroupName:      groupName,
		ServiceID:      serviceID,
		ServiceName:    serviceName,
		ToolName:       params.Name,
		Method:         method,
		RequestID:      truncate(requestID, 64),
		RequestPayload: truncate(string(params.Arguments), 65535),
		ResponseStatus: status,
		DurationMs:     int(duration.Milliseconds()),
		ErrorMessage:   truncate(errMsg, 65535),
		ClientIP:       logCtx.ClientIP,
		UserAgent:      truncate(logCtx.UserAgent, 512),
	}
	applyBillingToLog(callLog, billing)
	go h.recordLog(callLog)

	return resp
}

// applyBillingToLog 把计费结算结果写入日志的计费列(§4.5)。billing 为 nil 时保持默认 skipped。
func applyBillingToLog(log *model.McpCallLog, b *billingOutcome) {
	if b == nil {
		return
	}
	if b.Status != "" {
		log.BillingStatus = b.Status
	}
	log.BillingType = b.BillingType
	log.UnitPrice = b.UnitPrice
	log.QuotaConsumed = b.Quota
	log.PriceScope = b.PriceScope
	log.MarketplaceItemID = b.ItemID
}

// routeAndCall handles virtual tool check, session routing, and tool execution.
// 仅当解析到的服务为市场来源(source=marketplace)时触发计费(§6);自有/虚拟工具免费。
func (h *GatewayHandler) routeAndCall(ctx context.Context, reqID interface{}, logCtx *LogContext, toolName string, args json.RawMessage, svcID *int64, svcName *string, requestID string, billing *billingOutcome) *JSONRPCResponse {
	parsedSvc, parsedTool := bridge.ParseNamespacedName(toolName)

	// Check virtual tools first (vision/camera 等属自有性质,免费)
	if h.virtualRegistry != nil && parsedSvc != "" {
		if vSvcID, entry, ok := h.virtualRegistry.LookupByName(logCtx.UserID, parsedSvc); ok {
			vResult, vErr := h.virtualRegistry.Handle(ctx, vSvcID, entry.Config, parsedTool, args)
			*svcID = vSvcID
			*svcName = entry.Name
			if vErr != nil {
				return h.errorResponse(reqID, -32603, "Virtual tool failed: "+vErr.Error())
			}
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      reqID,
				Result:  json.RawMessage(vResult),
			}
		}
	}

	// Route to real MCP service
	session, resolvedTool, err := h.routeOrConnect(ctx, toolName, logCtx.UserID)
	if err != nil {
		return h.errorResponse(reqID, -32602, err.Error())
	}

	*svcID = session.ServiceID
	*svcName = session.ServiceName

	// 计费插入点 A:预扣(仅 marketplace)。余额不足 → 拒绝本次调用,不调上游。
	if session.Source == "marketplace" {
		if !h.preConsumeBilling(ctx, logCtx, session, resolvedTool, requestID, billing) {
			return h.errorResponse(reqID, -32603, billing.BlockMsg)
		}
	}

	callParams := map[string]interface{}{
		"name":      resolvedTool,
		"arguments": args,
	}

	result, err := session.Adapter.Call(ctx, "tools/call", callParams)

	// 计费插入点 B:成功确认 / 失败退款(仅已启动计费的市场调用)
	if session.Source == "marketplace" {
		h.finalizeBilling(billing, err == nil)
	}

	if err != nil {
		return h.errorResponse(reqID, -32603, "Tool execution failed: "+err.Error())
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result:  json.RawMessage(result),
	}
}

func (h *GatewayHandler) handleSearch(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage) *JSONRPCResponse {
	var params struct {
		Query string `json:"query"`
		Scope string `json:"scope"`
		Group string `json:"group"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Scope == "" {
		params.Scope = "all"
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}

	results, err := h.searchEngine.Search(ctx, logCtx.ApiKeyID, params.Query, smart.SearchOptions{
		Scope: params.Scope,
		Group: params.Group,
		Limit: params.Limit,
	})
	if err != nil {
		return h.errorResponse(reqID, -32603, "Search failed: "+err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d results:\n", len(results))
	for _, r := range results {
		if r.Doc.Type == "mcp" {
			label := r.Doc.Name
			if r.Doc.Name != r.Doc.ServiceName {
				label = fmt.Sprintf("%s (%s)", r.Doc.Name, r.Doc.ServiceName)
			}
			fmt.Fprintf(&sb, "- **%s** (service, %d tools) %s [%s]\n", label, r.Doc.ToolCount, r.Doc.Description, r.Doc.GroupName)
		} else {
			fmt.Fprintf(&sb, "- **%s.%s** (tool) %s [%s]\n", r.Doc.ServiceName, r.Doc.Name, r.Doc.Description, r.Doc.GroupName)
		}
	}
	resultText := sb.String()

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": resultText},
			},
		},
	}
}

func (h *GatewayHandler) handleDescribe(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage) *JSONRPCResponse {
	var params struct {
		Targets       []string `json:"targets"`
		IncludeSchema *bool    `json:"include_schema"`
	}
	_ = json.Unmarshal(args, &params)

	includeSchema := true
	if params.IncludeSchema != nil {
		includeSchema = *params.IncludeSchema
	}

	results, err := h.searchEngine.Describe(params.Targets, logCtx.ApiKeyID)
	if err != nil {
		return h.errorResponse(reqID, -32603, "Describe failed: "+err.Error())
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": smart.FormatDescribeResult(results, includeSchema)},
			},
		},
	}
}

func (h *GatewayHandler) handleExecute(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage, requestID string) *executeResult {
	var params struct {
		ToolID    string          `json:"tool_id"`
		Arguments json.RawMessage `json:"arguments"`
		TimeoutMs int             `json:"timeout_ms"`
	}
	_ = json.Unmarshal(args, &params)

	if params.ToolID == "" {
		return &executeResult{Resp: h.errorResponse(reqID, -32602, "tool_id is required")}
	}

	if params.TimeoutMs <= 0 {
		params.TimeoutMs = 30000
	}

	// Verify group scope: the service must be in one of the API key's allowed groups
	svcName, _ := bridge.ParseNamespacedName(params.ToolID)
	if svcName == "" {
		svcName = params.ToolID // non-namespaced fallback
	}
	if !h.isServiceInApiKeyScope(svcName, logCtx) {
		return &executeResult{Resp: h.errorResponse(reqID, -32602, fmt.Sprintf("service '%s' is not accessible with this API key", svcName))}
	}

	// Check virtual tools first
	if h.virtualRegistry != nil {
		parsedSvc, parsedTool := bridge.ParseNamespacedName(params.ToolID)
		if parsedSvc != "" {
			if vSvcID, entry, ok := h.virtualRegistry.LookupByName(logCtx.UserID, parsedSvc); ok {
				vResult, vErr := h.virtualRegistry.Handle(ctx, vSvcID, entry.Config, parsedTool, params.Arguments)
				if vErr != nil {
					return &executeResult{
						Resp:        h.errorResponse(reqID, -32603, "Virtual tool failed: "+vErr.Error()),
						ToolName:    parsedTool,
						ServiceID:   vSvcID,
						ServiceName: entry.Name,
					}
				}
				gID, gName := h.resolveGroupForService(vSvcID, logCtx)
				return &executeResult{
					Resp: &JSONRPCResponse{
						JSONRPC: "2.0",
						ID:      reqID,
						Result:  json.RawMessage(vResult),
					},
					ToolName:    parsedTool,
					ServiceID:   vSvcID,
					ServiceName: entry.Name,
					GroupID:     gID,
					GroupName:   gName,
				}
			}
		}
	}

	session, toolName, err := h.routeOrConnect(ctx, params.ToolID, logCtx.UserID)
	if err != nil {
		return &executeResult{Resp: h.errorResponse(reqID, -32602, err.Error())}
	}

	gID, gName := h.resolveGroupForService(session.ServiceID, logCtx)

	// 计费插入点 A:预扣(仅 marketplace)。余额不足 → 拒绝,不调上游。
	var billing *billingOutcome
	if session.Source == "marketplace" {
		billing = &billingOutcome{}
		if !h.preConsumeBilling(ctx, logCtx, session, toolName, requestID, billing) {
			return &executeResult{
				Resp:        h.errorResponse(reqID, -32603, billing.BlockMsg),
				ToolName:    toolName,
				ServiceID:   session.ServiceID,
				ServiceName: session.ServiceName,
				GroupID:     gID,
				GroupName:   gName,
				Billing:     billing,
			}
		}
	}

	if params.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(params.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	callParams := map[string]interface{}{
		"name":      toolName,
		"arguments": params.Arguments,
	}

	result, err := session.Adapter.Call(ctx, "tools/call", callParams)

	// 计费插入点 B:成功确认 / 失败退款
	if billing != nil {
		h.finalizeBilling(billing, err == nil)
	}

	if err != nil {
		return &executeResult{
			Resp:        h.errorResponse(reqID, -32603, "Execution failed: "+err.Error()),
			ToolName:    toolName,
			ServiceID:   session.ServiceID,
			ServiceName: session.ServiceName,
			GroupID:     gID,
			GroupName:   gName,
			Billing:     billing,
		}
	}

	return &executeResult{
		Resp: &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      reqID,
			Result:  json.RawMessage(result),
		},
		ToolName:    toolName,
		ServiceID:   session.ServiceID,
		ServiceName: session.ServiceName,
		GroupID:     gID,
		GroupName:   gName,
		Billing:     billing,
	}
}

// resolveGroupForService finds the first group containing this service within the API key's scope.
func (h *GatewayHandler) resolveGroupForService(serviceID int64, logCtx *LogContext) (int64, string) {
	info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
	if err != nil {
		return 0, ""
	}
	groups, err := bridge.GetGroupsForApiKey(info)
	if err != nil {
		return 0, ""
	}
	// Batched: all (group, service) pairs via two queries, instead of one per group.
	pairs, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil {
		return 0, ""
	}
	for _, p := range pairs {
		if p.Service.ID == serviceID {
			return p.Group.ID, p.Group.Name
		}
	}
	return 0, ""
}

// isServiceInApiKeyScope checks whether a service name is within the API key's group scope.
func (h *GatewayHandler) isServiceInApiKeyScope(serviceName string, logCtx *LogContext) bool {
	info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
	if err != nil {
		return false
	}
	groups, err := bridge.GetGroupsForApiKey(info)
	if err != nil {
		return false
	}
	// Batched: all services in scope via two queries, instead of one per group+service.
	pairs, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil {
		return false
	}
	for _, p := range pairs {
		if svc := p.Service; svc.Name == serviceName || svc.DisplayName == serviceName {
			return true
		}
	}
	return false
}

func (h *GatewayHandler) getDirectToolsForApiKey(apiKeyID int64) ([]map[string]interface{}, error) {
	info, err := bridge.ResolveApiKeyInfo(apiKeyID)
	if err != nil {
		return nil, err
	}

	groups, err := bridge.GetGroupsForApiKey(info)
	if err != nil {
		return nil, err
	}

	entries, err := bridge.CollectToolsForGroups(groups, true)
	if err != nil {
		return nil, err
	}

	return bridge.ToolsToMaps(entries), nil
}

func (h *GatewayHandler) routeOrConnect(ctx context.Context, namespacedTool string, userID int64) (*bridge.McpSession, string, error) {
	session, toolName, err := h.toolRouter.Route(namespacedTool, userID)
	if err == nil {
		return session, toolName, nil
	}

	svcName, parsedToolName := bridge.ParseNamespacedName(namespacedTool)
	if svcName != "" {
		var svc model.McpService
		if dbErr := model.DB.Where("name = ? AND user_id = ?", svcName, userID).First(&svc).Error; dbErr != nil {
			return nil, "", fmt.Errorf("service not found: %s", svcName)
		}
		if !h.userOwnedServicesAllowed(svc.Source) {
			return nil, "", fmt.Errorf("user-owned services are disabled")
		}
		if mErr := h.materializeMarketplaceConfig(&svc); mErr != nil {
			return nil, "", mErr
		}
		session, connErr := h.pool.GetOrConnect(ctx, &svc)
		if connErr != nil {
			return nil, "", fmt.Errorf("failed to connect service %s: %v", svcName, connErr)
		}
		return session, parsedToolName, nil
	}

	// Non-namespaced: search DB for a service that has this tool
	var services []model.McpService
	model.DB.Where("user_id = ?", userID).Find(&services)
	for i := range services {
		var tools []struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(services[i].ToolsCache), &tools) == nil {
			for _, t := range tools {
				if t.Name == namespacedTool {
					if !h.userOwnedServicesAllowed(services[i].Source) {
						return nil, "", fmt.Errorf("user-owned services are disabled")
					}
					if mErr := h.materializeMarketplaceConfig(&services[i]); mErr != nil {
						return nil, "", mErr
					}
					session, connErr := h.pool.GetOrConnect(ctx, &services[i])
					if connErr != nil {
						return nil, "", fmt.Errorf("failed to connect service %s: %v", services[i].Name, connErr)
					}
					return session, namespacedTool, nil
				}
			}
		}
	}

	return nil, "", fmt.Errorf("tool not found: %s", namespacedTool)
}

// userOwnedServicesAllowed 报告当前是否允许调用给定来源的服务(§7.5)。
// UserOwnedServicesEnabled=false(纯市场模式)时,仅禁止 source=user 自有服务;
// 市场引用/平台/虚拟服务不受影响。
func (h *GatewayHandler) userOwnedServicesAllowed(source string) bool {
	if source == "user" && !model.GetOptionBool("UserOwnedServicesEnabled") {
		return false
	}
	return true
}

// materializeMarketplaceConfig 为市场引用服务(source=marketplace)从 marketplace_items
// 注入平台上游配置/凭证到内存 McpService(不落库:引用行 config 始终为空,凭证不暴露给用户)。
// transport_type 哨兵值 "marketplace" 会被还原为市场项的真实 transport。
func (h *GatewayHandler) materializeMarketplaceConfig(svc *model.McpService) error {
	if svc.Source != "marketplace" || svc.MarketplaceItemID == nil {
		return nil
	}
	item, err := model.GetMarketplaceItemByID(*svc.MarketplaceItemID)
	if err != nil {
		return fmt.Errorf("marketplace item not found for service %s", svc.Name)
	}
	if item.Status != common.StatusEnabled {
		return fmt.Errorf("marketplace item %s is not available", item.Name)
	}
	// config_template 加密落库(§4.3):平台凭证 Decrypt 后注入;存量明文项 Decrypt 失败则回退原值
	if plain, dErr := common.Decrypt(item.ConfigTemplate); dErr == nil && plain != "" {
		svc.Config = plain
	} else {
		svc.Config = item.ConfigTemplate // 兼容未加密的存量项
	}
	if svc.TransportType == "" || svc.TransportType == "marketplace" {
		svc.TransportType = item.TransportType
	}
	return nil
}

// preConsumeBilling 计费插入点 A:解析 3 级定价并预扣(§6.2)。
// 返回 true=放行(含免费/欠账/已预扣),false=拒绝本次调用(余额不足/未定价/计费不可用)。
// 仅市场来源服务调用(调用方已按 session.Source == "marketplace" 判定)。
func (h *GatewayHandler) preConsumeBilling(ctx context.Context, logCtx *LogContext, session *bridge.McpSession, toolName, requestID string, out *billingOutcome) bool {
	out.ItemID = session.MarketplaceItemID
	if session.MarketplaceItemID == nil {
		out.Status = "skipped"
		return true // 无市场项关联,不计费
	}
	user, err := model.GetUserByID(logCtx.UserID)
	if err != nil {
		out.Status = "skipped"
		return true // 无法取用户信息:FailOpen 放行(免费)
	}

	price, perr := billing.ResolveMarketplacePrice(*session.MarketplaceItemID, toolName, user.Group)
	out.UnitPrice = price.UnitPriceDecimal
	out.BillingType = price.BillingType
	out.PriceScope = price.Scope

	// 免费:放行,记 skipped
	if price.BillingType == billing.BillingTypeFree || price.UnitPriceQuota <= 0 {
		out.Status = "skipped"
		return true
	}
	// 价格未配置(非自用模式未显式定价):拒绝
	if errors.Is(perr, billing.ErrPriceNotConfigured) {
		out.Status = "blocked"
		out.BlockMsg = "marketplace service price not configured"
		return false
	}
	// 价格加载失败:FailOpen 放行
	if perr != nil {
		out.Status = "skipped"
		return true
	}

	sess, err := h.billing.PreConsume(billing.PreConsumeRequest{
		Price:     price,
		UserID:    logCtx.UserID,
		ApiKeyID:  logCtx.ApiKeyID,
		UserRole:  user.Role,
		RequestID: requestID,
	})
	if errors.Is(err, billing.ErrInsufficientQuota) {
		out.Status = "blocked"
		out.BlockMsg = "用户额度不足,剩余额度不足,请充值或兑换"
		return false
	}
	if err != nil {
		// 计费 DB 异常且非 FailOpen:拒绝;FailOpen 时 PreConsume 返回 nil err + sess.Debt
		out.Status = "blocked"
		out.BlockMsg = "计费服务暂时不可用,请稍后重试"
		return false
	}
	out.sess = sess
	if sess.Debt {
		out.Status = "debt"
	} else {
		out.Status = "pending"
	}
	return true
}

// finalizeBilling 计费插入点 B:成功确认 / 失败退款(§6.2)。仅对已启动计费的会话结算。
func (h *GatewayHandler) finalizeBilling(out *billingOutcome, success bool) {
	if out == nil || out.sess == nil {
		return
	}
	if out.sess.Debt {
		out.Status = "debt"
		out.Quota = 0
		return
	}
	if success {
		_ = h.billing.Confirm(out.sess)
		out.Status = "charged"
		out.Quota = out.sess.ConsumedQuota
	} else {
		_ = h.billing.Refund(out.sess)
		out.Status = "refunded"
		out.Quota = 0
	}
}

func (h *GatewayHandler) errorResponse(id interface{}, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}

func (h *GatewayHandler) recordLog(log *model.McpCallLog) {
	_ = log.Insert()
	if log.UserID > 0 {
		_ = model.IncreaseUserRequestCount(log.UserID)
	}
}

func (h *GatewayHandler) smartToolsResponse(id interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]interface{}{"tools": smart.MetaTools},
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
