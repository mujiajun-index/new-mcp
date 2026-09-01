package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

// 网关对上游 Resources/Prompts 做聚合透传:
//   - 资源 URI 加命名空间前缀 newmcp://{service}/{原始URI},消除多服务间的 URI 冲突;
//     resources/read 时按前缀路由回源。资源模板的 uriTemplate 同样加前缀,客户端
//     按模板展开出的 URI 依然能路由。
//   - 提示名与工具一致采用 {service}__{name} 命名空间(ParseNamespacedName 解析)。
//   - 分组内可按条目勾选启停(mcp_group_items,无行=启用):list 聚合时剔除禁用条目,
//     resources/read、prompts/get 拒绝禁用条目(否则隐藏但仍可直读,勾选形同虚设)。
//     模板禁用只隐藏 templates/list 条目;按模板展开出的 URI 读取不做前缀匹配拦截。
//   - resources/read、prompts/get 会真实调用上游,计入调用日志;市场来源服务按
//     条目级定价计费(条目价命中即用;资源/提示缺省免费,显式 inherit 继承服务价,
//     与 tools/call 同一套预扣→确认/退款链路)。list 聚合仍不计费。
//   - 仅直连模式开放原生 list/能力声明;智能模式下资源/提示经元工具发现(见上
//     nativeItemsAllowed),list 返回空、initialize 不声明 resources/prompts。
const gatewayURIScheme = "newmcp"

// 条目种类常量,与 model.McpGroupItem.ItemKind 对应。
const (
	itemKindResource = "resource"
	itemKindTemplate = "template"
	itemKindPrompt   = "prompt"
)

// upstreamFanoutConcurrency 聚合类请求(resources/list 等)并发连上游的上限。
const upstreamFanoutConcurrency = 8

func gatewayResourceURI(serviceName, upstreamURI string) string {
	return gatewayURIScheme + "://" + serviceName + "/" + upstreamURI
}

// parseGatewayResourceURI 从网关 URI 拆出 (服务名, 上游原始 URI)。非网关前缀、
// 服务名或 URI 为空都视为非法。
func parseGatewayResourceURI(uri string) (serviceName, upstreamURI string, ok bool) {
	rest := strings.TrimPrefix(uri, gatewayURIScheme+"://")
	if len(rest) == len(uri) {
		return "", "", false
	}
	idx := strings.Index(rest, "/")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// itemFilter 分组条目级禁用集合:key "groupID:serviceID:kind:itemKey"。
// 仅装载 Enabled=false 的行(无行=启用);加载失败返回空集合(fail-open,与工具过滤的容错一致)。
type itemFilter map[string]bool

func loadItemFilter(groupIDs []int64, kinds ...string) itemFilter {
	rows, err := model.GetGroupItemsByGroupIDsAndKinds(groupIDs, kinds)
	if err != nil {
		return nil
	}
	f := itemFilter{}
	for _, r := range rows {
		if !r.Enabled {
			f[itemFilterKey(r.GroupID, r.ServiceID, r.ItemKind, r.ItemKey)] = true
		}
	}
	return f
}

func itemFilterKey(groupID, serviceID int64, kind, key string) string {
	return fmt.Sprintf("%d:%d:%s:%s", groupID, serviceID, kind, key)
}

func (f itemFilter) disabled(groupID, serviceID int64, kind, key string) bool {
	return f[itemFilterKey(groupID, serviceID, kind, key)]
}

// scopeEntry 范围内一个服务及其"归属分组"(去重后首次出现的分组,
// 与 tools 聚合的去重取首规则一致,过滤以该分组为准)。
type scopeEntry struct {
	svc     *model.McpService
	groupID int64
}

// servicesInScope 返回该请求可见的服务(去重、排除 vision/camera 虚拟服务)。
// GroupSlug 非空时限定该分组并校验访问权;否则聚合 API Key 绑定分组的全部服务,
// 与 tools/list 的取数口径一致。
func (h *GatewayHandler) servicesInScope(logCtx *LogContext) ([]scopeEntry, error) {
	var groups []model.McpGroup
	if logCtx.GroupSlug != "" {
		group, err := model.GetGroupBySlug(logCtx.UserID, logCtx.GroupSlug)
		if err != nil {
			return nil, fmt.Errorf("group not found: %s", logCtx.GroupSlug)
		}
		info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
		if err != nil {
			return nil, fmt.Errorf("invalid API key")
		}
		if !bridge.HasGroupAccess(info, group.Name) {
			return nil, fmt.Errorf("API key does not have access to group: %s", logCtx.GroupSlug)
		}
		groups = []model.McpGroup{*group}
	} else {
		info, err := bridge.ResolveApiKeyInfo(logCtx.ApiKeyID)
		if err != nil {
			return nil, err
		}
		if groups, err = bridge.GetGroupsForApiKey(info); err != nil {
			return nil, err
		}
	}

	pairs, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]bool)
	var scope []scopeEntry
	for _, p := range pairs {
		if seen[p.Service.ID] || p.Service.TransportType == "virtual" {
			continue
		}
		seen[p.Service.ID] = true
		svc := p.Service
		scope = append(scope, scopeEntry{svc: &svc, groupID: p.Group.ID})
	}
	return scope, nil
}

