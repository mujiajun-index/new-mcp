package router

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp" // package frontend: embedded web/dist SPA assets
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/camera"
	"github.com/mujkjk/newmcp/internal/mcp/cloud"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var (
	GatewayHandler  *handler.GatewayHandler
	SessionPool     *bridge.SessionPool
	CloudManager    *cloud.Manager
	VirtualRegistry *virtual.VirtualToolRegistry
	CameraStream    *camera.CameraStreamManager
)

func InitGateway() {
	SessionPool = bridge.NewSessionPool()
	toolRouter := bridge.NewToolRouter(SessionPool)
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	CameraStream = camera.NewCameraStreamManager()

	virtual.StreamManager = CameraStream

	GatewayHandler = handler.NewGatewayHandler(SessionPool, toolRouter, VirtualRegistry)

	loadVirtualServices()

	CloudManager = cloud.NewManager(SessionPool, toolRouter, GatewayHandler)
	service.CloudManager = CloudManager
	service.SessionPool = SessionPool
	service.VirtualRegistry = VirtualRegistry
	service.CameraStreamMgr = CameraStream
}

func loadVirtualServices() {
	var services []model.McpService
	model.DB.Where("transport_type = ?", "virtual").Find(&services)

	for _, svc := range services {
		config := virtual.ParseConfig(svc.Config)

		virtualType, _ := config["virtual_type"].(string)
		switch virtualType {
		case "vision":
			VirtualRegistry.Register(svc.ID, svc.UserID, svc.Name, config, virtual.VisionHandler)
		case "camera":
			VirtualRegistry.Register(svc.ID, svc.UserID, svc.Name, config, virtual.CameraHandler)
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

// serveFrontend serves the React SPA from the frontend assets embedded into the
// binary (web/dist, via //go:embed). Embedding the build output removes any
// runtime dependency on the filesystem layout, so the served paths always match
// what rsbuild produced — no more /static vs /assets mismatches.
func serveFrontend(engine *gin.Engine) {
	dist, err := fs.Sub(frontend.DistFS, "web/dist")
	if err != nil {
		log.Printf("[web] embedded frontend unavailable: %v", err)
		return
	}
	fileServer := http.FileServer(http.FS(dist))

	engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Unmatched API/MCP requests return a JSON 404; everything else is a SPA
		// route and gets the HTML shell. Use "/api/" (trailing slash) so frontend
		// pages like /api-keys are NOT mistaken for API calls.
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/mcp") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// Serve a real embedded asset (js/css/svg/png/favicon) when present.
		if path != "/" {
			rel := strings.TrimPrefix(path, "/")
			if f, openErr := dist.Open(rel); openErr == nil {
				isDir := false
				if st, statErr := f.Stat(); statErr == nil {
					isDir = st.IsDir()
				}
				f.Close()
				if !isDir {
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
			}
		}

		// SPA fallback: unknown routes serve index.html for client-side routing.
		// Serve the bytes directly via c.Data — http.FileServer redirects any
		// path ending in /index.html to "./", which would loop forever on "/".
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", frontend.IndexHTML)
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
