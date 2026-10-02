package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
)

var gatewayHandler *handler.GatewayHandler

func SetMCPRouter(engine *gin.Engine, h *handler.GatewayHandler) {
	gatewayHandler = h

	for _, endpoint := range []struct {
		path, mode string
		group      bool
	}{
		{"/mcp", "direct", false}, {"/smart/mcp", "smart", false}, {"/mcp/group/:slug", "", true},
	} {
		for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
			engine.Handle(method, endpoint.path, middleware.APIKeyAuth(), middleware.RateLimit(), serveMCPHTTP(h, endpoint.mode, endpoint.group))
		}
	}

	// WebSocket - Direct mode
	engine.GET("/mcp/ws", middleware.APIKeyAuth(), middleware.RateLimit(), serveMCPWebSocket(h, "direct", false))

	// WebSocket - Smart mode
	engine.GET("/smart/mcp/ws", middleware.APIKeyAuth(), middleware.RateLimit(), serveMCPWebSocket(h, "smart", false))

	// WebSocket - Group endpoint
	engine.GET("/mcp/ws/group/:slug", middleware.APIKeyAuth(), middleware.RateLimit(), serveMCPWebSocket(h, "", true))

	// Local MCP servers connect here; the gateway acts as their MCP client.
	engine.GET("/mcp/passive/", HandlePassiveWebSocket)
}

func buildLogContext(c *gin.Context, exposeMode string) *handler.LogContext {
	apiKeyID := c.GetInt64("api_key_id")
	userID := c.GetInt64("api_key_user_id")

	info, err := bridge.ResolveApiKeyInfo(apiKeyID)
	if err != nil {
		return &handler.LogContext{
			ApiKeyID:   apiKeyID,
			UserID:     userID,
			ExposeMode: exposeMode,
			ClientIP:   middleware.GetRequestIP(c),
			UserAgent:  c.Request.UserAgent(),
		}
	}

	return &handler.LogContext{
		ApiKeyID:   info.ApiKeyID,
		UserID:     info.UserID,
		Username:   info.Username,
		ApiKeyName: info.ApiKeyName,
		ExposeMode: exposeMode,
		ClientIP:   middleware.GetRequestIP(c),
		UserAgent:  c.Request.UserAgent(),
	}
}

const maxMCPBody = 16 << 20

func decodeMCPRequest(body []byte) (*handler.JSONRPCRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request handler.JSONRPCRequest
	if err := decoder.Decode(&request); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	return &request, nil
}

func allowedMCPOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Host == request.Host {
		hostname := u.Hostname()
		ip := net.ParseIP(hostname)
		if hostname == "localhost" || (ip != nil && ip.IsLoopback()) {
			return true
		}
	}
	for _, allowed := range strings.Split(model.GetOptionString("MCPAllowedOrigins"), ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	serverURL, err := url.Parse(model.GetOptionString("ServerAddress"))
	return err == nil && serverURL.Host != "" && origin == serverURL.Scheme+"://"+serverURL.Host
}