// aggregateUpstream 并发遍历范围内服务并执行 fetch,按范围内原顺序收集成功结果。
// 单个服务连接/拉取失败只跳过该服务(降级为无资源/无提示,与 tools 对故障上游的
// 容错口径一致),不让整个响应失败。
func (h *GatewayHandler) aggregateUpstream(ctx context.Context, scope []scopeEntry, fetch func(session *bridge.McpSession, groupID int64) ([]map[string]interface{}, error)) []map[string]interface{} {
	results := make([][]map[string]interface{}, len(scope))
	sem := make(chan struct{}, upstreamFanoutConcurrency)
	var wg sync.WaitGroup
	for i := range scope {
		wg.Add(1)
		go func(idx int, entry scopeEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if !h.userOwnedServicesAllowed(entry.svc.Source) {
				return
			}
			if err := h.materializeMarketplaceConfig(entry.svc); err != nil {
				return
			}
			session, err := h.pool.GetOrConnect(ctx, entry.svc)
			if err != nil {
				return
			}
			if items, err := fetch(session, entry.groupID); err == nil {
				results[idx] = items
			}
		}(i, scope[i])
	}
	wg.Wait()

	var out []map[string]interface{}
	for _, r := range results {
		out = append(out, r...)
	}
	return out
}

// connectServiceByName 按服务名定位并连接该用户的服务:先查会话池热连接,
// 未命中则回源 DB(含市场引用配置物化)后建连。
func (h *GatewayHandler) connectServiceByName(ctx context.Context, serviceName string, userID int64) (*bridge.McpSession, error) {
	if session := h.pool.GetByNameForUser(serviceName, userID); session != nil && session.Adapter.IsConnected() {
		return session, nil
	}
	var svc model.McpService
	if err := model.DB.Where("name = ? AND user_id = ?", serviceName, userID).First(&svc).Error; err != nil {
		return nil, fmt.Errorf("service not found: %s", serviceName)
	}
	if !h.userOwnedServicesAllowed(svc.Source) {
		return nil, fmt.Errorf("user-owned services are disabled")
	}
	if err := h.materializeMarketplaceConfig(&svc); err != nil {
		return nil, err
	}
	session, err := h.pool.GetOrConnect(ctx, &svc)
	if err != nil {
		return nil, fmt.Errorf("failed to connect service %s: %v", serviceName, err)
	}
	return session, nil
}

// unmarshalEntryList 把上游 List* 结果拆成条目 map 列表,便于改写字段后重组。
func unmarshalEntryList(raw json.RawMessage, field string) []map[string]interface{} {
	var res map[string]json.RawMessage
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil
	}
	listRaw, ok := res[field]
	if !ok {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(listRaw, &items); err != nil {
		return nil
	}
	return items
}

// nativeItemsAllowed 原生 resources/prompts 方法对该端点是否开放:仅直连模式开放。
// 智能模式(/smart/mcp 或 expose_mode=smart 的分组端点)的资源/提示一律经元工具
// (mcp.search/mcp.describe/mcp.read)渐进发现——list 类方法返回空、initialize 不声明
// 能力,避免连接时自动枚举的客户端绕过元工具全量拉取。resources/read、prompts/get
// 保留可用:单 URI 定点读取无枚举开销,且与 mcp.read 共用同一上游核心。
func (h *GatewayHandler) nativeItemsAllowed(logCtx *LogContext) bool {
	if logCtx.GroupSlug == "" {
		return logCtx.ExposeMode == "direct"
	}
	group, err := model.GetGroupBySlug(logCtx.UserID, logCtx.GroupSlug)
	if err != nil {
		return false
	}
	return group.ExposeMode == "direct"
}

