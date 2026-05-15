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
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/model"
)

type XiaoZhiClient struct {
	endpointID int64
	wssURL     string
	apiKeyID   int64
	exposeMode string

	conn    *websocket.Conn
	handler *handler.GatewayHandler

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
		apiKeyID:   apiKeyID,
		exposeMode: ep.ExposeMode,
		handler:    handler.NewGatewayHandler(pool, router, nil),
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

	// Auto-respond to pings from the server
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.done = make(chan struct{})
	c.closeOnce = sync.Once{}
	c.mu.Unlock()

	c.updateStatus(common.ConnConnected, "")

	ctx, c.cancel = context.WithCancel(context.Background())
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

func (c *XiaoZhiClient) buildLogCtx() *handler.LogContext {
	logCtx := &handler.LogContext{
		ApiKeyID:   c.apiKeyID,
		ExposeMode: c.exposeMode,
	}
	var apiKey model.ApiKey
	if err := model.DB.First(&apiKey, c.apiKeyID).Error; err == nil {
		logCtx.UserID = apiKey.UserID
		logCtx.ApiKeyName = apiKey.Name
		var user model.User
		if err := model.DB.Select("username").First(&user, apiKey.UserID).Error; err == nil {
			logCtx.Username = user.Username
		}
	}
	return logCtx
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

	logCtx := c.buildLogCtx()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("[xiaozhi:%d] read error: %v", c.endpointID, err)
			return
		}

		log.Printf("[xiaozhi:%d] recv: %s", c.endpointID, string(message))

		var req handler.JSONRPCRequest
		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		// Handle initialize with dynamic protocol version
		if req.Method == "initialize" {
			resp := c.handleInitialize(&req)
			c.sendResponse(resp)
			continue
		}

		// MCP ping: respond with empty result
		if req.Method == "ping" {
			c.sendResponse(&handler.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]interface{}{},
			})
			continue
		}

		// Delegate everything else to GatewayHandler (same logic as /mcp)
		resp := c.handler.HandleRequest(ctx, &req, logCtx)
		if resp == nil {
			continue
		}

		c.sendResponse(resp)
	}
}

func (c *XiaoZhiClient) handleInitialize(req *handler.JSONRPCRequest) *handler.JSONRPCResponse {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(req.Params, &params)
	if params.ProtocolVersion == "" {
		params.ProtocolVersion = "2024-11-05"
	}

	return &handler.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": params.ProtocolVersion,
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

func (c *XiaoZhiClient) sendResponse(resp *handler.JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	log.Printf("[xiaozhi:%d] send: %s", c.endpointID, string(data))

	c.mu.Lock()
	if c.conn != nil {
		c.conn.WriteMessage(websocket.TextMessage, data)
	}
	c.mu.Unlock()
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
