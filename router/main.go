package router

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/cloud"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/service"
)

var (
	GatewayHandler *handler.GatewayHandler
	SessionPool    *bridge.SessionPool
	CloudManager   *cloud.Manager
)

func InitGateway() {
	SessionPool = bridge.NewSessionPool()
	toolRouter := bridge.NewToolRouter(SessionPool)
	GatewayHandler = handler.NewGatewayHandler(SessionPool, toolRouter)

	CloudManager = cloud.NewManager(SessionPool, toolRouter)
	service.CloudManager = CloudManager
	service.SessionPool = SessionPool
}

func SetRouter(engine *gin.Engine) {
	InitGateway()
	SetApiRouter(engine)
	SetMCPRouter(engine, GatewayHandler)
	serveFrontend(engine)
}

// serveFrontend serves the React SPA static files from web/dist.
// If the directory doesn't exist (backend-only mode), this is a no-op.
func serveFrontend(engine *gin.Engine) {
	distDir := filepath.Join("web", "dist")
	if info, err := os.Stat(distDir); err != nil || !info.IsDir() {
		return
	}

	engine.Static("/assets", filepath.Join(distDir, "assets"))

	// Favicon and other root-level static files
	engine.StaticFile("/favicon.ico", filepath.Join(distDir, "favicon.ico"))
	engine.StaticFile("/vite.svg", filepath.Join(distDir, "vite.svg"))

	// SPA fallback: any non-API, non-MCP route serves index.html
	engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if len(path) >= 4 && (path[:4] == "/api" || path[:4] == "/mcp") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		indexPath := filepath.Join(distDir, "index.html")
		c.File(indexPath)
	})
}

func StartCloudConnections() {
	if CloudManager != nil {
		CloudManager.StartAll()
	}
}

func StopCloudConnections() {
	if CloudManager != nil {
		CloudManager.StopAll()
	}
}
