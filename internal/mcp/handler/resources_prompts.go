package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/bridge"
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
//   - resources/read、prompts/get 会真实调用上游,计入调用日志(计费口径与 tools/call
//     不同,市场服务暂不计费,日志 billing_status 落默认 skipped)。
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
		group, err := model.GetGroupBySlug(logCtx.GroupSlug)
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

func (h *GatewayHandler) handleResourcesList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
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

	start := time.Now()
	resp, serviceID, resolvedName := h.readUpstreamResource(ctx, req.ID, logCtx, serviceName, upstreamURI)
	h.recordConsumeLog(logCtx, serviceID, resolvedName, "resources/read", params.URI, resp, start)
	return resp
}

func (h *GatewayHandler) readUpstreamResource(ctx context.Context, reqID interface{}, logCtx *LogContext, serviceName, upstreamURI string) (*JSONRPCResponse, int64, string) {
	if !h.isServiceInApiKeyScope(serviceName, logCtx) {
		resp := h.errorResponse(reqID, -32602, fmt.Sprintf("service '%s' is not accessible with this API key", serviceName))
		return resp, 0, serviceName
	}

	session, err := h.connectServiceByName(ctx, serviceName, logCtx.UserID)
	if err != nil {
		return h.errorResponse(reqID, -32602, err.Error()), 0, serviceName
	}

	if h.itemDisabledInOwningGroup(logCtx, session.ServiceID, itemKindResource, upstreamURI) {
		return h.errorResponse(reqID, -32602, "resource is disabled in its group"), session.ServiceID, session.ServiceName
	}

	raw, err := session.Adapter.ReadResource(ctx, upstreamURI)
	if err != nil {
		return h.errorResponse(reqID, -32603, "Failed to read resource: "+err.Error()), session.ServiceID, session.ServiceName
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
	}, session.ServiceID, session.ServiceName
}

func (h *GatewayHandler) handlePromptsGet(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return h.errorResponse(req.ID, -32602, "Invalid params: name is required")
	}

	start := time.Now()
	serviceName, promptName := bridge.ParseNamespacedName(params.Name)
	if serviceName == "" {
		resp := h.errorResponse(req.ID, -32602, "Invalid prompt name, expected '<service>__<prompt>'")
		h.recordConsumeLog(logCtx, 0, "", "prompts/get", params.Name, resp, start)
		return resp
	}

	if !h.isServiceInApiKeyScope(serviceName, logCtx) {
		resp := h.errorResponse(req.ID, -32602, fmt.Sprintf("service '%s' is not accessible with this API key", serviceName))
		h.recordConsumeLog(logCtx, 0, serviceName, "prompts/get", params.Name, resp, start)
		return resp
	}

	session, err := h.connectServiceByName(ctx, serviceName, logCtx.UserID)
	if err != nil {
		resp := h.errorResponse(req.ID, -32602, err.Error())
		h.recordConsumeLog(logCtx, 0, serviceName, "prompts/get", params.Name, resp, start)
		return resp
	}

	if h.itemDisabledInOwningGroup(logCtx, session.ServiceID, itemKindPrompt, promptName) {
		resp := h.errorResponse(req.ID, -32602, "prompt is disabled in its group")
		h.recordConsumeLog(logCtx, session.ServiceID, session.ServiceName, "prompts/get", params.Name, resp, start)
		return resp
	}

	raw, err := session.Adapter.GetPrompt(ctx, promptName, params.Arguments)
	var resp *JSONRPCResponse
	if err != nil {
		resp = h.errorResponse(req.ID, -32603, "Failed to get prompt: "+err.Error())
	} else {
		resp = &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(raw),
		}
	}
	h.recordConsumeLog(logCtx, session.ServiceID, session.ServiceName, "prompts/get", params.Name, resp, start)
	return resp
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
func (h *GatewayHandler) recordConsumeLog(logCtx *LogContext, serviceID int64, serviceName, method, target string, resp *JSONRPCResponse, start time.Time) {
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
		RequestID:      truncate(target, 64),
		RequestPayload: truncate(string(payload), 65535),
		ResponseStatus: status,
		DurationMs:     int(time.Since(start).Milliseconds()),
		ErrorMessage:   truncate(errMsg, 65535),
		ClientIP:       logCtx.ClientIP,
		UserAgent:      truncate(logCtx.UserAgent, 512),
	}
	go h.recordLog(callLog)
}
