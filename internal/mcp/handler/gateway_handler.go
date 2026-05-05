package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/smart"
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
}

type GatewayHandler struct {
	pool         *bridge.SessionPool
	toolRouter   *bridge.ToolRouter
	searchEngine *smart.SearchEngine
}

func NewGatewayHandler(pool *bridge.SessionPool, toolRouter *bridge.ToolRouter) *GatewayHandler {
	return &GatewayHandler{
		pool:         pool,
		toolRouter:   toolRouter,
		searchEngine: smart.NewSearchEngine(),
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
	groupSlug := logCtx.GroupSlug
	if groupSlug == "" {
		return h.smartToolsResponse(req.ID)
	}

	group, err := model.GetGroupBySlug(groupSlug)
	if err != nil {
		return h.errorResponse(req.ID, -32602, "Group not found: "+groupSlug)
	}

	if !h.hasGroupAccess(logCtx.ApiKeyID, group.Name) {
		return h.errorResponse(req.ID, -32602, "API key does not have access to group: "+groupSlug)
	}

	switch group.ExposeMode {
	case "smart":
		return h.smartToolsResponse(req.ID)
	case "direct":
		tools, err := h.getDirectTools(ctx, group.ID)
		if err != nil {
			return h.errorResponse(req.ID, -32603, "Failed to get tools")
		}
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
		// Verify group access for direct tool calls
		if logCtx.GroupSlug != "" {
			group, err := model.GetGroupBySlug(logCtx.GroupSlug)
			if err != nil {
				resp = h.errorResponse(req.ID, -32602, "Group not found: "+logCtx.GroupSlug)
			} else if !h.hasGroupAccess(logCtx.ApiKeyID, group.Name) {
				resp = h.errorResponse(req.ID, -32602, "API key does not have access to group: "+logCtx.GroupSlug)
			}
		}

		if resp == nil {
			session, toolName, err := h.toolRouter.Route(params.Name)
			if err != nil {
				resp = h.errorResponse(req.ID, -32602, err.Error())
			} else {
				serviceID = session.ServiceID
				serviceName = session.ServiceName

				callParams := map[string]interface{}{
					"name":      toolName,
					"arguments": params.Arguments,
				}

				result, err := session.Adapter.Call(ctx, "tools/call", callParams)
				if err != nil {
					resp = h.errorResponse(req.ID, -32603, "Tool execution failed: "+err.Error())
				} else {
					resp = &JSONRPCResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Result:  json.RawMessage(result),
					}
				}
			}
		}
	}

	duration := time.Since(start)
	status := "success"
	var errMsg string
	if resp.Error != nil {
		status = "error"
		errMsg = resp.Error.Message
	}

	// Async log
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

func (h *GatewayHandler) handleSearch(ctx context.Context, reqID interface{}, logCtx *LogContext, args json.RawMessage) *JSONRPCResponse {
	var params struct {
		Query string `json:"query"`
		Scope string `json:"scope"`
		Group string `json:"group"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Scope == "" {
		params.Scope = "mcp"
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

	session, toolName, err := h.toolRouter.Route(params.ToolID)
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

func (h *GatewayHandler) hasGroupAccess(apiKeyID int64, groupName string) bool {
	var apiKey model.ApiKey
	if err := model.DB.First(&apiKey, apiKeyID).Error; err != nil {
		return false
	}
	var permissions struct {
		Groups []string `json:"groups"`
	}
	_ = json.Unmarshal([]byte(apiKey.Permissions), &permissions)

	for _, g := range permissions.Groups {
		if g == "*" || g == groupName {
			return true
		}
	}
	return false
}

func (h *GatewayHandler) smartToolsResponse(id interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  map[string]interface{}{"tools": smart.MetaTools},
	}
}

func (h *GatewayHandler) getDirectTools(ctx context.Context, groupID int64) ([]map[string]interface{}, error) {
	groupServices, err := model.GetEnabledGroupServices(groupID)
	if err != nil {
		return nil, err
	}

	var allTools []map[string]interface{}
	for _, gs := range groupServices {
		svc, err := model.GetServiceByIDWithoutUser(gs.ServiceID)
		if err != nil {
			continue
		}

		var tools []map[string]interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

		for _, t := range tools {
			name, _ := t["name"].(string)
			t["name"] = svc.Name + "__" + name
			allTools = append(allTools, t)
		}
	}
	return allTools, nil
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
