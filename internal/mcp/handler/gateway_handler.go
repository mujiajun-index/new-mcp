package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/smart"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
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

// 定价条目种类别名(billing.EntryKind*):本包内 billing 常被用作
// *billingOutcome 变量名而遮蔽包名,统一经别名引用。
const (
	priceKindTool     = billing.EntryKindTool
	priceKindResource = billing.EntryKindResource
	priceKindPrompt   = billing.EntryKindPrompt
)

// 失败计费判定(§6.6)同因遮蔽经别名引用。
var (
	shouldChargeCall  = billing.ShouldChargeCall
	toolResultIsError = billing.ToolResultIsError
)

type executeResult struct {
	Resp        *JSONRPCResponse
	ToolName    string
	ServiceID   int64
	ServiceName string
	GroupID     int64
	GroupName   string
	Billing     *billingOutcome // 市场来源服务计费结果(供日志记录);nil=自有/免费
	KeyIndex    int             // 多秘钥调用所用秘钥池内序号;0=单秘钥/不适用
}

// billingOutcome 一次调用的计费结算结果,写入 mcp_call_logs 计费列。
//
//	Status: skipped(自有/免费) / pending(已预扣待结算) / charged / refunded / blocked(余额不足) / debt(FailOpen 欠账)
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
		return h.handleInitialize(req, logCtx)
	case "notifications/initialized":
		return nil
	case "tools/list":
		return h.handleToolsList(ctx, req, logCtx)
	case "tools/call":
		return h.handleToolsCall(ctx, req, logCtx)
	case "resources/list":
		return h.handleResourcesList(ctx, req, logCtx)
	case "resources/read":
		return h.handleResourcesRead(ctx, req, logCtx)
	case "resources/templates/list":
		return h.handleResourceTemplatesList(ctx, req, logCtx)
	case "prompts/list":
		return h.handlePromptsList(ctx, req, logCtx)
	case "prompts/get":
		return h.handlePromptsGet(ctx, req, logCtx)
	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

// MCP 版本协商:与官方 TS/Go SDK 行为一致——客户端 initialize 请求的版本在
// 支持列表内则原样回显,否则回落到本网关支持的最新版本(客户端不支持时可自行断开)。
// 集合与 go-sdk v1.6.1 的 supportedProtocolVersions 保持同步。
var supportedProtocolVersions = []string{
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

const latestProtocolVersion = "2025-11-25"

func negotiateProtocolVersion(requested string) string {
	if slices.Contains(supportedProtocolVersions, requested) {
		return requested
	}
	return latestProtocolVersion
}

func (h *GatewayHandler) handleInitialize(req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(req.Params, &params)

	// resources/prompts 能力仅直连模式声明:智能模式的契约是"只经元工具渐进发现"
	// (mcp.search/mcp.describe/mcp.read),若声明能力,连接时自动枚举 resources/list
	// 的客户端会绕过元工具把聚合结果全量拉进上下文,违背智能模式的初衷。
	capabilities := map[string]interface{}{
		"tools": map[string]interface{}{},
	}
	if h.nativeItemsAllowed(logCtx) {
		// 未声明 subscribe/listChanged:网关的 HTTP 端点按请求生命周期工作,无法向
		// 客户端推送变更通知,客户端应自行重新 list。
		capabilities["resources"] = map[string]interface{}{}
		capabilities["prompts"] = map[string]interface{}{}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": negotiateProtocolVersion(params.ProtocolVersion),
			"capabilities":    capabilities,
			"serverInfo": map[string]string{
				"name":    "newmcp",
				"version": common.Version,
			},
			// V1.1: declare the local-image workflow globally so smart-mode clients
			// (whose tools/list returns only the meta-tools) still learn the path.
			// 智能模式再前置一段发现工作流:instructions 是 Tier-2 通道(部分客户端
			// 注入 system prompt、部分忽略),工具描述(Tier-1)已各自携带链路信息,
			// 这里只兜底教全局顺序。
			"instructions": smartModeInstructions(h.nativeItemsAllowed(logCtx)),
		},
	}
}

// smartInstructionsText 教智能模式的渐进发现工作流(search→describe→execute/read),
// 只在智能模式返回,避免直连模式下指向不存在的元工具。
const smartInstructionsText = "This gateway aggregates many MCP services behind 5 discovery tools. Workflow: mcp.search with task keywords in English plus the task language (English-dominant catalog — for non-English tasks query both) to find services/tools/resources/prompts; mcp.describe on a service name or \"service.toolName\" to inspect parameters; mcp.execute with tool_id \"service.toolName\" to call a tool; mcp.execute_batch to run several independent calls concurrently in one request — e.g. batch-controlling several switches or devices at once (never batch calls where one needs another's result or hits the same target in order); mcp.read with a newmcp://<service>/<uri> or <service>__<promptName> to fetch a resource or render a prompt. Describe a tool before executing it rather than guessing arguments."

