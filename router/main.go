package router

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/camera"
	"github.com/mujkjk/newmcp/internal/mcp/cloud"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var (
	GatewayHandler *handler.GatewayHandler
	SessionPool    *bridge.SessionPool
	CloudManager   *cloud.Manager
	VirtualRegistry *virtual.VirtualToolRegistry
	CameraStream   *camera.CameraStreamManager
)

func InitGateway() {
	SessionPool = bridge.NewSessionPool()
	toolRouter := bridge.NewToolRouter(SessionPool)
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	CameraStream = camera.NewCameraStreamManager()

	virtual.StreamManager = CameraStream

	GatewayHandler = handler.NewGatewayHandler(SessionPool, toolRouter, VirtualRegistry)

	loadVirtualServices()

	CloudManager = cloud.NewManager(SessionPool, toolRouter)
	service.CloudManager = CloudManager
	service.SessionPool = SessionPool
	service.VirtualRegistry = VirtualRegistry
	service.CameraStreamMgr = CameraStream
}

func loadVirtualServices() {
	var services []model.McpService
	model.DB.Where("transport_type = ?", "virtual").Find(&services)

	for _, svc := range services {
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(svc.Config), &config)

		virtualType, _ := config["virtual_type"].(string)
		switch virtualType {
		case "vision":
			VirtualRegistry.Register(svc.ID, virtual.VisionHandler)
		case "camera":
			VirtualRegistry.Register(svc.ID, virtual.CameraHandler)
		default:
			log.Printf("[virtual] unknown virtual_type %q for service %d", virtualType, svc.ID)
		}
	}
	log.Printf("[virtual] loaded %d virtual services", len(services))
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
