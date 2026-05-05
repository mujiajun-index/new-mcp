package router

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
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

func buildLogContext(c *gin.Context, slug string) *handler.LogContext {
	apiKeyID := c.GetInt64("api_key_id")
	userID := c.GetInt64("api_key_user_id")

	var username, apiKeyName string
	var user model.User
	if err := model.DB.Select("username").First(&user, userID).Error; err == nil {
		username = user.Username
	}
	var apiKey model.ApiKey
	if err := model.DB.Select("name").First(&apiKey, apiKeyID).Error; err == nil {
		apiKeyName = apiKey.Name
	}

	return &handler.LogContext{
		ApiKeyID:   apiKeyID,
		UserID:     userID,
		Username:   username,
		ApiKeyName: apiKeyName,
		GroupSlug:  slug,
		ClientIP:   c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	}
}

func handleStreamableHTTP(defaultSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		logCtx := buildLogContext(c, defaultSlug)

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
		logCtx := buildLogContext(c, slug)

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

func handleWebSocket(defaultSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "WebSocket transport coming soon"})
	}
}

func handleWebSocketWithSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "WebSocket transport coming soon"})
	}
}