const visionInstructionsText = "To analyze a LOCAL image: if it is small (roughly <= 10KB), inline it as base64 directly to analyze_image; otherwise call upload_image with local_path to get an upload_command matched to your OS (curl.exe on Windows PowerShell where bare `curl` is an alias for Invoke-WebRequest; curl elsewhere) + image_url, run it via your shell (no API key needed), then call analyze_image with the image_url. Never paste large image base64 into tool arguments."

func smartModeInstructions(nativeAllowed bool) string {
	if nativeAllowed {
		return visionInstructionsText
	}
	return smartInstructionsText + "\n\n" + visionInstructionsText
}

func (h *GatewayHandler) handleToolsList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if logCtx.GroupSlug == "" {
		// /mcp or /smart/mcp — mode driven by route, not group config
		if logCtx.ExposeMode == "direct" {
			tools, err := h.getDirectToolsForApiKey(logCtx.ApiKeyID)
			if err != nil {
				return h.errorResponse(req.ID, -32603, "Failed to get tools")
			}
			// upload_image is no longer appended globally; it is a per-config
			// built-in tool that travels with each vision service (see
			// service.buildToolsCache), so CollectToolsForGroups emits it as
			// vision_<id>__upload_image alongside the other vision tools.
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]interface{}{"tools": tools},
			}
		}
		return h.smartToolsResponse(req.ID)
	}

	// /mcp/group/:slug — mode from group config; slug resolves within the
	// authenticated user's own groups (per-user uniqueness)
	group, err := model.GetGroupBySlug(logCtx.UserID, logCtx.GroupSlug)
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
		tools := bridge.ToolsToMaps(entries) // upload_image is per-config now
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"tools": tools},
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
	var batchLogs []*model.McpCallLog
	var keyIndex int // 多秘钥调用所用秘钥池内序号(写日志 key_index)
	originalToolName := "" // records meta-tool name for smart mode
	// request_id:计费幂等键 = JSON-RPC id + 工具与参数短哈希。纯 id 在不同客户端/会话复用同一 id 时
	// 会把新逻辑请求误判为重试而漏扣;加入 tool/args 哈希后,仅真正的同请求重试(id/工具/参数全同)才命中跳过。
	requestID := billingRequestID(req.ID, params.Name, params.Arguments)

	// Resolve group info (within the authenticated user's groups)
	if logCtx.GroupSlug != "" {
		if group, err := model.GetGroupBySlug(logCtx.UserID, logCtx.GroupSlug); err == nil {
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
		keyIndex = result.KeyIndex
	case "mcp.execute_batch":
		// 批量执行:每项一条日志(method=mcp.execute_batch、计费按项结算),不走
		// 下方单条汇总;请求参数校验失败时 Logs 为空,回落单条日志路径。
		originalToolName = "mcp.execute_batch"
		batch := h.handleExecuteBatch(ctx, req.ID, logCtx, params.Arguments, groupID, groupName)
		resp = batch.Resp
		batchLogs = batch.Logs
	case "mcp.read":
		// 智能模式读资源/取提示:method 记 mcp.read(日志可区分智能模式调用),
		// tool_name 记目标 URI/提示名;计费与原生 resources/read、prompts/get 同口径
		// (市场服务且条目有价时扣费,幂等键复用外层 requestID)。
		originalToolName = "mcp.read"
		readResult := h.handleMetaRead(ctx, req.ID, logCtx, params.Arguments, requestID)
		resp = readResult.Resp
		billing = readResult.Billing
		if readResult.ToolName != "" {
			params.Name = readResult.ToolName
		}
		if readResult.ServiceID != 0 {
			serviceID = readResult.ServiceID
			serviceName = readResult.ServiceName
		}
		keyIndex = readResult.KeyIndex
	default:
		// Verify group access for group-scoped requests
		if logCtx.GroupSlug != "" {
			info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
			if err != nil {
				resp = h.errorResponse(req.ID, -32602, "Invalid API key")
			} else {
				group, gErr := model.GetGroupBySlug(logCtx.UserID, logCtx.GroupSlug)
				if gErr != nil {
					resp = h.errorResponse(req.ID, -32602, "Group not found: "+logCtx.GroupSlug)
				} else if !bridge.HasGroupAccess(info, group.Name) {
					resp = h.errorResponse(req.ID, -32602, "API key does not have access to group: "+logCtx.GroupSlug)
				}
			}
		}

		if resp == nil {
			billing = &billingOutcome{}
			resp = h.routeAndCall(ctx, req.ID, logCtx, params.Name, params.Arguments, &serviceID, &serviceName, requestID, billing, &keyIndex)
		}
	}

	// 批量路径已逐项记日志(一次请求仍只递增一次请求数),不走下方单条汇总。
	if batchLogs != nil {
		go h.recordLogs(batchLogs, logCtx.UserID)
		return resp
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

	// 直连模式(/mcp)请求未携带分组 slug,前面不会解析到分组。这里按已路由到的 service
	// 反查其所属分组回填,使调用日志能记录"工具所属分组"。与智能模式 handleExecute 中
	// resolveGroupForService 的做法一致。仅在尚未确定分组时执行:分组端点(/mcp/group/:slug)
	// 已按 slug 解析、mcp.execute 已按 service 解析的都会被跳过,不产生额外查询。
	if groupID == 0 && serviceID != 0 {
		groupID, groupName = h.resolveGroupForService(serviceID, logCtx)
	}

	callLog := &model.McpCallLog{
		Type:           model.LogTypeConsume,
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
		RequestPayload: redactRequestPayload(params.Name, params.Arguments),
		ResponseStatus: status,
		DurationMs:     int(duration.Milliseconds()),
		ErrorMessage:   truncate(errMsg, 65535),
		KeyIndex:       keyIndex,
		ClientIP:       logCtx.ClientIP,
		UserAgent:      truncate(logCtx.UserAgent, 512),
	}
	applyBillingToLog(callLog, billing)
	// 非市场来源调用(billing==nil:自有服务 mcp.read/mcp.execute、无条目价资源/提示
	// 读取等)日志缺市场 item 归属,按服务行点查回填(市场条目健康按 marketplace_item_id
	// 聚合,口径需完整)。市场计费路径已写入、search/describe 无服务归属,均不触发。
	if billing == nil && serviceID != 0 {
		callLog.MarketplaceItemID = model.GetServiceMarketplaceItemID(serviceID)
	}
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
func (h *GatewayHandler) routeAndCall(ctx context.Context, reqID interface{}, logCtx *LogContext, toolName string, args json.RawMessage, svcID *int64, svcName *string, requestID string, billing *billingOutcome, keyIndex *int) *JSONRPCResponse {
	parsedSvc, parsedTool := bridge.ParseNamespacedName(toolName)

	// Check virtual tools first (vision/camera 等属自有性质,免费). upload_image is
	// now a per-config vision tool, dispatched here like analyze_image (parsedSvc
	// is the real "vision_<id>" service name). The caller's userID is injected so
	// the upload branch can attribute ownership + enforce the per-user quota.
	if h.virtualRegistry != nil && parsedSvc != "" {
		if vSvcID, entry, ok := h.virtualRegistry.LookupByName(logCtx.UserID, parsedSvc); ok {
			vResult, vErr := h.virtualRegistry.Handle(virtual.WithCallerUserID(ctx, logCtx.UserID), vSvcID, entry.Config, parsedTool, args)
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
		if !h.preConsumeBilling(ctx, logCtx, session, priceKindTool, resolvedTool, requestID, billing) {
			return h.errorResponse(reqID, -32603, billing.BlockMsg)
		}
	}

	callParams := map[string]interface{}{
		"name":      resolvedTool,
		"arguments": args,
	}

	result, kIdx, err := callTool(ctx, session.Adapter, callParams)

	// 计费插入点 B:成功确认 / 失败退款(仅已启动计费的市场调用)。
	// 成败含结果内 isError(工具层失败,如上游 key 错误/余额不足):默认退款,
	// ChargeOnClientError/ChargeOnTimeout 打开时对应失败形态才计费(§6.6)。
	if session.Source == "marketplace" {
		h.finalizeBilling(billing, shouldChargeCall(err, toolResultIsError(result)))
	}
	if keyIndex != nil {
		*keyIndex = kIdx
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
		Query  string `json:"query"`
		Scope  string `json:"scope"`
		Group  string `json:"group"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Scope == "" {
		params.Scope = "all"
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	results, total, err := h.searchEngine.Search(ctx, logCtx.ApiKeyID, params.Query, smart.SearchOptions{
		Scope:  params.Scope,
		Group:  params.Group,
		Limit:  params.Limit,
		Offset: params.Offset,
	})
	if err != nil {
		return h.errorResponse(reqID, -32603, "Search failed: "+err.Error())
	}

	// 引擎侧同样钳制 limit/offset 并返回切片前的匹配总数;头部由 FormatSearchResult
	// 依据总数生成 "共 N 条 / 下一页 offset" 的分页提示。
	resultText := smart.FormatSearchResult(results, total, params.Offset, params.Query)

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

	text := smart.FormatDescribeResult(results, includeSchema)
	// Describe 对未知 target 静默跳过;部分命中时不点名缺哪个(缓存里没有对应关系),
	// 但至少提示"有 target 未找到"并把模型指回 mcp.search,避免它把残缺清单当全量。
	if requested := countNonEmptyTargets(params.Targets); len(results) < requested {
		text += "\n---\nNote: " + fmt.Sprintf("%d of %d targets were not found (services must be within your groups; tool names must be exact). Use mcp.search to find valid names.", requested-len(results), requested)
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
		},
	}
}

func countNonEmptyTargets(targets []string) int {
	n := 0
	for _, t := range targets {
		if t != "" {
			n++
		}
	}
	return n
}

// callOutcome 单次工具调用的执行结果(不含 JSON-RPC 包装),单次 mcp.execute 与
// 批量 mcp.execute_batch 共用同一条执行路径。
type callOutcome struct {
	Result      json.RawMessage // 成功时的上游 tools/call 结果(CallToolResult 原文);失败为 nil
	Err         string          // 网关级失败原因;空=成功
	ErrCode     int             // 网关级失败的 JSON-RPC 错误码(-32602/-32603)
	ToolName    string
	ServiceID   int64
	ServiceName string
	GroupID     int64
	GroupName   string
	Billing     *billingOutcome // 市场来源服务计费结果(供日志记录);nil=自有/免费
	KeyIndex    int             // 多秘钥调用所用秘钥池内序号;0=单秘钥/不适用(写日志 key_index)
}

// callTool 经可选的 MetaCaller 执行 tools/call,带回本次调用实际使用的秘钥序号
// (非多秘钥 adapter 恒 0),供调用日志 key_index 归因。
func callTool(ctx context.Context, adapter transport.TransportAdapter, params map[string]interface{}) (json.RawMessage, int, error) {
	if mc, ok := adapter.(transport.MetaCaller); ok {
		raw, meta, err := mc.CallWithMeta(ctx, "tools/call", params)
		return raw, meta.KeyIndex, err
	}
	raw, err := adapter.Call(ctx, "tools/call", params)
	return raw, 0, err
}

// executeOne 执行一次工具调用:作用域校验 → 虚拟工具 → 路由 → 计费 A/B → 上游调用。
// itemRequestID 为本次调用的计费幂等键。
func (h *GatewayHandler) executeOne(ctx context.Context, logCtx *LogContext, toolID string, arguments json.RawMessage, timeoutMs int, itemRequestID string) *callOutcome {
	if toolID == "" {
		return &callOutcome{Err: "tool_id is required", ErrCode: -32602}
	}

	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	// Verify group scope: the service must be in one of the API key's allowed groups
	svcName, _ := bridge.ParseNamespacedName(toolID)
	if svcName == "" {
		svcName = toolID // non-namespaced fallback
	}
	if !h.isServiceInApiKeyScope(svcName, logCtx) {
		return &callOutcome{Err: fmt.Sprintf("service '%s' is not accessible with this API key", svcName), ErrCode: -32602}
	}

	// Check virtual tools first
	if h.virtualRegistry != nil {
		parsedSvc, parsedTool := bridge.ParseNamespacedName(toolID)
		if parsedSvc != "" {
			if vSvcID, entry, ok := h.virtualRegistry.LookupByName(logCtx.UserID, parsedSvc); ok {
				vResult, vErr := h.virtualRegistry.Handle(virtual.WithCallerUserID(ctx, logCtx.UserID), vSvcID, entry.Config, parsedTool, arguments)
				gID, gName := h.resolveGroupForService(vSvcID, logCtx)
				if vErr != nil {
					return &callOutcome{
						Err:         "Virtual tool failed: " + vErr.Error(),
						ErrCode:     -32603,
						ToolName:    parsedTool,
						ServiceID:   vSvcID,
						ServiceName: entry.Name,
						GroupID:     gID,
						GroupName:   gName,
					}
				}
				return &callOutcome{
					Result:      json.RawMessage(vResult),
					ToolName:    parsedTool,
					ServiceID:   vSvcID,
					ServiceName: entry.Name,
					GroupID:     gID,
					GroupName:   gName,
				}
			}
		}
	}

	session, toolName, err := h.routeOrConnect(ctx, toolID, logCtx.UserID)
	if err != nil {
		return &callOutcome{Err: err.Error(), ErrCode: -32602}
	}

	gID, gName := h.resolveGroupForService(session.ServiceID, logCtx)

	// 计费插入点 A:预扣(仅 marketplace)。余额不足 → 拒绝,不调上游。
	var billing *billingOutcome
	if session.Source == "marketplace" {
		billing = &billingOutcome{}
		if !h.preConsumeBilling(ctx, logCtx, session, priceKindTool, toolName, itemRequestID, billing) {
			return &callOutcome{
				Err:         billing.BlockMsg,
				ErrCode:     -32603,
				ToolName:    toolName,
				ServiceID:   session.ServiceID,
				ServiceName: session.ServiceName,
				GroupID:     gID,
				GroupName:   gName,
				Billing:     billing,
			}
		}
	}

	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	callParams := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}

	result, keyIndex, err := callTool(ctx, session.Adapter, callParams)

	// 计费插入点 B:成功确认 / 失败退款(成败含结果内 isError,§6.6,同 routeAndCall)
	if billing != nil {
		h.finalizeBilling(billing, shouldChargeCall(err, toolResultIsError(result)))
	}

	oc := &callOutcome{
		ToolName:    toolName,
		ServiceID:   session.ServiceID,
		ServiceName: session.ServiceName,
		GroupID:     gID,
		GroupName:   gName,
		Billing:     billing,
		KeyIndex:    keyIndex,
	}
	if err != nil {
		oc.Err = "Execution failed: " + err.Error()
		oc.ErrCode = -32603
		return oc
	}
	oc.Result = json.RawMessage(result)
	return oc
}

func (h *GatewayHandler) handleExecute(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage, requestID string) *executeResult {
	var params struct {
		ToolID    string          `json:"tool_id"`
		Arguments json.RawMessage `json:"arguments"`
		TimeoutMs int             `json:"timeout_ms"`
	}
	_ = json.Unmarshal(args, &params)

	oc := h.executeOne(ctx, logCtx, params.ToolID, params.Arguments, params.TimeoutMs, requestID)
	if oc.Err != "" {
		return &executeResult{
			Resp:        h.errorResponse(reqID, oc.ErrCode, oc.Err),
			ToolName:    oc.ToolName,
			ServiceID:   oc.ServiceID,
			ServiceName: oc.ServiceName,
			GroupID:     oc.GroupID,
			GroupName:   oc.GroupName,
			Billing:     oc.Billing,
			KeyIndex:    oc.KeyIndex,
		}
	}
	return &executeResult{
		Resp: &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      reqID,
			Result:  json.RawMessage(oc.Result),
		},
		ToolName:    oc.ToolName,
		ServiceID:   oc.ServiceID,
		ServiceName: oc.ServiceName,
		GroupID:     oc.GroupID,
		GroupName:   oc.GroupName,
		Billing:     oc.Billing,
		KeyIndex:    oc.KeyIndex,
	}
}

// executeBatchMaxCalls/executeBatchMaxParallel 批量执行的条目上限与网关内并发上限。
// 上限同时写在 inputSchema(maxItems)里约束模型,此处是服务端兜底;并发用信号量钳制,
// 避免一次批量把上游(尤其市场服务的限流)瞬时打满。
const (
	executeBatchMaxCalls    = 10
	executeBatchMaxParallel = 5
)

type executeBatchCall struct {
	ToolID    string          `json:"tool_id"`
	Arguments json.RawMessage `json:"arguments"`
}

type executeBatchResult struct {
	Resp *JSONRPCResponse
	Logs []*model.McpCallLog // 逐项日志;校验失败时为 nil(回落单条日志路径)
}

// handleExecuteBatch 执行 mcp.execute_batch:各项并发(信号量限流)走与 mcp.execute
// 完全相同的 executeOne 路径。两种入参形态在入口归一化为 calls 后走同一下游:
//   - calls: [{tool_id, arguments}] —— 混合不同工具;
//   - tool_id + arguments_list —— 同工具扇出(批量控制开关/设备等场景的短形态,
//     少一层嵌套、无需逐项重复 tool_id,降低小参数量模型生成非法 JSON 的概率)。
// timeout_ms 为整批统一超时(不再逐项设置,减少逐项 schema 噪音)。
// 结果聚合为一个 CallToolResult(逐项 [index] 头 + 上游 content 原样透传,全失败才置
// isError);日志逐项落一条(method=mcp.execute_batch,计费列按项结算),分组归属沿用
// 单项路径的"项内解析优先、slug 分组兜底"口径。
func (h *GatewayHandler) handleExecuteBatch(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage, slugGroupID int64, slugGroupName string) *executeBatchResult {
	var params struct {
		ToolID        string             `json:"tool_id"`
		ArgumentsList []json.RawMessage  `json:"arguments_list"`
		Calls         []executeBatchCall `json:"calls"`
		TimeoutMs     int                `json:"timeout_ms"`
	}
	_ = json.Unmarshal(args, &params)

	fanout := params.ToolID != "" || len(params.ArgumentsList) > 0
	mixed := len(params.Calls) > 0

	calls := params.Calls
	switch {
	case fanout && mixed:
		return &executeBatchResult{Resp: h.errorResponse(reqID, -32602, "provide either calls (mixed tools) or tool_id + arguments_list (same tool), not both")}
	case !fanout && !mixed:
		return &executeBatchResult{Resp: h.errorResponse(reqID, -32602, "provide either calls (an array of {tool_id, arguments} entries, mixed tools) or tool_id + arguments_list (one arguments object per call, same tool)")}
	case fanout:
		if params.ToolID == "" {
			return &executeBatchResult{Resp: h.errorResponse(reqID, -32602, "tool_id is required with arguments_list")}
		}
		if len(params.ArgumentsList) == 0 {
			return &executeBatchResult{Resp: h.errorResponse(reqID, -32602, "arguments_list is required with tool_id")}
		}
		calls = make([]executeBatchCall, len(params.ArgumentsList))
		for i, a := range params.ArgumentsList {
			calls[i] = executeBatchCall{ToolID: params.ToolID, Arguments: a}
		}
	}

	if len(calls) > executeBatchMaxCalls {
		return &executeBatchResult{Resp: h.errorResponse(reqID, -32602, fmt.Sprintf("too many calls: %d (max %d); split into multiple batches", len(calls), executeBatchMaxCalls))}
	}
	for i, c := range calls {
		if c.ToolID == "" {
			return &executeBatchResult{Resp: h.errorResponse(reqID, -32602, fmt.Sprintf("calls[%d].tool_id is required", i))}
		}
	}

	timeoutMs := params.TimeoutMs

	// 计费幂等键带批内序号:批内两项 (tool_id, arguments) 完全相同时若共用键,
	// 第二项的预扣会被幂等去重跳过而漏扣。
	itemReqIDs := make([]string, len(calls))
	for i, c := range calls {
		itemReqIDs[i] = billingRequestID(reqID, fmt.Sprintf("%s#%d", c.ToolID, i), c.Arguments)
	}

	outcomes := make([]*callOutcome, len(calls))
	durations := make([]time.Duration, len(calls))
	sem := make(chan struct{}, executeBatchMaxParallel)
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			start := time.Now()
			outcomes[i] = h.executeOne(ctx, logCtx, calls[i].ToolID, calls[i].Arguments, timeoutMs, itemReqIDs[i])
			durations[i] = time.Since(start)
		}(i)
	}
	wg.Wait()

	content, failed := buildExecuteBatchContent(calls, outcomes)
	result := map[string]interface{}{"content": content}
	if failed == len(outcomes) {
		result["isError"] = true
	}

	logs := make([]*model.McpCallLog, 0, len(outcomes))
	for i, oc := range outcomes {
		gID, gName := slugGroupID, slugGroupName
		if oc.GroupID != 0 {
			gID, gName = oc.GroupID, oc.GroupName
		}
		status := "success"
		var errMsg string
		if oc.Err != "" {
			status = "error"
			errMsg = oc.Err
		}
		// 项级失败(作用域/路由)拿不到解析后的裸工具名时,用完整 tool_id 兜底,
		// 比 logs 里再记一条 mcp.execute_batch 更可查。
		toolName := oc.ToolName
		if toolName == "" {
			toolName = calls[i].ToolID
		}
		l := &model.McpCallLog{
			Type:           model.LogTypeConsume,
			UserID:         logCtx.UserID,
			Username:       logCtx.Username,
			ApiKeyID:       logCtx.ApiKeyID,
			ApiKeyName:     logCtx.ApiKeyName,
			GroupID:        gID,
			GroupName:      gName,
			ServiceID:      oc.ServiceID,
			ServiceName:    oc.ServiceName,
			ToolName:       toolName,
			Method:         "mcp.execute_batch",
			RequestID:      truncate(itemReqIDs[i], 64),
			RequestPayload: redactRequestPayload(toolName, calls[i].Arguments),
			ResponseStatus: status,
			DurationMs:     int(durations[i].Milliseconds()),
			ErrorMessage:   truncate(errMsg, 65535),
			KeyIndex:       oc.KeyIndex,
			ClientIP:       logCtx.ClientIP,
			UserAgent:      truncate(logCtx.UserAgent, 512),
		}
		applyBillingToLog(l, oc.Billing)
		logs = append(logs, l)
	}

	return &executeBatchResult{
		Resp: &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      reqID,
			Result:  result,
		},
		Logs: logs,
	}
}

// batchItemFallbackMaxChars 上游结果缺 content 块(如仅 structuredContent)或不可
// 解析时,退化为原文文本块的截断上限,保证模型至少能看到内容。
const batchItemFallbackMaxChars = 2048

// buildExecuteBatchContent 把批量各项结果聚合为一个 CallToolResult 的 content 数组:
// 首块汇总(N 项、几成败),其后每项一个 "[index] tool_id — ok|failed" 头块,成功项把
// 上游 content 块原样透传(text/image 等保持原类型,字节不重编码),失败项原因写进头。
// 失败=网关级错误或上游 isError=true(MCP 工具级错误语义)。返回 (content, 失败项数)。
func buildExecuteBatchContent(calls []executeBatchCall, outcomes []*callOutcome) ([]interface{}, int) {
	failed := 0
	blocks := make([]interface{}, 0, 2*len(outcomes)+1)
	for i, oc := range outcomes {
		itemOK := oc.Err == ""
		var itemBlocks []interface{}
		if itemOK {
			var cr struct {
				Content []json.RawMessage `json:"content"`
				IsError bool              `json:"isError"`
			}
			if err := json.Unmarshal(oc.Result, &cr); err == nil && len(cr.Content) > 0 {
				for _, b := range cr.Content {
					itemBlocks = append(itemBlocks, b)
				}
				if cr.IsError {
					itemOK = false
				}
			} else {
				itemBlocks = []interface{}{
					map[string]interface{}{"type": "text", "text": truncate(string(oc.Result), batchItemFallbackMaxChars)},
				}
			}
		}

		var header string
		switch {
		case itemOK:
			header = fmt.Sprintf("[%d] %s — ok", i, calls[i].ToolID)
		case oc.Err != "":
			header = fmt.Sprintf("[%d] %s — failed: %s", i, calls[i].ToolID, oc.Err)
			failed++
		default:
			header = fmt.Sprintf("[%d] %s — failed: tool reported an error", i, calls[i].ToolID)
			failed++
		}
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": header})
		blocks = append(blocks, itemBlocks...)
	}

	summary := fmt.Sprintf("Batch of %d calls: %d ok, %d failed.", len(outcomes), len(outcomes)-failed, failed)
	if failed > 0 {
		// 部分失败教下一步:失败项不影响其他项,修正后用 mcp.execute 单项重试
		// (依赖其他项结果的场景本来就不该批量)。
		summary += " Failed items did not complete; fix the cause and retry them individually with mcp.execute."
	}
	return append([]interface{}{map[string]interface{}{"type": "text", "text": summary}}, blocks...), failed
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
		session, err := h.connectServiceByName(ctx, svcName, userID)
		if err != nil {
			return nil, "", err
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
	// 共享 stdio 条目:会话池按条目键控(全部安装用户复用一个平台子进程);
	// 与 service 侧 materializeMarketplace 同步设置。
	svc.SharedProcess = item.TransportType == "stdio" && !item.IsolatedProcess
	return nil
}

// preConsumeBilling 计费插入点 A:解析条目级定价并预扣(§6.2)。
// 条目价命中即用;工具缺省/条目显式 inherit 走"服务→全局默认"链,
// 资源/提示缺省免费(显式 inherit 同工具回退服务价)。
// 返回 true=放行(含免费/欠账/已预扣),false=拒绝本次调用(余额不足/未定价/计费不可用)。
// 仅市场来源服务调用(调用方已按 session.Source == "marketplace" 判定)。
func (h *GatewayHandler) preConsumeBilling(ctx context.Context, logCtx *LogContext, session *bridge.McpSession, kind, entryName, requestID string, out *billingOutcome) bool {
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

	price, perr := billing.ResolveMarketplaceEntryPrice(*session.MarketplaceItemID, kind, entryName, user.Group)
	out.UnitPrice = price.UnitPriceDecimal
	out.BillingType = price.BillingType
	out.PriceScope = price.Scope

	// 价格未配置(非自用模式未显式定价):拒绝。须在免费判断之前——该错误返回的
	// base 是 free,先判免费会把未定价调用放行为免费(顺序 bug,原被上架门控掩盖)。
	if errors.Is(perr, billing.ErrPriceNotConfigured) {
		out.Status = "blocked"
		out.BlockMsg = "marketplace service price not configured"
		return false
	}
	// 免费:放行,记 skipped
	if price.BillingType == billing.BillingTypeFree || price.UnitPriceQuota <= 0 {
		out.Status = "skipped"
		return true
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
// charge=是否计费:调用方以 billing.ShouldChargeCall 判定(成败含结果内 isError,
// 客户端错误/超时受 ChargeOnClientError/ChargeOnTimeout 开关控制,§6.6)。
func (h *GatewayHandler) finalizeBilling(out *billingOutcome, charge bool) {
	if out == nil || out.sess == nil {
		return
	}
	if out.sess.Debt {
		out.Status = "debt"
		out.Quota = 0
		return
	}
	if charge {
		_ = h.billing.Confirm(out.sess)
		// 仅在真正扣费(ConsumedQuota>0)时记 charged;幂等重试等零消费记 skipped,
		// 避免"未实扣却显示已扣费"误导日志,并防止 HasChargedRequest 误命中零额度行。
		if out.sess.ConsumedQuota > 0 {
			out.Status = "charged"
			out.Quota = out.sess.ConsumedQuota
		} else {
			out.Status = "skipped"
			out.Quota = 0
		}
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
	// 健康回写:失败风暴标 unhealthy / 成功恢复 healthy(与手动测试共用,见 model.ApplyHealthWriteBack)
	model.ApplyHealthWriteBack(log)
	if log.UserID > 0 {
		_ = model.IncreaseUserRequestCount(log.UserID)
	}
}

// recordLogs 批量落多条日志(一次 mcp.execute_batch 请求逐项一行),但请求数只按
// 一次请求递增,与单次调用的统计口径对齐。
func (h *GatewayHandler) recordLogs(logs []*model.McpCallLog, userID int64) {
	for _, l := range logs {
		_ = l.Insert()
		model.ApplyHealthWriteBack(l)
	}
	if userID > 0 {
		_ = model.IncreaseUserRequestCount(userID)
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

// billingRequestID 派生计费幂等键:JSON-RPC id(可读前缀,便于与客户端日志关联)+ 工具名与参数的短哈希。
// 仅用 JSON-RPC id 时,不同客户端/会话复用同一 id 的请求会被误判为重试而漏扣;
// 加入 tool+args 哈希后,只有同一请求的重试(id/工具/参数全同)才命中幂等跳过。
func billingRequestID(rpcID interface{}, toolName string, args json.RawMessage) string {
	h := sha256.Sum256([]byte(toolName + "\x00" + string(args)))
	return fmt.Sprintf("%v:%x", rpcID, h[:8])
}

// visionImageTool reports whether the tool embeds a base64 image in its request
// arguments (vision analyze_image). The image payload is far too large to
// persist in call logs and is stripped by redactRequestPayload. Matches the
// same suffix rule used in internal/mcp/virtual/vision_handler.go.
func visionImageTool(name string) bool {
	return strings.HasSuffix(name, "analyze_image")
}

// redactRequestPayload returns the request arguments to persist in the call log.
// Vision image tools carry a full base64 image in the "image" field; it is
// replaced with a short placeholder (carrying the original size) so the log row
// stays small while the remaining arguments (e.g. the prompt) are preserved.
func redactRequestPayload(toolName string, args json.RawMessage) string {
	if visionImageTool(toolName) {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(args, &m); err == nil {
			if img, ok := m["image"]; ok {
				m["image"] = json.RawMessage(fmt.Sprintf(`"[redacted:%db]"`, len(img)))
				if b, err := json.Marshal(m); err == nil {
					return truncate(string(b), 65535)
				}
			}
		}
	}
	return truncate(string(args), 65535)
}