func serveMCPHTTP(h *handler.GatewayHandler, exposeMode string, group bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !allowedMCPOrigin(c.Request) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Origin is not allowed"})
			return
		}
		logCtx := buildLogContext(c, exposeMode)
		if group {
			logCtx.GroupSlug = c.Param("slug")
		}
		logCtx.SessionID = c.GetHeader("Mcp-Session-Id")
		if c.Request.Method != http.MethodPost {
			if c.GetHeader("MCP-Protocol-Version") >= "2026-07-28" || logCtx.SessionID == "" {
				c.Header("Allow", "POST")
				c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Use POST subscriptions/listen for event subscriptions"})
				return
			}
			if c.Request.Method == http.MethodDelete {
				if err := h.RemoveLegacySession(logCtx); err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
					return
				}
				c.Status(http.StatusNoContent)
				return
			}
			if !strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
				c.JSON(http.StatusNotAcceptable, gin.H{"error": "Accept must include text/event-stream"})
				return
			}
			if c.GetHeader("Last-Event-ID") != "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "SSE replay is unavailable; reconnect and subscribe again"})
				return
			}
			stream, err := h.OpenLegacyStream(c.Request.Context(), logCtx)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			serveSSE(c, h, nil, stream)
			return
		}
		contentType := c.GetHeader("Content-Type")
		mediaType, _, mediaErr := mime.ParseMediaType(contentType)
		if contentType != "" && (mediaErr != nil || mediaType != "application/json") {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Content-Type must be application/json"})
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxMCPBody))
		if err != nil {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "MCP request body is too large or unreadable"})
			return
		}
		request, err := decodeMCPRequest(body)
		if err != nil {
			c.JSON(http.StatusBadRequest, &handler.JSONRPCResponse{JSONRPC: "2.0", Error: &handler.RPCError{Code: -32700, Message: "Invalid JSON-RPC JSON"}})
			return
		}
		if contentType == "" && handler.IsModernRequest(request) {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Content-Type must be application/json"})
			return
		}
		if failure := handler.ValidateHTTPHeaders(request, c.Request.Header); failure != nil {
			c.JSON(http.StatusBadRequest, failure)
			return
		}
		if failure := handler.ValidateRequest(request); failure != nil {
			c.JSON(handler.HTTPStatus(request, failure), failure)
			return
		}
		if failure := h.ValidateToolHeaders(c.Request.Context(), request, c.Request.Header, logCtx); failure != nil {
			c.JSON(http.StatusBadRequest, failure)
			return
		}
		if handler.IsModernRequest(request) {
			logCtx.SessionID = ""
		} else if request.Method != "initialize" && logCtx.SessionID != "" {
			if err := h.ValidateLegacySession(logCtx); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
		}
		if request.Method == "subscriptions/listen" || request.Method == "events/stream" {
			if !strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
				c.JSON(http.StatusNotAcceptable, gin.H{"error": "Accept must include text/event-stream"})
				return
			}
			var stream *handler.NotificationStream
			var failure *handler.JSONRPCResponse
			if request.Method == "subscriptions/listen" {
				stream, failure = h.OpenSubscriptions(c.Request.Context(), request, logCtx)
			} else {
				stream, failure = h.OpenEventStream(c.Request.Context(), request, logCtx)
			}
			if failure != nil {
				c.JSON(handler.HTTPStatus(request, failure), failure)
				return
			}
			serveSSE(c, h, request, stream)
			return
		}
		response := h.HandleRequest(c.Request.Context(), request, logCtx)
		if request.Method == "initialize" && response != nil && response.Error == nil {
			id, err := h.CreateLegacySession(logCtx)
			if err != nil {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
				return
			}
			c.Header("Mcp-Session-Id", id)
		}
		if response == nil {
			c.Status(http.StatusAccepted)
			return
		}
		c.JSON(handler.HTTPStatus(request, response), response)
	}
}

func serveSSE(c *gin.Context, h *handler.GatewayHandler, request *handler.JSONRPCRequest, stream *handler.NotificationStream) {
	defer stream.Stop()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	write := func(value any) bool {
		raw, err := json.Marshal(value)
		if err != nil {
			return false
		}
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := c.Writer.Write(append(append([]byte("event: message\ndata: "), raw...), '\n', '\n')); err != nil {
			return false
		}
		c.Writer.Flush()
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
		return true
	}
	if request != nil && request.Method == "subscriptions/listen" {
		if !write(handler.JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/subscriptions/acknowledged", Params: map[string]any{"notifications": stream.Acknowledged, "_meta": map[string]any{mcp.MetaKeySubscriptionID: request.ID}}}) {
			return
		}
	} else {
		_, _ = c.Writer.Write([]byte(": connected\n\n"))
		c.Writer.Flush()
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-stream.Done:
			if !drainStream(c.Request.Context(), stream, write) {
				return
			}
			if c.Request.Context().Err() == nil && request != nil {
				write(h.StreamCompletion(request))
			}
			return
		case notification := <-stream.Messages:
			if !write(notification) {
				return
			}
		case <-ticker.C:
			_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := c.Writer.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			c.Writer.Flush()
			_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
		}
	}
}

// A naturally ended event producer may have queued a final terminated message.
// Wait for its producer, then send the remaining messages before completion.
func drainStream(ctx context.Context, stream *handler.NotificationStream, write func(any) bool) bool {
	if stream.Finished != nil {
		select {
		case <-stream.Finished:
		case <-ctx.Done():
			return false
		}
	}
	for {
		select {
		case <-ctx.Done():
			return false
		case notification, ok := <-stream.Messages:
			if !ok {
				return true
			}
			if !write(notification) {
				return false
			}
		default:
			return true
		}
	}
}

