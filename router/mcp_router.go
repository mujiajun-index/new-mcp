package router

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/middleware"
)

var gatewayHandler *handler.GatewayHandler

func SetMCPRouter(engine *gin.Engine, h *handler.GatewayHandler) {
	gatewayHandler = h

	// Streamable HTTP - Direct mode (expose all tools from APIKey's groups)
	engine.POST("/mcp", middleware.APIKeyAuth(), handleStreamableHTTP("direct"))

	// Streamable HTTP - Smart mode (expose 3 meta-tools)
	engine.POST("/smart/mcp", middleware.APIKeyAuth(), handleStreamableHTTP("smart"))

	// Streamable HTTP - Group endpoint (mode from group config)
	engine.POST("/mcp/group/:slug", middleware.APIKeyAuth(), handleStreamableHTTPWithSlug())

	// WebSocket - Direct mode
	engine.GET("/mcp/ws", middleware.APIKeyAuth(), handleWebSocket("direct"))

	// WebSocket - Smart mode
	engine.GET("/smart/mcp/ws", middleware.APIKeyAuth(), handleWebSocket("smart"))

	// WebSocket - Group endpoint
	engine.GET("/mcp/ws/group/:slug", middleware.APIKeyAuth(), handleWebSocketWithSlug())
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
			ClientIP:   c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		}
	}

	return &handler.LogContext{
		ApiKeyID:   info.ApiKeyID,
		UserID:     info.UserID,
		Username:   info.Username,
		ApiKeyName: info.ApiKeyName,
		ExposeMode: exposeMode,
		ClientIP:   c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	}
}

func handleStreamableHTTP(exposeMode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		logCtx := buildLogContext(c, exposeMode)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read body failed"})
			return
		}

		var req handler.JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON-RPC"})
			return
		}

		resp := gatewayHandler.HandleRequest(c.Request.Context(), &req, logCtx)
		if resp == nil {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func handleStreamableHTTPWithSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		logCtx := buildLogContext(c, "")
		logCtx.GroupSlug = slug

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "read body failed"})
			return
		}

		var req handler.JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON-RPC"})
			return
		}

		resp := gatewayHandler.HandleRequest(c.Request.Context(), &req, logCtx)
		if resp == nil {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func handleWebSocket(exposeMode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "WebSocket transport coming soon"})
	}
}

func handleWebSocketWithSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "WebSocket transport coming soon"})
	}
}
