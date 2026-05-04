package router

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/middleware"
)

var gatewayHandler *handler.GatewayHandler

func SetMCPRouter(engine *gin.Engine, h *handler.GatewayHandler) {
	gatewayHandler = h

	// Streamable HTTP - main endpoint (Smart mode, all API key groups)
	engine.POST("/mcp", middleware.APIKeyAuth(), handleStreamableHTTP(""))

	// Streamable HTTP - group endpoint (mode driven by group config)
	engine.POST("/mcp/group/:slug", middleware.APIKeyAuth(), handleStreamableHTTPWithSlug())

	// WebSocket - main endpoint
	engine.GET("/mcp/ws", middleware.APIKeyAuth(), handleWebSocket(""))

	// WebSocket - group endpoint
	engine.GET("/mcp/ws/group/:slug", middleware.APIKeyAuth(), handleWebSocketWithSlug())
}

func handleStreamableHTTP(defaultSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKeyID := c.GetInt64("api_key_id")

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

		resp := gatewayHandler.HandleRequest(c.Request.Context(), &req, apiKeyID, defaultSlug)
		if resp == nil {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func handleStreamableHTTPWithSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKeyID := c.GetInt64("api_key_id")
		slug := c.Param("slug")

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

		resp := gatewayHandler.HandleRequest(c.Request.Context(), &req, apiKeyID, slug)
		if resp == nil {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func handleWebSocket(defaultSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket upgrade and message loop will be implemented
		// when gorilla/websocket is integrated
		c.JSON(http.StatusNotImplemented, gin.H{"error": "WebSocket transport coming soon"})
	}
}

func handleWebSocketWithSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "WebSocket transport coming soon"})
	}
}