func (h *GatewayHandler) handleResourcesList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if !h.nativeItemsAllowed(logCtx) {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"resources": []map[string]interface{}{}},
		}
	}
	scope, err := h.servicesInScope(logCtx)
	if err != nil {
		return h.errorResponse(req.ID, -32602, err.Error())
	}
	disabled := loadItemFilter(scopeGroupIDs(scope), itemKindResource)

	resources := h.aggregateUpstream(ctx, scope, func(session *bridge.McpSession, groupID int64) ([]map[string]interface{}, error) {
		raw, err := session.Adapter.ListResources(ctx)
		if err != nil {
			return nil, err
		}
		var out []map[string]interface{}
		for _, item := range unmarshalEntryList(raw, "resources") {
			uri, _ := item["uri"].(string)
			if uri == "" {
				out = append(out, item) // 无 URI 的条目无从过滤,保留
				continue
			}
			if disabled.disabled(groupID, session.ServiceID, itemKindResource, uri) {
				continue
			}
			item["uri"] = gatewayResourceURI(session.ServiceName, uri)
			out = append(out, item)
		}
		return out, nil
	})

	if resources == nil {
		resources = []map[string]interface{}{}
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"resources": resources},
	}
}

func (h *GatewayHandler) handleResourceTemplatesList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if !h.nativeItemsAllowed(logCtx) {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"resourceTemplates": []map[string]interface{}{}},
		}
	}
	scope, err := h.servicesInScope(logCtx)
	if err != nil {
		return h.errorResponse(req.ID, -32602, err.Error())
	}
	disabled := loadItemFilter(scopeGroupIDs(scope), itemKindTemplate)

	templates := h.aggregateUpstream(ctx, scope, func(session *bridge.McpSession, groupID int64) ([]map[string]interface{}, error) {
		raw, err := session.Adapter.ListResourceTemplates(ctx)
		if err != nil {
			return nil, err
		}
		var out []map[string]interface{}
		for _, item := range unmarshalEntryList(raw, "resourceTemplates") {
			tpl, _ := item["uriTemplate"].(string)
			if tpl == "" {
				out = append(out, item)
				continue
			}
			if disabled.disabled(groupID, session.ServiceID, itemKindTemplate, tpl) {
				continue
			}
			item["uriTemplate"] = gatewayResourceURI(session.ServiceName, tpl)
			out = append(out, item)
		}
		return out, nil
	})

	if templates == nil {
		templates = []map[string]interface{}{}
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"resourceTemplates": templates},
	}
}

func (h *GatewayHandler) handlePromptsList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if !h.nativeItemsAllowed(logCtx) {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"prompts": []map[string]interface{}{}},
		}
	}
	scope, err := h.servicesInScope(logCtx)
	if err != nil {
		return h.errorResponse(req.ID, -32602, err.Error())
	}
	disabled := loadItemFilter(scopeGroupIDs(scope), itemKindPrompt)

	prompts := h.aggregateUpstream(ctx, scope, func(session *bridge.McpSession, groupID int64) ([]map[string]interface{}, error) {
		raw, err := session.Adapter.ListPrompts(ctx)
		if err != nil {
			return nil, err
		}
		var out []map[string]interface{}
		for _, item := range unmarshalEntryList(raw, "prompts") {
			name, _ := item["name"].(string)
			if name == "" {
				out = append(out, item)
				continue
			}
			if disabled.disabled(groupID, session.ServiceID, itemKindPrompt, name) {
				continue
			}
			item["name"] = session.ServiceName + "__" + name
			out = append(out, item)
		}
		return out, nil
	})

	if prompts == nil {
		prompts = []map[string]interface{}{}
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"prompts": prompts},
	}
}

func scopeGroupIDs(scope []scopeEntry) []int64 {
	seen := make(map[int64]bool)
	var ids []int64
	for _, e := range scope {
		if !seen[e.groupID] {
			seen[e.groupID] = true
			ids = append(ids, e.groupID)
		}
	}
	return ids
}

