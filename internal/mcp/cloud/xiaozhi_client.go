package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/smart"
	"github.com/mujkjk/newmcp/model"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type XiaoZhiClient struct {
	endpointID int64
	wssURL     string
	groupID    int64
	apiKeyID   int64

	conn        *websocket.Conn
	pool        *bridge.SessionPool
	toolRouter  *bridge.ToolRouter
	search      *smart.SearchEngine

	mu        sync.Mutex
	connected bool
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

func NewXiaoZhiClient(ep *model.CloudEndpoint, pool *bridge.SessionPool, router *bridge.ToolRouter) *XiaoZhiClient {
	apiKeyID := int64(0)
	if ep.ApiKeyID != nil {
		apiKeyID = *ep.ApiKeyID
	}
	return &XiaoZhiClient{
		endpointID: ep.ID,
		wssURL:     ep.WssURL,
		groupID:    derefInt64(ep.GroupID),
		apiKeyID:   apiKeyID,
		pool:       pool,
		toolRouter: router,
		search:     smart.NewSearchEngine(),
		done:       make(chan struct{}),
	}
}

func (c *XiaoZhiClient) Connect(ctx context.Context) error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.DialContext(ctx, c.wssURL, nil)
	if err != nil {
		return fmt.Errorf("dial xiaozhi: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.done = make(chan struct{})
	c.closeOnce = sync.Once{}
	c.mu.Unlock()

	// Update DB status
	c.updateStatus(common.ConnConnected, "")

	ctx, c.cancel = context.WithCancel(ctx)
	go c.messageLoop(ctx)
	go c.pingLoop(ctx)

	return nil
}

func (c *XiaoZhiClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
	}
	c.updateStatus(common.ConnDisconnected, "")
}

func (c *XiaoZhiClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

func (c *XiaoZhiClient) Done() <-chan struct{} {
	return c.done
}

func (c *XiaoZhiClient) messageLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[xiaozhi:%d] messageLoop panic: %v", c.endpointID, r)
		}
		c.mu.Lock()
		c.connected = false
		if c.conn != nil {
			c.conn.Close()
		}
		c.mu.Unlock()
		c.updateStatus(common.ConnDisconnected, "connection lost")
		c.closeOnce.Do(func() { close(c.done) })
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[xiaozhi:%d] read error: %v", c.endpointID, err)
			}
			return
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		resp := c.handleRequest(ctx, &req)

		// Notifications have no response
		if resp == nil {
			continue
		}

		data, err := json.Marshal(resp)
		if err != nil {
			continue
		}

		c.mu.Lock()
		if c.conn != nil {
			c.conn.WriteMessage(websocket.TextMessage, data)
		}
		c.mu.Unlock()
	}
}

func (c *XiaoZhiClient) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.conn != nil {
				c.conn.WriteMessage(websocket.PingMessage, nil)
			}
			c.mu.Unlock()
		}
	}
}

func (c *XiaoZhiClient) handleRequest(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return c.handleInitialize(req)
	case "notifications/initialized":
		return nil
	case "tools/list":
		return c.handleToolsList(ctx, req)
	case "tools/call":
		return c.handleToolsCall(ctx, req)
	default:
		return c.errorResponse(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (c *XiaoZhiClient) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		ID: req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "newmcp-gateway",
				"version": "1.0.0",
			},
		},
	}
}

func (c *XiaoZhiClient) handleToolsList(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	if c.groupID == 0 {
		return c.errorResponse(req.ID, -32602, "no group bound to this connection")
	}

	group, err := model.GetGroupByID(0, c.groupID)
	if err != nil {
		return c.errorResponse(req.ID, -32602, "group not found")
	}

	switch group.ExposeMode {
	case common.ExposeModeSmart:
		return c.smartToolsResponse(req.ID)
	case common.ExposeModeDirect:
		tools, err := c.getDirectTools(ctx)
		if err != nil {
			return c.errorResponse(req.ID, -32603, "failed to get tools: "+err.Error())
		}
		return &jsonRPCResponse{ID: req.ID, Result: map[string]interface{}{"tools": tools}}
	default:
		return c.smartToolsResponse(req.ID)
	}
}

