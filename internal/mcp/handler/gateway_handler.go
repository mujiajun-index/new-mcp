package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

type GatewayHandler struct {
	pool            *bridge.SessionPool
	toolRouter      *bridge.ToolRouter
	searchEngine    *smart.SearchEngine
	virtualRegistry *virtual.VirtualToolRegistry
}

func NewGatewayHandler(pool *bridge.SessionPool, toolRouter *bridge.ToolRouter, vr *virtual.VirtualToolRegistry) *GatewayHandler {
	return &GatewayHandler{
		pool:            pool,
		toolRouter:      toolRouter,
		searchEngine:    smart.NewSearchEngine(),
		virtualRegistry: vr,
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
		resp = h.handleSearch(ctx, req.ID, logCtx, params.Arguments)
	case "mcp.describe":
		resp = h.handleDescribe(ctx, req.ID, logCtx, params.Arguments)
	case "mcp.execute":
		resp = h.handleExecute(ctx, req.ID, logCtx, params.Arguments)
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
			resp = h.routeAndCall(ctx, req.ID, logCtx, params.Name, params.Arguments, &serviceID, &serviceName)
		}
	}

	duration := time.Since(start)
	status := "success"
	var errMsg string
	if resp.Error != nil {
		status = "error"
		errMsg = resp.Error.Message
	}

	go h.recordLog(&model.McpCallLog{
		UserID:         logCtx.UserID,
		Username:       logCtx.Username,
		ApiKeyID:       logCtx.ApiKeyID,
		ApiKeyName:     logCtx.ApiKeyName,
		GroupID:        groupID,
		GroupName:      groupName,
		ServiceID:      serviceID,
		ServiceName:    serviceName,
		ToolName:       params.Name,
		Method:         "tools/call",
		RequestPayload: truncate(string(params.Arguments), 65535),
		ResponseStatus: status,
		DurationMs:     int(duration.Milliseconds()),
		ErrorMessage:   truncate(errMsg, 65535),
		ClientIP:       logCtx.ClientIP,
		UserAgent:      truncate(logCtx.UserAgent, 512),
	})

	return resp
}

// routeAndCall handles virtual tool check, session routing, and tool execution.
func (h *GatewayHandler) routeAndCall(ctx context.Context, reqID interface{}, logCtx *LogContext, toolName string, args json.RawMessage, svcID *int64, svcName *string) *JSONRPCResponse {
	parsedSvc, parsedTool := bridge.ParseNamespacedName(toolName)

	// Check virtual tools first
	if h.virtualRegistry != nil && parsedSvc != "" {
		var virtualSvc model.McpService
		if model.DB.Where("name = ? AND transport_type = ? AND user_id = ?", parsedSvc, "virtual", logCtx.UserID).First(&virtualSvc).Error == nil {
			vConfig := map[string]interface{}{}
			_ = json.Unmarshal([]byte(virtualSvc.Config), &vConfig)
			vResult, vErr := h.virtualRegistry.Handle(ctx, virtualSvc.ID, vConfig, parsedTool, args)
			*svcID = virtualSvc.ID
			*svcName = virtualSvc.Name
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

	callParams := map[string]interface{}{
		"name":      resolvedTool,
		"arguments": args,
	}

	result, err := session.Adapter.Call(ctx, "tools/call", callParams)
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

	var textResults []string
	for _, r := range results {
		if r.Doc.Type == "mcp" {
			textResults = append(textResults, fmt.Sprintf("- **%s** (服务, %d 工具) %s [%s]", r.Doc.Name, r.Doc.ToolCount, r.Doc.Description, r.Doc.GroupName))
		} else {
			textResults = append(textResults, fmt.Sprintf("- **%s.%s** (工具) %s [%s]", r.Doc.ServiceName, r.Doc.Name, r.Doc.Description, r.Doc.GroupName))
		}
	}

	resultText := fmt.Sprintf("找到 %d 个结果:\n", len(textResults))
	for _, t := range textResults {
		resultText += t + "\n"
	}

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
		Targets []string `json:"targets"`
	}
	_ = json.Unmarshal(args, &params)

	results, err := h.searchEngine.Describe(params.Targets, logCtx.ApiKeyID)
	if err != nil {
		return h.errorResponse(reqID, -32603, "Describe failed: "+err.Error())
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("%v", results)},
			},
		},
	}
}

func (h *GatewayHandler) handleExecute(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage) *JSONRPCResponse {
	var params struct {
		ToolID    string          `json:"tool_id"`
		Arguments json.RawMessage `json:"arguments"`
		TimeoutMs int             `json:"timeout_ms"`
	}
	_ = json.Unmarshal(args, &params)

	if params.TimeoutMs <= 0 {
		params.TimeoutMs = 30000
	}

	session, toolName, err := h.routeOrConnect(ctx, params.ToolID, logCtx.UserID)
	if err != nil {
		return h.errorResponse(reqID, -32602, err.Error())
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
	if err != nil {
		return h.errorResponse(reqID, -32603, "Execution failed: "+err.Error())
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result:  json.RawMessage(result),
	}
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
	session, toolName, err := h.toolRouter.Route(namespacedTool)
	if err == nil {
		return session, toolName, nil
	}

	svcName, parsedToolName := bridge.ParseNamespacedName(namespacedTool)
	if svcName != "" {
		var svc model.McpService
		if dbErr := model.DB.Where("name = ? AND user_id = ?", svcName, userID).First(&svc).Error; dbErr != nil {
			return nil, "", fmt.Errorf("service not found: %s", svcName)
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

func (h *GatewayHandler) errorResponse(id interface{}, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}

func (h *GatewayHandler) recordLog(log *model.McpCallLog) {
	_ = log.Insert()
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