func (h *GatewayHandler) handleResourcesRead(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.URI == "" {
		return h.errorResponse(req.ID, -32602, "Invalid params: uri is required")
	}

	serviceName, upstreamURI, ok := parseGatewayResourceURI(params.URI)
	if !ok {
		return h.errorResponse(req.ID, -32602, "Invalid resource URI, expected "+gatewayURIScheme+"://<service>/<upstream-uri>")
	}

	// 计费幂等键(id+URI 哈希):仅同请求重试命中,不同 URI 各自计费。
	requestID := billingRequestID(req.ID, params.URI, nil)
	start := time.Now()
	resp, serviceID, resolvedName, bill, keyIndex := h.readUpstreamResource(ctx, req.ID, logCtx, serviceName, upstreamURI, requestID)
	h.recordConsumeLog(logCtx, serviceID, resolvedName, "resources/read", params.URI, requestID, resp, bill, keyIndex, start)
	return resp
}

func (h *GatewayHandler) readUpstreamResource(ctx context.Context, reqID interface{}, logCtx *LogContext, serviceName, upstreamURI, requestID string) (*JSONRPCResponse, int64, string, *billingOutcome, int) {
	if !h.isServiceInApiKeyScope(serviceName, logCtx) {
		resp := h.errorResponse(reqID, -32602, fmt.Sprintf("service '%s' is not accessible with this API key", serviceName))
		return resp, 0, serviceName, nil, 0
	}

	session, err := h.connectServiceByName(ctx, serviceName, logCtx.UserID)
	if err != nil {
		return h.errorResponse(reqID, -32602, err.Error()), 0, serviceName, nil, 0
	}

	if h.itemDisabledInOwningGroup(logCtx, session.ServiceID, itemKindResource, upstreamURI) {
		return h.errorResponse(reqID, -32602, "resource is disabled in its group"), session.ServiceID, session.ServiceName, nil, 0
	}

	// 计费插入点 A:市场来源按条目价预扣(条目键=上游原始 URI,与管理端快照一致;
	// 无条目价缺省免费放行,显式 inherit 按服务价)。余额不足/未定价 → 拒绝,不调上游。
	var bill *billingOutcome
	if session.Source == "marketplace" {
		bill = &billingOutcome{}
		if !h.preConsumeBilling(ctx, logCtx, session, priceKindResource, upstreamURI, requestID, bill) {
			return h.errorResponse(reqID, -32603, bill.BlockMsg), session.ServiceID, session.ServiceName, bill, 0
		}
	}

	raw, keyIndex, err := readResourceWithMeta(ctx, session.Adapter, upstreamURI)
	// 计费插入点 B:成功确认 / 失败退款(资源结果无 isError 概念,仅看 err;超时归 ChargeOnTimeout)
	if bill != nil {
		h.finalizeBilling(bill, billing.ShouldChargeCall(err, false))
	}
	if err != nil {
		return h.errorResponse(reqID, -32603, "Failed to read resource: "+err.Error()), session.ServiceID, session.ServiceName, bill, keyIndex
	}

	// 回写网关命名空间 URI,让 contents[].uri 与 resources/list 暴露给客户端的一致。
	if items := unmarshalEntryList(raw, "contents"); len(items) > 0 {
		for _, item := range items {
			if uri, _ := item["uri"].(string); uri != "" {
				item["uri"] = gatewayResourceURI(session.ServiceName, uri)
			}
		}
		if b, mErr := json.Marshal(map[string]interface{}{"contents": items}); mErr == nil {
			raw = b
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result:  json.RawMessage(raw),
	}, session.ServiceID, session.ServiceName, bill, keyIndex
}

func (h *GatewayHandler) handlePromptsGet(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return h.errorResponse(req.ID, -32602, "Invalid params: name is required")
	}

	// 计费幂等键(id+提示名+参数哈希):同提示不同参数视为新请求各自计费。
	argsRaw, _ := json.Marshal(params.Arguments)
	requestID := billingRequestID(req.ID, params.Name, argsRaw)
	start := time.Now()
	serviceName, promptName := bridge.ParseNamespacedName(params.Name)
	if serviceName == "" {
		resp := h.errorResponse(req.ID, -32602, "Invalid prompt name, expected '<service>__<prompt>'")
		h.recordConsumeLog(logCtx, 0, "", "prompts/get", params.Name, requestID, resp, nil, 0, start)
		return resp
	}

	resp, serviceID, resolvedName, bill, keyIndex := h.getUpstreamPrompt(ctx, req.ID, logCtx, serviceName, promptName, params.Arguments, requestID)
	h.recordConsumeLog(logCtx, serviceID, resolvedName, "prompts/get", params.Name, requestID, resp, bill, keyIndex, start)
	return resp
}