func (c *XiaoZhiClient) handleToolsCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return c.errorResponse(req.ID, -32602, "invalid params")
	}

	// Smart mode meta-tools
	switch params.Name {
	case "mcp.search":
		return c.handleSearch(ctx, req.ID, params.Arguments)
	case "mcp.describe":
		return c.handleDescribe(ctx, req.ID, params.Arguments)
	case "mcp.execute":
		return c.handleExecute(ctx, req.ID, params.Arguments)
	}

	// Direct mode: route to upstream service via tool router
	session, toolName, err := c.toolRouter.Route(params.Name)
	if err != nil {
		return c.errorResponse(req.ID, -32602, err.Error())
	}

	callParams := map[string]interface{}{
		"name":      toolName,
		"arguments": params.Arguments,
	}
	result, err := session.Adapter.Call(ctx, "tools/call", callParams)
	if err != nil {
		return c.errorResponse(req.ID, -32603, "tool call failed: "+err.Error())
	}

	var parsed interface{}
	json.Unmarshal(result, &parsed)
	return &jsonRPCResponse{ID: req.ID, Result: parsed}
}

func (c *XiaoZhiClient) handleSearch(ctx context.Context, reqID interface{}, args json.RawMessage) *jsonRPCResponse {
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

	results, err := c.search.Search(ctx, c.apiKeyID, params.Query, smart.SearchOptions{
		Scope: params.Scope,
		Group: params.Group,
		Limit: params.Limit,
	})
	if err != nil {
		return c.errorResponse(reqID, -32603, "search failed: "+err.Error())
	}

	var lines []string
	for _, r := range results {
		if r.Doc.Type == "mcp" {
			lines = append(lines, fmt.Sprintf("- **%s** (服务, %d 工具) %s",
				r.Doc.Name, r.Doc.ToolCount, r.Doc.Description))
		} else {
			lines = append(lines, fmt.Sprintf("- **%s.%s** (工具) %s",
				r.Doc.ServiceName, r.Doc.Name, r.Doc.Description))
		}
	}

	text := fmt.Sprintf("找到 %d 个结果:\n", len(lines))
	for _, l := range lines {
		text += l + "\n"
	}

	return &jsonRPCResponse{
		ID: reqID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
		},
	}
}

func (c *XiaoZhiClient) handleDescribe(ctx context.Context, reqID interface{}, args json.RawMessage) *jsonRPCResponse {
	var params struct {
		Targets []string `json:"targets"`
	}
	_ = json.Unmarshal(args, &params)

	results, err := c.search.Describe(params.Targets, c.apiKeyID)
	if err != nil {
		return c.errorResponse(reqID, -32603, "describe failed: "+err.Error())
	}

	text := ""
	for _, r := range results {
		b, _ := json.MarshalIndent(r, "", "  ")
		text += string(b) + "\n\n"
	}

	return &jsonRPCResponse{
		ID: reqID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
		},
	}
}

func (c *XiaoZhiClient) handleExecute(ctx context.Context, reqID interface{}, args json.RawMessage) *jsonRPCResponse {
	var params struct {
		ToolID    string          `json:"tool_id"`
		Arguments json.RawMessage `json:"arguments"`
		TimeoutMs int             `json:"timeout_ms"`
	}
	_ = json.Unmarshal(args, &params)
	if params.TimeoutMs <= 0 {
		params.TimeoutMs = 30000
	}

	session, toolName, err := c.toolRouter.Route(params.ToolID)
	if err != nil {
		return c.errorResponse(reqID, -32602, err.Error())
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(params.TimeoutMs)*time.Millisecond)
	defer cancel()

	callParams := map[string]interface{}{
		"name":      toolName,
		"arguments": params.Arguments,
	}
	result, err := session.Adapter.Call(timeoutCtx, "tools/call", callParams)
	if err != nil {
		return c.errorResponse(reqID, -32603, "execution failed: "+err.Error())
	}

	var parsed interface{}
	json.Unmarshal(result, &parsed)
	return &jsonRPCResponse{ID: reqID, Result: parsed}
}

func (c *XiaoZhiClient) smartToolsResponse(id interface{}) *jsonRPCResponse {
	return &jsonRPCResponse{
		ID:     id,
		Result: map[string]interface{}{"tools": smart.MetaTools},
	}
}

func (c *XiaoZhiClient) getDirectTools(ctx context.Context) ([]map[string]interface{}, error) {
	groupServices, err := model.GetEnabledGroupServices(c.groupID)
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

func (c *XiaoZhiClient) errorResponse(id interface{}, code int, message string) *jsonRPCResponse {
	return &jsonRPCResponse{
		ID: id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	}
}

func (c *XiaoZhiClient) updateStatus(status, errMsg string) {
	updates := map[string]interface{}{
		"connection_status": status,
	}
	if status == common.ConnConnected {
		now := time.Now()
		updates["last_connected_at"] = now
		updates["last_error"] = ""
	} else if errMsg != "" {
		updates["last_error"] = errMsg
	}
	model.DB.Model(&model.CloudEndpoint{}).Where("id = ?", c.endpointID).Updates(updates)
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