func serveMCPWebSocket(h *handler.GatewayHandler, exposeMode string, group bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		logCtx := buildLogContext(c, exposeMode)
		if group {
			logCtx.GroupSlug = c.Param("slug")
		}
		upgrader := websocket.Upgrader{CheckOrigin: allowedMCPOrigin}
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		logCtx.ConnectionID = uuid.NewString()
		conn.SetReadLimit(maxMCPBody)
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		closeOnCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer closeOnCancel()
		var writeMu sync.Mutex
		write := func(value any) bool {
			writeMu.Lock()
			defer writeMu.Unlock()
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteJSON(value) != nil {
				cancel()
				_ = conn.Close()
				return false
			}
			return true
		}
		var legacyID string
		defer func() {
			if legacyID != "" {
				clone := *logCtx
				clone.SessionID = legacyID
				_ = h.RemoveLegacySession(&clone)
			}
		}()
		semaphore := make(chan struct{}, 32)
		for {
			kind, body, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if kind != websocket.TextMessage {
				continue
			}
			request, err := decodeMCPRequest(body)
			if err != nil {
				write(&handler.JSONRPCResponse{JSONRPC: "2.0", Error: &handler.RPCError{Code: -32700, Message: "Invalid JSON-RPC JSON"}})
				continue
			}
			if failure := handler.ValidateRequest(request); failure != nil {
				write(failure)
				continue
			}
			clone := *logCtx
			clone.SessionID = legacyID
			if handler.IsModernRequest(request) {
				clone.SessionID = ""
			}
			if request.Method != "notifications/cancelled" {
				if h.CheckPrincipal(ctx, &clone) != nil {
					write(&handler.JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: &handler.RPCError{Code: -32012, Message: "API key or endpoint access is no longer authorized"}})
					return
				}
			}
			if request.Method == "tools/call" {
				user, err := model.GetUserByID(clone.UserID)
				if err != nil {
					return
				}
				wait, _ := middleware.ConsumeMCPToolQuota(clone.UserID, user.Group)
				if wait > 0 {
					write(&handler.JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: &handler.RPCError{Code: -32013, Message: "Tool call rate limit exceeded", Data: map[string]any{"retryAfterSeconds": wait}}})
					continue
				}
			}
			if request.Method == "initialize" {
				response := h.HandleRequest(ctx, request, &clone)
				if response == nil || response.Error != nil {
					if response != nil {
						write(response)
					}
					continue
				}
				if legacyID != "" {
					_ = h.RemoveLegacySession(&clone)
				}
				legacyID, err = h.CreateLegacySession(&clone)
				if err != nil {
					write(&handler.JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: &handler.RPCError{Code: -32013, Message: err.Error()}})
					continue
				}
				clone.SessionID = legacyID
				if !write(response) {
					return
				}
				stream, err := h.OpenLegacyStream(ctx, &clone)
				if err == nil {
					go func() {
						defer stream.Stop()
						for {
							select {
							case <-ctx.Done():
								return
							case <-stream.Done:
								return
							case n := <-stream.Messages:
								if !write(n) {
									return
								}
							}
						}
					}()
				}
				continue
			}
			if request.Method == "notifications/cancelled" || request.Method == "notifications/initialized" {
				h.HandleRequest(ctx, request, &clone)
				continue
			}
			streaming := request.Method == "subscriptions/listen" || request.Method == "events/stream"
			if !streaming {
				select {
				case semaphore <- struct{}{}:
				case <-ctx.Done():
					return
				default:
					write(&handler.JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: &handler.RPCError{Code: -32013, Message: "Too many concurrent WebSocket requests"}})
					continue
				}
			}
			go func(request *handler.JSONRPCRequest, scope handler.LogContext, streaming bool) {
				if !streaming {
					defer func() { <-semaphore }()
				}
				if request.Method != "subscriptions/listen" && request.Method != "events/stream" {
					if response := h.HandleRequest(ctx, request, &scope); response != nil {
						write(response)
					}
					return
				}
				var stream *handler.NotificationStream
				var failure *handler.JSONRPCResponse
				if request.Method == "subscriptions/listen" {
					stream, failure = h.OpenSubscriptions(ctx, request, &scope)
				} else {
					stream, failure = h.OpenEventStream(ctx, request, &scope)
				}
				if failure != nil {
					write(failure)
					return
				}
				defer stream.Stop()
				if request.Method == "subscriptions/listen" && !write(handler.JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/subscriptions/acknowledged", Params: map[string]any{"notifications": stream.Acknowledged, "_meta": map[string]any{mcp.MetaKeySubscriptionID: request.ID}}}) {
					return
				}
				for {
					select {
					case <-ctx.Done():
						return
					case <-stream.Done:
						if !drainStream(ctx, stream, write) {
							return
						}
						if ctx.Err() == nil {
							write(h.StreamCompletion(request))
						}
						return
					case n := <-stream.Messages:
						if !write(n) {
							return
						}
					}
				}
			}(request, clone, streaming)
		}
	}
}