// getUpstreamPrompt 提示读取核心(API key 范围校验/分组禁用拒绝/市场条目价计费/真实
// 调用上游),供原生 prompts/get 与智能模式元工具 mcp.read 共用;日志由调用方各自记录。
func (h *GatewayHandler) getUpstreamPrompt(ctx context.Context, reqID interface{}, logCtx *LogContext, serviceName, promptName string, arguments map[string]string, requestID string) (*JSONRPCResponse, int64, string, *billingOutcome, int) {
	if !h.isServiceInApiKeyScope(serviceName, logCtx) {
		return h.errorResponse(reqID, -32602, fmt.Sprintf("service '%s' is not accessible with this API key", serviceName)), 0, serviceName, nil, 0
	}

	session, err := h.connectServiceByName(ctx, serviceName, logCtx.UserID)
	if err != nil {
		return h.errorResponse(reqID, -32602, err.Error()), 0, serviceName, nil, 0
	}

	if h.itemDisabledInOwningGroup(logCtx, session.ServiceID, itemKindPrompt, promptName) {
		return h.errorResponse(reqID, -32602, "prompt is disabled in its group"), session.ServiceID, session.ServiceName, nil, 0
	}

	// 计费插入点 A:市场来源按条目价预扣(条目键=拆命名空间后的上游提示名)。
	var bill *billingOutcome
	if session.Source == "marketplace" {
		bill = &billingOutcome{}
		if !h.preConsumeBilling(ctx, logCtx, session, priceKindPrompt, promptName, requestID, bill) {
			return h.errorResponse(reqID, -32603, bill.BlockMsg), session.ServiceID, session.ServiceName, bill, 0
		}
	}

	raw, keyIndex, err := getPromptWithMeta(ctx, session.Adapter, promptName, arguments)
	// 计费插入点 B:成功确认 / 失败退款(提示结果无 isError 概念,仅看 err;超时归 ChargeOnTimeout)
	if bill != nil {
		h.finalizeBilling(bill, billing.ShouldChargeCall(err, false))
	}
	if err != nil {
		return h.errorResponse(reqID, -32603, "Failed to get prompt: "+err.Error()), session.ServiceID, session.ServiceName, bill, keyIndex
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result:  json.RawMessage(raw),
	}, session.ServiceID, session.ServiceName, bill, keyIndex
}

// readResourceWithMeta/getPromptWithMeta 经可选的 ResourceMetaCaller/PromptMetaCaller
// 读资源/取提示,带回本次实际使用的秘钥序号(非多秘钥 adapter 恒 0),供日志
// key_index 归因;与 tools/call 的 callTool(gateway_handler.go)同一模式。
func readResourceWithMeta(ctx context.Context, adapter transport.TransportAdapter, uri string) (json.RawMessage, int, error) {
	if mc, ok := adapter.(transport.ResourceMetaCaller); ok {
		raw, meta, err := mc.ReadResourceWithMeta(ctx, uri)
		return raw, meta.KeyIndex, err
	}
	raw, err := adapter.ReadResource(ctx, uri)
	return raw, 0, err
}

func getPromptWithMeta(ctx context.Context, adapter transport.TransportAdapter, name string, arguments map[string]string) (json.RawMessage, int, error) {
	if mc, ok := adapter.(transport.PromptMetaCaller); ok {
		raw, meta, err := mc.GetPromptWithMeta(ctx, name, arguments)
		return raw, meta.KeyIndex, err
	}
	raw, err := adapter.GetPrompt(ctx, name, arguments)
	return raw, 0, err
}

// itemDisabledInOwningGroup 读侧强制:按该 API key 范围内首个包含此服务的分组,
// 检查条目是否被勾选禁用(与聚合去重取首分组的过滤口径一致)。多分组场景下仅首个分组生效。
func (h *GatewayHandler) itemDisabledInOwningGroup(logCtx *LogContext, serviceID int64, kind, key string) bool {
	groupID, _ := h.resolveGroupForService(serviceID, logCtx)
	if groupID == 0 {
		return false
	}
	return loadItemFilter([]int64{groupID}, kind).disabled(groupID, serviceID, kind, key)
}

