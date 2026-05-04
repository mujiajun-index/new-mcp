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

func (h *GatewayHandler) HandleRequest(ctx context.Context, req *JSONRPCRequest, apiKeyID int64, groupSlug string) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return h.handleInitialize(req)
	case "notifications/initialized":
		return nil // no response for notifications
	case "tools/list":
		return h.handleToolsList(ctx, apiKeyID, groupSlug)
	case "tools/call":
		return h.handleToolsCall(ctx, req, apiKeyID, groupSlug)
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

func (h *GatewayHandler) handleToolsList(ctx context.Context, apiKeyID int64, groupSlug string) *JSONRPCResponse {
	if groupSlug == "" {
		// Main endpoint: fixed Smart mode
		return h.smartToolsResponse(nil)
	}

	// Group endpoint: check expose_mode
	group, err := model.GetGroupBySlug(groupSlug)
	if err != nil {
		return h.errorResponse(nil, -32602, "Group not found: "+groupSlug)
	}

	switch group.ExposeMode {
	case "smart":
		return h.smartToolsResponse(nil)
	case "direct":
		tools, err := h.getDirectTools(ctx, group.ID)
		if err != nil {
			return h.errorResponse(nil, -32603, "Failed to get tools")
		}
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  map[string]interface{}{"tools": tools},
		}
	default:
		return h.smartToolsResponse(nil)
	}
}

func (h *GatewayHandler) handleToolsCall(ctx context.Context, req *JSONRPCRequest, apiKeyID int64, groupSlug string) *JSONRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return h.errorResponse(req.ID, -32602, "Invalid params")
	}

	// Smart mode meta-tools
	switch params.Name {
	case "mcp.search":
		return h.handleSearch(ctx, req.ID, apiKeyID, params.Arguments)
	case "mcp.describe":
		return h.handleDescribe(ctx, req.ID, apiKeyID, params.Arguments)
	case "mcp.execute":
		return h.handleExecute(ctx, req.ID, apiKeyID, params.Arguments)
	}

	// Direct mode: route to upstream service
	session, toolName, err := h.toolRouter.Route(params.Name)
	if err != nil {
		return h.errorResponse(req.ID, -32602, err.Error())
	}

	callParams := map[string]interface{}{
		"name":      toolName,
		"arguments": params.Arguments,
	}

	result, err := session.Adapter.Call(ctx, "tools/call", callParams)
	if err != nil {
		return h.errorResponse(req.ID, -32603, "Tool execution failed: "+err.Error())
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage(result),
	}
}

func (h *GatewayHandler) handleSearch(ctx context.Context, reqID interface{}, apiKeyID int64, args json.RawMessage) *JSONRPCResponse {
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

	results, err := h.searchEngine.Search(ctx, apiKeyID, params.Query, smart.SearchOptions{
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

func (h *GatewayHandler) handleDescribe(ctx context.Context, reqID interface{}, apiKeyID int64, args json.RawMessage) *JSONRPCResponse {
	var params struct {
		Targets []string `json:"targets"`
	}
	_ = json.Unmarshal(args, &params)

	results, err := h.searchEngine.Describe(params.Targets, apiKeyID)
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

func (h *GatewayHandler) handleExecute(ctx context.Context, reqID interface{}, apiKeyID int64, args json.RawMessage) *JSONRPCResponse {
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
		svc, err := model.GetServiceByID(0, gs.ServiceID)
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