// recordConsumeLog 把 resources/read、prompts/get 这类真实触达上游的调用记入
// mcp_call_logs(与 tools/call 同一审计面)。target 为提示名或资源 URI。
// requestID 为计费幂等键(写入 request_id 列——HasChargedRequest 防重扣依赖该列);
// bill 为计费结算结果(非市场来源为 nil,billing_status 落默认 skipped);
// keyIndex=多秘钥服务本次实际使用的池内序号(0=单秘钥/未调上游)。
func (h *GatewayHandler) recordConsumeLog(logCtx *LogContext, serviceID int64, serviceName, method, target, requestID string, resp *JSONRPCResponse, bill *billingOutcome, keyIndex int, start time.Time) {
	groupID, groupName := int64(0), ""
	if serviceID != 0 {
		groupID, groupName = h.resolveGroupForService(serviceID, logCtx)
	}

	status := "success"
	var errMsg string
	if resp.Error != nil {
		status = "error"
		errMsg = resp.Error.Message
	}

	payload, _ := json.Marshal(map[string]string{"target": target})
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
		ToolName:       truncate(target, 255), // 资源=网关 URI / 提示=命名空间名,日志主列与 tool_name 过滤都依赖它
		Method:         method,
		KeyIndex:       keyIndex,
		RequestID:      truncate(requestID, 64), // 计费幂等键(同 tools/call 口径),target 仍在 ToolName/payload
		RequestPayload: truncate(string(payload), 65535),
		ResponseStatus: status,
		DurationMs:     int(time.Since(start).Milliseconds()),
		ErrorMessage:   truncate(errMsg, 65535),
		ClientIP:       logCtx.ClientIP,
		UserAgent:      truncate(logCtx.UserAgent, 512),
	}
	// 计费结算结果:市场来源且条目有价时由 readUpstreamResource/getUpstreamPrompt 结算
	// 后传入(含市场项归属);须在下方服务行回填之前,回填以 nil 判定是否需要补。
	applyBillingToLog(callLog, bill)
	// 市场来源服务的 item 归属回填:tools/call 由计费路径(applyBillingToLog)写入,
	// 无条目价(免费)/非市场来源的 resources/prompts 走这里补上——市场条目健康按
	// 日志 marketplace_item_id 聚合,漏写则这几类真实上游调用不计入条目健康。
	if callLog.MarketplaceItemID == nil && serviceID != 0 {
		callLog.MarketplaceItemID = model.GetServiceMarketplaceItemID(serviceID)
	}
	go h.recordLog(callLog)
}

// handleMetaRead 智能模式元工具 mcp.read:经 tools/call 读资源或取提示。复用原生
// resources/read、prompts/get 的上游核心(范围校验/禁用拒绝/条目价计费/命名空间回写),
// 结果转换为 tools/call 的 content 形态。日志由 handleToolsCall 统一记录(method=
// mcp.read,tool_name=target,与原生路径同列、可用工具名过滤);计费口径与原生一致
// (市场服务且条目有价时扣费,幂等键复用外层 requestID)。非市场来源 Billing 为 nil。
func (h *GatewayHandler) handleMetaRead(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage, requestID string) *executeResult {
	var params struct {
		Type      string            `json:"type"`
		Target    string            `json:"target"`
		Arguments map[string]string `json:"arguments"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Target == "" || (params.Type != "resource" && params.Type != "prompt") {
		return &executeResult{Resp: h.errorResponse(reqID, -32602, "Invalid params: type must be 'resource' or 'prompt', and target is required")}
	}

	result := &executeResult{ToolName: truncate(params.Target, 255)}
	if params.Type == "resource" {
		serviceName, upstreamURI, ok := parseGatewayResourceURI(params.Target)
		if !ok {
			result.Resp = h.errorResponse(reqID, -32602, "Invalid resource target, expected "+gatewayURIScheme+"://<service>/<upstream-uri>")
			return result
		}
		resp, serviceID, resolvedName, bill, keyIndex := h.readUpstreamResource(ctx, reqID, logCtx, serviceName, upstreamURI, requestID)
		result.Resp = resp
		result.Billing = bill
		result.KeyIndex = keyIndex
		if serviceID != 0 {
			result.ServiceID = serviceID
			result.ServiceName = resolvedName
			if converted, ok := resourceResultToContent(resp); ok {
				result.Resp = converted
			}
		}
		return result
	}

	serviceName, promptName := bridge.ParseNamespacedName(params.Target)
	if serviceName == "" {
		result.Resp = h.errorResponse(reqID, -32602, "Invalid prompt target, expected '<service>__<prompt>'")
		return result
	}
	resp, serviceID, resolvedName, bill, keyIndex := h.getUpstreamPrompt(ctx, reqID, logCtx, serviceName, promptName, params.Arguments, requestID)
	result.Resp = resp
	result.Billing = bill
	result.KeyIndex = keyIndex
	if serviceID != 0 {
		result.ServiceID = serviceID
		result.ServiceName = resolvedName
		if converted, ok := promptResultToContent(resp); ok {
			result.Resp = converted
		}
	}
	return result
}

// resourceResultToContent 把 resources/read 的 {contents:[...]} 结果转成 tools/call
// 的 content 形态(mcp.read 的返回)。text 条目直传;图片 blob 转 image 内容项;其余
// blob 无法被模型消费,以占位说明返回(保留 URI 便于换途径取原始数据)。失败返回 false,
// 调用方保留原始响应。
func resourceResultToContent(resp *JSONRPCResponse) (*JSONRPCResponse, bool) {
	if resp.Error != nil || resp.Result == nil {
		return nil, false
	}
	raw, err := json.Marshal(resp.Result) // Result 持 json.RawMessage,原样序列化
	if err != nil {
		return nil, false
	}
	var res struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Blob     string `json:"blob"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || len(res.Contents) == 0 {
		return nil, false
	}

	content := make([]map[string]interface{}, 0, len(res.Contents))
	for _, c := range res.Contents {
		switch {
		case c.Text != "":
			content = append(content, map[string]interface{}{"type": "text", "text": c.Text})
		case c.Blob != "" && strings.HasPrefix(c.MimeType, "image/"):
			content = append(content, map[string]interface{}{"type": "image", "data": c.Blob, "mimeType": c.MimeType})
		case c.Blob != "":
			content = append(content, map[string]interface{}{"type": "text", "text": "[base64 " + c.MimeType + " resource: " + c.URI + "]"})
		default:
			content = append(content, map[string]interface{}{"type": "text", "text": "[empty resource: " + c.URI + "]"})
		}
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      resp.ID,
		Result:  map[string]interface{}{"content": content},
	}, true
}

// promptResultToContent 把 prompts/get 的 {messages:[...]} 结果转成 tools/call 的
// content 形态,每条 text 消息带角色前缀;image/audio 内容保真嵌入。失败返回 false,
// 调用方保留原始响应。
func promptResultToContent(resp *JSONRPCResponse) (*JSONRPCResponse, bool) {
	if resp.Error != nil || resp.Result == nil {
		return nil, false
	}
	raw, err := json.Marshal(resp.Result) // Result 持 json.RawMessage,原样序列化
	if err != nil {
		return nil, false
	}
	var res struct {
		Description string `json:"description"`
		Messages    []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, false
	}

	var content []map[string]interface{}
	if res.Description != "" {
		content = append(content, map[string]interface{}{"type": "text", "text": "Description: " + res.Description})
	}
	for _, m := range res.Messages {
		var mc struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		}
		if err := json.Unmarshal(m.Content, &mc); err != nil {
			continue
		}
		switch mc.Type {
		case "text":
			content = append(content, map[string]interface{}{"type": "text", "text": "[" + m.Role + "]\n" + mc.Text})
		case "image":
			content = append(content, map[string]interface{}{"type": "image", "data": mc.Data, "mimeType": mc.MimeType})
		case "audio":
			content = append(content, map[string]interface{}{"type": "audio", "data": mc.Data, "mimeType": mc.MimeType})
		default:
			if b, err := json.Marshal(m.Content); err == nil {
				content = append(content, map[string]interface{}{"type": "text", "text": "[" + m.Role + "]\n" + string(b)})
			}
		}
	}
	if content == nil {
		return nil, false
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      resp.ID,
		Result:  map[string]interface{}{"content": content},
	}, true
}
